package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestP7LargeRepository500KLinesStaysInMemory(t *testing.T) {
	if testing.Short() {
		t.Fatal("no short-mode skip: full 500K LOC gate is mandatory")
	}
	root := t.TempDir()
	t.Setenv("CX_READY_TIMEOUT", "120s")
	// 1000 Java files, each with 12 methods and ~500 LOC; substantial parse input.
	// The benchmark uses code statements, not 500K blank/comment-only lines.
	totalLines := 0
	for f := 0; f < 1000; f++ {
		var b strings.Builder
		fmt.Fprintf(&b, "class GeneratedCode%d {\n", f)
		totalLines++
		for m := 0; m < 12; m++ {
			fmt.Fprintf(&b, "  int task%d(int input) {\n    int result = input;\n", m)
			totalLines += 2
			for j := 0; j < 39; j++ {
				fmt.Fprintf(&b, "    result += %d;\n", j+1)
				totalLines++
			}
			b.WriteString("    return result;\n  }\n")
			totalLines += 2
		}
		b.WriteString("}\n")
		totalLines++
		put(t, root, fmt.Sprintf("src/GeneratedCode%d.java", f), b.String())
	}
	if totalLines < 500000 {
		t.Fatalf("benchmark did not generate 500K LOC: %d", totalLines)
	}
	// This test starts the deterministic in-memory index without model vectors.
	// Model memory and full semantic-indexing wall time are reported separately.
	e := start(t, root, false)
	s := e.status(t)
	if s.IndexedJavaFiles != 1000 {
		t.Fatalf("expected 1000 Java files, got %+v", s)
	}
	if s.GoHeapAllocBytes >= 512*1024*1024 {
		t.Fatalf("Go index exceeded 512 MiB budget: %d bytes", s.GoHeapAllocBytes)
	}
	started := time.Now()
	requirePath(t, e.search(t, "GeneratedCode999.task11", "lexical"), "GeneratedCode999.java")
	if time.Since(started) > time.Second {
		t.Fatalf("warm single symbol query took >1s")
	}
}
func TestP7PinnedRealPetClinicSmoke(t *testing.T) {
	repo := os.Getenv("PETCLINIC_ROOT")
	expected := os.Getenv("PETCLINIC_COMMIT")
	if repo == "" || expected == "" {
		t.Fatal("PETCLINIC_ROOT and PETCLINIC_COMMIT required (clone official spring-projects/spring-petclinic and pin SHA)")
	}
	output, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("PetClinic git checkout required: %v", err)
	}
	if strings.TrimSpace(string(output)) != expected {
		t.Fatalf("expected pinned PetClinic commit %s, got %s", expected, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(filepath.Join(repo, "pom.xml")); err != nil {
		t.Fatalf("expected PetClinic Maven project: %v", err)
	}
	e := start(t, repo, false)
	results := e.search(t, "OwnerController", "lexical")
	if !containsSymbol(results, "OwnerController") {
		t.Fatalf("PetClinic controller symbol missing: %#v", results)
	}
	// PetClinic is a real validation corpus, not a fixture-specific search mapping.
	if e.status(t).IndexedJavaFiles < 20 {
		t.Fatalf("not enough Java files indexed from PetClinic")
	}
}
func TestP7MeasuredBenchmarkReportExists(t *testing.T) {
	// A separate CLI benchmarking tool/step must write measured, reproducible data.
	// File may exist on disk: the INDEX still must not persist any source/vector data.
	path := filepath.Join("..", "reports", "benchmark.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("benchmark report not produced: %v", err)
	}
	var report struct {
		Repository            string  `json:"repository"`
		GoIndexMemoryBytes    uint64  `json:"go_index_memory_bytes"`
		WarmLexicalP95Ms      float64 `json:"warm_lexical_p95_ms"`
		SemanticRecallAt10    float64 `json:"semantic_recall_at_10"`
		EmbeddingIndexSeconds float64 `json:"embedding_index_seconds"`
		Model                 string  `json:"model"`
		Commit                string  `json:"commit"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Repository == "" || report.Commit == "" || report.Model == "" || report.GoIndexMemoryBytes == 0 || report.WarmLexicalP95Ms <= 0 || report.SemanticRecallAt10 <= 0 || report.EmbeddingIndexSeconds <= 0 {
		t.Fatalf("report missing measurement fields: %+v", report)
	}
}
