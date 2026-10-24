package acceptance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestP6CreateModifyDeleteAndRenameAreVisible(t *testing.T) {
	root := t.TempDir()
	put(t, root, "LiveAlpha.java", "class LiveAlpha {}\n")
	e := start(t, root, false)
	requirePath(t, e.search(t, "LiveAlpha", "lexical"), "LiveAlpha.java")
	put(t, root, "LiveAlpha.java", "class LiveBeta {}\n")
	poll(t, "modified Java symbol", func() bool { return hasPath(e.search(t, "LiveBeta", "lexical"), "LiveAlpha.java") })
	poll(t, "old symbol removal", func() bool { return !hasPath(e.search(t, "LiveAlpha", "lexical"), "LiveAlpha.java") })
	if err := os.Rename(filepath.Join(root, "LiveAlpha.java"), filepath.Join(root, "Renamed.java")); err != nil {
		t.Fatal(err)
	}
	poll(t, "renamed path", func() bool { return hasPath(e.search(t, "LiveBeta", "lexical"), "Renamed.java") })
	if err := os.Remove(filepath.Join(root, "Renamed.java")); err != nil {
		t.Fatal(err)
	}
	poll(t, "deleted symbol removal", func() bool { return !hasPath(e.search(t, "LiveBeta", "lexical"), "Renamed.java") })
}
func TestP6UpdatesExcludedFilesWithoutLeaks(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Generated/Ignore.java", "class Ignore {}\n")
	e := start(t, root, false, "Generated/**")
	put(t, root, "Generated/NewDecoy.java", "class NewDecoy {}\n")
	time.Sleep(400 * time.Millisecond)
	requireNoPath(t, e.search(t, "NewDecoy", "lexical"), "NewDecoy.java")
}
func TestP6ConcurrentQueriesSeeConsistentIndex(t *testing.T) {
	root := t.TempDir()
	put(t, root, "ConcurrentSearch.java", "class ConcurrentSearch { void concurrentMethod() {} }\n")
	e := start(t, root, false)
	var wg sync.WaitGroup
	failures := make(chan string, 100)
	for worker := 0; worker < 12; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				var response searchReply
				if err := e.get("/v1/search?q=ConcurrentSearch&mode=lexical", &response); err != nil {
					failures <- err.Error()
					return
				}
				if !hasPath(response.Results, "ConcurrentSearch.java") {
					failures <- "inconsistent or missing result"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for msg := range failures {
		t.Errorf("query failed: %s", msg)
	}
}
func TestP6MCPStdioAdvertisesAndExecutesFindTool(t *testing.T) {
	root := t.TempDir()
	put(t, root, "ToolSmoke.java", "class ToolSmoke {}\n")
	cmd := exec.Command(binary(t), "mcp", "--root", root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	reader := bufio.NewReader(out)
	send := func(v any) {
		b, _ := json.Marshal(v)
		_, err := fmt.Fprintln(in, string(b))
		if err != nil {
			t.Fatal(err)
		}
	}
	receive := func() map[string]any {
		ch := make(chan string, 1)
		go func() { line, _ := reader.ReadString('\n'); ch <- line }()
		select {
		case s := <-ch:
			var msg map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &msg); err != nil {
				t.Fatalf("MCP stdout must contain only JSON-RPC, got %q; stderr=%s", s, stderr.String())
			}
			return msg
		case <-time.After(25 * time.Second):
			t.Fatalf("MCP stdio response timeout; stderr=%s", stderr.String())
		}
		return nil
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "cx-acceptance", "version": "0.1"}}})
	init := receive()
	if init["result"] == nil {
		t.Fatalf("MCP initialize did not succeed: %+v", init)
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	listed := receive()
	body, _ := json.Marshal(listed)
	if !bytes.Contains(body, []byte(`"find"`)) {
		t.Fatalf("MCP find tool not advertised: %s", body)
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "find", "arguments": map[string]any{"query": "ToolSmoke"}}})
	called := receive()
	body, _ = json.Marshal(called)
	if !bytes.Contains(body, []byte("ToolSmoke.java")) {
		t.Fatalf("MCP tool did not return real source: %s", body)
	}
}
func TestP6ModifiedJavaGetsNewSemanticVector(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/BankDocumentSigner.java", `class BankDocumentSigner {
 // Sign documents with a digital certificate after validation.
 void signPaper() {}
}`)
	e := start(t, root, true)
	e.waitSemantic(t)
	initialSemanticGeneration := e.status(t).SemanticGeneration
	put(t, root, "src/BankDocumentSigner.java", `class BankDocumentSigner {
 // Reconcile inventory inventory counts in cold warehouses.
 void reconcileStocks() {}
}`)
	poll(t, "semantic reindexing after Java edit", func() bool {
		s := e.status(t)
		if s.SemanticState != "ready" || s.SemanticGeneration <= initialSemanticGeneration {
			return false
		}
		return hasPath(e.search(t, "warehouse inventory stock reconciliation", "semantic"), "BankDocumentSigner.java")
	})
	// A changed class must not still claim an old exact method.
	if containsSymbol(e.search(t, "BankDocumentSigner.signPaper", "lexical"), "signPaper") {
		t.Fatal("stale Java symbol survived edit")
	}
}
