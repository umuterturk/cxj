package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	Results []result `json:"results"`
}
type statusReply struct {
	Status           string `json:"status"`
	IndexedFiles     int    `json:"indexed_files"`
	IndexedJavaFiles int    `json:"indexed_java_files"`
	Generation       int    `json:"generation"`
	GoHeapAllocBytes uint64 `json:"go_heap_alloc_bytes"`
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

type workspace struct {
	root     string
	excludes []string
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

func open(t *testing.T, root string, excludes ...string) *workspace {
	t.Helper()
	return &workspace{root: root, excludes: excludes}
}

func (w *workspace) extra() []string {
	var out []string
	for _, e := range w.excludes {
		out = append(out, "--exclude", e)
	}
	return out
}

func (w *workspace) run(t *testing.T, args ...string) []byte {
	t.Helper()
	final := append([]string{}, args...)
	final = append(final, "--root", w.root)
	final = append(final, w.extra()...)
	cmd := exec.Command(binary(t), final...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cx %v failed: %v; output=%s", args, err, out)
	}
	return out
}

func (w *workspace) runJSON(t *testing.T, dest any, args ...string) {
	t.Helper()
	out := w.run(t, append(args, "--json")...)
	if err := json.Unmarshal(out, dest); err != nil {
		t.Fatalf("cx %v JSON: %v; output=%s", args, err, out)
	}
}

func (w *workspace) status(t *testing.T) statusReply {
	t.Helper()
	var v statusReply
	w.runJSON(t, &v, "status")
	return v
}

func (w *workspace) search(t *testing.T, query string) []result {
	t.Helper()
	var v searchReply
	w.runJSON(t, &v, "find", query)
	return v.Results
}

func (w *workspace) references(t *testing.T, symbol string) refReply {
	t.Helper()
	var v refReply
	w.runJSON(t, &v, "references", symbol)
	return v
}

func (w *workspace) endpoints(t *testing.T, method, path string) endpointReply {
	t.Helper()
	args := []string{"endpoints"}
	if method != "" {
		args = append(args, "--method", method)
	}
	if path != "" {
		args = append(args, "--path", path)
	}
	var v endpointReply
	w.runJSON(t, &v, args...)
	return v
}

func (w *workspace) beans(t *testing.T, typ string) beanReply {
	t.Helper()
	var v beanReply
	w.runJSON(t, &v, "beans", "--type", typ)
	return v
}

func (w *workspace) config(t *testing.T, key, profile string) configReply {
	t.Helper()
	args := []string{"config", "--key", key}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	var v configReply
	w.runJSON(t, &v, args...)
	return v
}

func (w *workspace) context(t *testing.T, symbol string) map[string]any {
	t.Helper()
	var v map[string]any
	out := w.run(t, "context", "--symbol", symbol, "--budget", "500")
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("cx context JSON: %v; output=%s", err, out)
	}
	return v
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
