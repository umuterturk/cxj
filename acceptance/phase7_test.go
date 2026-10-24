package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestP7LargeRepository500KLinesStaysInMemory(t *testing.T) {
	if testing.Short() {
		t.Fatal("no short-mode skip: full 500K LOC gate is mandatory")
	}
	root := t.TempDir()
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
	e := open(t, root)
	s := e.status(t)
	if s.IndexedJavaFiles != 1000 {
		t.Fatalf("expected 1000 Java files, got %+v", s)
	}
	if s.GoHeapAllocBytes >= 512*1024*1024 {
		t.Fatalf("Go index exceeded 512 MiB budget: %d bytes", s.GoHeapAllocBytes)
	}
	requirePath(t, e.search(t, "GeneratedCode999.task11"), "GeneratedCode999.java")
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
	e := open(t, repo)
	results := e.search(t, "OwnerController")
	if !containsSymbol(results, "OwnerController") {
		t.Fatalf("PetClinic controller symbol missing: %#v", results)
	}
	if e.status(t).IndexedJavaFiles < 20 {
		t.Fatalf("not enough Java files indexed from PetClinic")
	}
}

func TestP7MeasuredBenchmarkReportExists(t *testing.T) {
	path := filepath.Join("..", "reports", "benchmark.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("benchmark report not produced: %v", err)
	}
	var report struct {
		Repository         string  `json:"repository"`
		GoIndexMemoryBytes uint64  `json:"go_index_memory_bytes"`
		WarmLexicalP95Ms   float64 `json:"warm_lexical_p95_ms"`
		Commit             string  `json:"commit"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Repository == "" || report.Commit == "" || report.GoIndexMemoryBytes == 0 || report.WarmLexicalP95Ms <= 0 {
		t.Fatalf("report missing measurement fields: %+v", report)
	}
}
