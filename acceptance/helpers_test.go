package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type result struct {
	Path    string  `json:"path"`
	Line    int     `json:"line"`
	Symbol  string  `json:"symbol"`
	Kind    string  `json:"kind"`
	Match   string  `json:"match"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}
type searchReply struct {
	Results       []result `json:"results"`
	SemanticState string   `json:"semantic_state"`
}
type statusReply struct {
	Status             string `json:"status"`
	IndexedFiles       int    `json:"indexed_files"`
	IndexedJavaFiles   int    `json:"indexed_java_files"`
	Generation         int    `json:"generation"`
	SemanticState      string `json:"semantic_state"`
	SemanticGeneration int    `json:"semantic_generation"`
	GoHeapAllocBytes   uint64 `json:"go_heap_alloc_bytes"`
}
type refReply struct {
	Results []struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Target    string `json:"target"`
		Certainty string `json:"certainty"`
	} `json:"results"`
}
type endpointReply struct {
	Results []struct {
		Path       string `json:"path"`
		Symbol     string `json:"symbol"`
		HTTPMethod string `json:"http_method"`
		Route      string `json:"route"`
		Certainty  string `json:"certainty"`
	} `json:"results"`
}
type beanReply struct {
	Results []struct {
		Bean         string   `json:"bean"`
		Symbol       string   `json:"symbol"`
		InjectedInto []string `json:"injected_into"`
		Certainty    string   `json:"certainty"`
	} `json:"results"`
}
type configReply struct {
	Definitions []struct {
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Value   string `json:"value"`
		Profile string `json:"profile"`
	} `json:"definitions"`
	Bindings        []string `json:"bindings"`
	RepositoryValue string   `json:"repository_value"`
	Resolution      string   `json:"resolution"`
}
type engine struct {
	root, endpoint string
	command        *exec.Cmd
	logs           *bytes.Buffer
}

func put(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
func binary(t *testing.T) string {
	t.Helper()
	b := os.Getenv("CX_BIN")
	if b == "" {
		t.Fatal("CX_BIN required; use ./scripts/acceptance.sh N")
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("CX_BIN not found: %v", err)
	}
	return b
}
func start(t *testing.T, root string, semantic bool, excludes ...string) *engine {
	t.Helper()
	addr := freeAddress(t)
	args := []string{"serve", "--root", root, "--http", addr}
	for _, exclude := range excludes {
		args = append(args, "--exclude", exclude)
	}
	if semantic {
		model := os.Getenv("CX_EMBED_MODEL")
		if model == "" {
			model = "qwen3-embedding:0.6b"
		}
		embedURL := os.Getenv("CX_EMBED_URL")
		if embedURL == "" {
			embedURL = "http://127.0.0.1:11434"
		}
		args = append(args, "--embed-url", embedURL, "--embed-model", model)
	}
	cmd := exec.Command(binary(t), args...)
	logs := &bytes.Buffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	e := &engine{root: root, endpoint: "http://" + addr, command: cmd, logs: logs}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(25 * time.Second)
	if timeout := os.Getenv("CX_READY_TIMEOUT"); timeout != "" {
		duration, err := time.ParseDuration(timeout)
		if err != nil {
			t.Fatalf("invalid CX_READY_TIMEOUT: %v", err)
		}
		deadline = time.Now().Add(duration)
	}
	for time.Now().Before(deadline) {
		var status statusReply
		if e.get("/v1/status", &status) == nil && status.Status == "ready" {
			return e
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("engine did not become lexical-ready; logs:\n%s", logs.String())
	return nil
}
func (e *engine) get(path string, out any) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(e.endpoint + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
func (e *engine) status(t *testing.T) statusReply {
	t.Helper()
	var v statusReply
	if err := e.get("/v1/status", &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func (e *engine) search(t *testing.T, query, mode string) []result {
	t.Helper()
	p := "/v1/search?q=" + url.QueryEscape(query) + "&mode=" + url.QueryEscape(mode) + "&limit=10"
	var v searchReply
	if err := e.get(p, &v); err != nil {
		t.Fatalf("search %q mode %s: %v; logs=%s", query, mode, err, e.logs.String())
	}
	return v.Results
}
func (e *engine) rawStatus(path string) (int, error) {
	response, err := http.Get(e.endpoint + path)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}
func (e *engine) waitSemantic(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		s := e.status(t)
		if s.SemanticState == "ready" {
			return
		}
		if s.SemanticState == "unavailable" {
			t.Fatalf("embedding provider unavailable; run ollama serve and ollama pull qwen3-embedding:0.6b; logs:\n%s", e.logs.String())
		}
		time.Sleep(400 * time.Millisecond)
	}
	t.Fatalf("semantic indexing did not become ready; logs:\n%s", e.logs.String())
}
func hasPath(results []result, suffix string) bool {
	for _, r := range results {
		if strings.HasSuffix(r.Path, suffix) && r.Line > 0 {
			return true
		}
	}
	return false
}
func containsSymbol(results []result, name string) bool {
	for _, r := range results {
		if strings.Contains(r.Symbol, name) && r.Line > 0 {
			return true
		}
	}
	return false
}
func requirePath(t *testing.T, results []result, suffix string) {
	t.Helper()
	if !hasPath(results, suffix) {
		t.Fatalf("expected file %q in results, got %#v", suffix, results)
	}
}
func requireNoPath(t *testing.T, results []result, suffix string) {
	t.Helper()
	if hasPath(results, suffix) {
		t.Fatalf("excluded or deleted file %q appeared: %#v", suffix, results)
	}
}
func poll(t *testing.T, desc string, ok func() bool) {
	t.Helper()
	end := time.Now().Add(8 * time.Second)
	for time.Now().Before(end) {
		if ok() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}
