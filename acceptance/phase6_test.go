package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
func TestP6CLIFindReturnsRealSource(t *testing.T) {
	root := t.TempDir()
	put(t, root, "ToolSmoke.java", "class ToolSmoke {}\n")
	e := start(t, root, false)
	cmd := exec.Command(binary(t), "find", "ToolSmoke", "--server", e.endpoint, "--mode", "lexical", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cx find failed: %v; output=%s", err, out)
	}
	if !bytes.Contains(out, []byte("ToolSmoke.java")) {
		t.Fatalf("cx find did not return real source: %s", out)
	}
	var reply searchReply
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("cx find --json must emit search JSON: %v; output=%s", err, out)
	}
	if !hasPath(reply.Results, "ToolSmoke.java") {
		t.Fatalf("expected ToolSmoke.java in results: %#v", reply.Results)
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
