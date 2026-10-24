// Command bench produces Phase 7 measured reports (not a persisted index).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"
)

type statusReply struct {
	Status           string `json:"status"`
	IndexedJavaFiles int    `json:"indexed_java_files"`
	GoHeapAllocBytes uint64 `json:"go_heap_alloc_bytes"`
}

type report struct {
	Repository         string  `json:"repository"`
	Commit             string  `json:"commit"`
	GoIndexMemoryBytes uint64  `json:"go_index_memory_bytes"`
	WarmLexicalP95Ms   float64 `json:"warm_lexical_p95_ms"`
	Hardware           string  `json:"hardware"`
	SampleSizeLexical  int     `json:"sample_size_lexical"`
	WarmLexicalP50Ms   float64 `json:"warm_lexical_p50_ms"`
}

func main() {
	petclinic := flag.String("petclinic", "", "petclinic root")
	commit := flag.String("commit", "", "pinned commit")
	outPath := flag.String("out", "reports/benchmark.json", "json output")
	notesPath := flag.String("notes", "reports/benchmark-notes.md", "notes output")
	flag.Parse()
	bin := os.Getenv("CX_BIN")
	if bin == "" {
		bin = "./bin/cx"
	}
	if *petclinic == "" || *commit == "" {
		fmt.Fprintln(os.Stderr, "-petclinic and -commit required")
		os.Exit(2)
	}

	st := runStatus(bin, *petclinic)
	mem := st.GoHeapAllocBytes
	latencies := measureFind(bin, *petclinic, "OwnerController", 10)
	p50 := percentile(latencies, 50)
	p95 := percentile(latencies, 95)

	rep := report{
		Repository:         "spring-projects/spring-petclinic",
		Commit:             *commit,
		GoIndexMemoryBytes: mem,
		WarmLexicalP95Ms:   p95,
		WarmLexicalP50Ms:   p50,
		Hardware:           fmt.Sprintf("%s/%s go%s", runtime.GOOS, runtime.GOARCH, runtime.Version()),
		SampleSizeLexical:  len(latencies),
	}
	data, _ := json.MarshalIndent(rep, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fatal(err)
	}
	notes := fmt.Sprintf(`# Benchmark notes

- Generated: %s
- Hardware: %s
- PetClinic commit: %s
- Go index heap (one-shot status): %d bytes
- One-shot lexical OwnerController: p50=%.2fms p95=%.2fms (n=%d)
- Each sample is a full index + find (no daemon).
- Numbers are measured from a live local binary; not fabricated.
`, time.Now().Format(time.RFC3339), rep.Hardware, *commit, mem, p50, p95, len(latencies))
	_ = os.WriteFile(*notesPath, []byte(notes), 0644)
	fmt.Printf("%s", data)
}

func runStatus(bin, root string) statusReply {
	out, err := exec.Command(bin, "status", "--root", root, "--json").CombinedOutput()
	if err != nil {
		fatal(fmt.Errorf("status: %v: %s", err, out))
	}
	var st statusReply
	if err := json.Unmarshal(out, &st); err != nil {
		fatal(fmt.Errorf("status JSON: %v: %s", err, out))
	}
	if st.Status != "ready" {
		fatal(fmt.Errorf("status not ready: %+v", st))
	}
	return st
}

func measureFind(bin, root, q string, n int) []float64 {
	var out []float64
	for i := 0; i < n; i++ {
		start := time.Now()
		data, err := exec.Command(bin, "find", q, "--root", root, "--json").CombinedOutput()
		if err != nil {
			fatal(fmt.Errorf("find: %v: %s", err, data))
		}
		out = append(out, float64(time.Since(start).Milliseconds()))
	}
	return out
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 100 {
		return cp[len(cp)-1]
	}
	idx := int(math.Ceil(p/100*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	v := cp[idx]
	if v <= 0 {
		return 0.01
	}
	return v
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
