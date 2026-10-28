package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"example.com/cx/internal/benchfixture"
)

func batchFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := benchfixture.Write(root, benchfixture.Options{Modules: 1, TypesPerModule: 1, TestTypesPerModule: -1}); err != nil {
		t.Fatal(err)
	}
	return root
}

func decodeLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", l, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestBatchHappyPath(t *testing.T) {
	root := batchFixture(t)
	in := strings.Join([]string{
		`{"cmd":"find","arg":"Widget0","limit":3}`,
		`{"cmd":"show","arg":"com.example.bench.mod0.Widget0Service.save","limit":10}`,
		`{"cmd":"trail","arg":"com.example.bench.mod0.Widget0Service.save"}`,
		`{"cmd":"endpoints","method":"GET"}`,
		`{"cmd":"status"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	code := runBatchWith(strings.NewReader(in), &out, []string{"--root", root})
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out.String())
	}
	lines := decodeLines(t, out.String())
	if len(lines) != 6 { // five answers plus the summary
		t.Fatalf("got %d lines:\n%s", len(lines), out.String())
	}
	for i, l := range lines[:5] {
		if l["ok"] != true {
			t.Errorf("line %d not ok: %v", i, l)
		}
		if got := int(l["i"].(float64)); got != i {
			t.Errorf("line %d carries i=%v; answers must arrive in request order", i, l["i"])
		}
	}
	sum := lines[5]
	if sum["batch"] != true || int(sum["items"].(float64)) != 5 || int(sum["ok"].(float64)) != 5 {
		t.Errorf("summary = %v", sum)
	}
	// find pulls ScopeAll breadth into the batch default; the summary says so.
	if sum["scope"] != "all" {
		t.Errorf("scope = %v, want all (find is in the batch)", sum["scope"])
	}
	if _, has := sum["open_ms"]; !has {
		t.Errorf("summary has no open_ms; the amortization proof is %v", sum)
	}
}

func TestBatchOrderSurvivesErrors(t *testing.T) {
	root := batchFixture(t)
	in := strings.Join([]string{
		`{"cmd":"status"}`,
		`this line does not parse`,
		`{"cmd":"install"}`,
		`{"cmd":"find"}`,
		`{"cmd":"show","arg":"com.example.bench.mod0.Widget0Service.save"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	code := runBatchWith(strings.NewReader(in), &out, []string{"--root", root})
	if code != 1 {
		t.Fatalf("exit %d, want 1 (two items failed)", code)
	}
	lines := decodeLines(t, out.String())
	if len(lines) != 6 {
		t.Fatalf("got %d lines:\n%s", len(lines), out.String())
	}
	if lines[1]["ok"] != false || !strings.HasPrefix(lines[1]["error"].(string), "bad JSON") {
		t.Errorf("line 1 = %v, want a bad-JSON error at its own position", lines[1])
	}
	if lines[2]["cmd"] != "install" || strings.Contains(lines[2]["error"].(string), "bad JSON") {
		t.Errorf("line 2 = %v, want a non-batchable error naming install", lines[2])
	}
	if lines[3]["ok"] != false || lines[3]["error"] != "arg required" {
		t.Errorf("line 3 = %v, want arg-required", lines[3])
	}
	if lines[4]["ok"] != true {
		t.Errorf("line 4 = %v, want ok despite earlier failures", lines[4])
	}
	if int(lines[4]["i"].(float64)) != 4 {
		t.Errorf("line 4 carries i=%v; indices must count input lines, errors included", lines[4]["i"])
	}
}

// TestBatchUnionPaysOnlyWhatItReads: a batch of two status lines needs no index
// at all, and the engine.Open of a caps-0 batch must still succeed and answer.
// (The union itself is engine.CapsFor; this pins the batch wiring around it.)
func TestBatchCapsZeroStillAnswers(t *testing.T) {
	root := batchFixture(t)
	var out bytes.Buffer
	code := runBatchWith(strings.NewReader(`{"cmd":"status"}`+"\n"), &out, []string{"--root", root})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out.String())
	}
	lines := decodeLines(t, out.String())
	if len(lines) != 2 || lines[0]["ok"] != true || lines[1]["batch"] != true {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
	if lines[1]["scope"] != "main" {
		t.Errorf("scope = %v, want main for a status-only batch", lines[1]["scope"])
	}
}

func TestBatchEmptyInput(t *testing.T) {
	var out bytes.Buffer
	code := runBatchWith(strings.NewReader("  \n\n# just a comment\n"), &out, []string{"--root", t.TempDir()})
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestBatchUnknownFieldRejected guards DisallowUnknownFields: {"scope":"test"}
// inside a query looks harmless and would silently do nothing.
func TestBatchUnknownFieldRejected(t *testing.T) {
	root := batchFixture(t)
	var out bytes.Buffer
	runBatchWith(strings.NewReader(`{"cmd":"status","scope":"test"}`+"\n"), &out, []string{"--root", root})
	lines := decodeLines(t, out.String())
	if lines[0]["ok"] != false || !strings.Contains(lines[0]["error"].(string), "scope") {
		t.Errorf("line = %v, want an error naming the unknown field", lines[0])
	}
}
