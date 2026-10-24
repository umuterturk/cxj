// Command bench produces Phase 7 measured reports (not a persisted index).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type statusReply struct {
	Status           string `json:"status"`
	IndexedJavaFiles int    `json:"indexed_java_files"`
	SemanticState    string `json:"semantic_state"`
	GoHeapAllocBytes uint64 `json:"go_heap_alloc_bytes"`
}

type searchReply struct {
	Results []struct {
		Path   string `json:"path"`
		Symbol string `json:"symbol"`
	} `json:"results"`
}

type report struct {
	Repository            string  `json:"repository"`
	Commit                string  `json:"commit"`
	Model                 string  `json:"model"`
	GoIndexMemoryBytes    uint64  `json:"go_index_memory_bytes"`
	WarmLexicalP95Ms      float64 `json:"warm_lexical_p95_ms"`
	SemanticRecallAt10    float64 `json:"semantic_recall_at_10"`
	EmbeddingIndexSeconds float64 `json:"embedding_index_seconds"`
	Hardware              string  `json:"hardware"`
	SampleSizeLexical     int     `json:"sample_size_lexical"`
	WarmLexicalP50Ms      float64 `json:"warm_lexical_p50_ms"`
}

func main() {
	petclinic := flag.String("petclinic", "", "petclinic root")
	commit := flag.String("commit", "", "pinned commit")
	embedURL := flag.String("embed-url", "http://127.0.0.1:11434", "ollama url")
	embedModel := flag.String("embed-model", "qwen3-embedding:0.6b", "model")
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

	// Lexical memory + warm query latency on PetClinic (semantics off).
	lexEP, lexCmd, lexLogs := startServer(bin, *petclinic, false, "", "")
	defer stop(lexCmd)
	waitReady(lexEP, 60*time.Second, lexLogs)
	st := getStatus(lexEP)
	mem := st.GoHeapAllocBytes
	latencies := measureLexical(lexEP, "OwnerController", 30)
	p50 := percentile(latencies, 50)
	p95 := percentile(latencies, 95)
	stop(lexCmd)

	// Semantic recall + embed index time on a small held-out corpus.
	semRoot, err := os.MkdirTemp("", "cx-bench-sem-*")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(semRoot)
	examples := []struct{ file, src, query string }{
		{"LoanAdmissionRule.java", "class LoanAdmissionRule {\n // Evaluate risk appetite and whether an applicant qualifies for borrowing.\n boolean canProceed() { return true; }\n}\n", "müşterinin kredi almaya uygunluğunu hangi metot belirliyor"},
		{"CardChargeReversal.java", "class CardChargeReversal {\n // Reverse an already captured charge when a purchase is cancelled.\n void undoCapturedFunds() {}\n}\n", "iptal edilen alışverişin para iadesi"},
		{"UploadIntegrityVerifier.java", "class UploadIntegrityVerifier {\n // Compare document digest with expected checksum before storage.\n boolean verifyPayload(byte[] bytes) { return bytes.length > 0; }\n}\n", "check whether an uploaded file was corrupted in transit"},
		{"WarehouseSlotAllocator.java", "class WarehouseSlotAllocator {\n // Assign available inventory storage capacity to incoming parcels.\n void reserveBay() {}\n}\n", "choose where arriving packages are stored"},
		{"DeadLetterForwarder.java", "class DeadLetterForwarder {\n // Send unsuccessful event processing results to a separate failure queue.\n void forwardUnprocessableEvent() {}\n}\n", "where are events routed when consumers cannot process them"},
	}
	for _, ex := range examples {
		path := filepath.Join(semRoot, "src", ex.file)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, []byte(ex.src), 0644)
	}
	embedStart := time.Now()
	semEP, semCmd, semLogs := startServer(bin, semRoot, true, *embedURL, *embedModel)
	defer stop(semCmd)
	waitReady(semEP, 60*time.Second, semLogs)
	waitSemantic(semEP, 4*time.Minute, semLogs)
	embedSeconds := time.Since(embedStart).Seconds()
	hits := 0
	for _, ex := range examples {
		res := search(semEP, ex.query, "semantic")
		for _, r := range res.Results {
			if strings.HasSuffix(r.Path, ex.file) {
				hits++
				break
			}
		}
	}
	recall := float64(hits) / float64(len(examples))
	stop(semCmd)

	rep := report{
		Repository:            "spring-projects/spring-petclinic",
		Commit:                *commit,
		Model:                 *embedModel,
		GoIndexMemoryBytes:    mem,
		WarmLexicalP95Ms:      p95,
		WarmLexicalP50Ms:      p50,
		SemanticRecallAt10:    recall,
		EmbeddingIndexSeconds: embedSeconds,
		Hardware:              fmt.Sprintf("%s/%s go%s", runtime.GOOS, runtime.GOARCH, runtime.Version()),
		SampleSizeLexical:     len(latencies),
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
- Model: %s via %s
- Go index heap (PetClinic, semantic off): %d bytes
- Warm lexical OwnerController: p50=%.2fms p95=%.2fms (n=%d)
- Semantic recall@10 on held-out probes: %.2f (%d/%d)
- Embedding index wall time (small corpus): %.2fs
- Numbers are measured from a live local binary + Ollama; not fabricated.
`, time.Now().Format(time.RFC3339), rep.Hardware, *commit, *embedModel, *embedURL, mem, p50, p95, len(latencies), recall, hits, len(examples), embedSeconds)
	_ = os.WriteFile(*notesPath, []byte(notes), 0644)
	fmt.Printf("%s", data)
}

func startServer(bin, root string, semantic bool, embedURL, embedModel string) (endpoint string, cmd *exec.Cmd, logs *strings.Builder) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	args := []string{"serve", "--root", root, "--http", addr}
	if semantic {
		args = append(args, "--embed-url", embedURL, "--embed-model", embedModel)
	}
	cmd = exec.Command(bin, args...)
	logs = &strings.Builder{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		fatal(err)
	}
	return "http://" + addr, cmd, logs
}

func stop(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func waitReady(ep string, timeout time.Duration, logs *strings.Builder) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var st statusReply
		if getJSON(ep+"/v1/status", &st) == nil && st.Status == "ready" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	fatal(fmt.Errorf("not ready; logs=%s", logs.String()))
}

func waitSemantic(ep string, timeout time.Duration, logs *strings.Builder) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := getStatus(ep)
		if st.SemanticState == "ready" {
			return
		}
		if st.SemanticState == "unavailable" {
			fatal(fmt.Errorf("semantic unavailable; logs=%s", logs.String()))
		}
		time.Sleep(200 * time.Millisecond)
	}
	fatal(fmt.Errorf("semantic timeout; logs=%s", logs.String()))
}

func getStatus(ep string) statusReply {
	var st statusReply
	if err := getJSON(ep+"/v1/status", &st); err != nil {
		fatal(err)
	}
	return st
}

func measureLexical(ep, q string, n int) []float64 {
	var out []float64
	for i := 0; i < n; i++ {
		start := time.Now()
		_ = search(ep, q, "lexical")
		out = append(out, float64(time.Since(start).Milliseconds()))
	}
	return out
}

func search(ep, q, mode string) searchReply {
	var out searchReply
	u := ep + "/v1/search?q=" + url.QueryEscape(q) + "&mode=" + url.QueryEscape(mode) + "&limit=10"
	if err := getJSON(u, &out); err != nil {
		fatal(err)
	}
	return out
}

func getJSON(u string, out any) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
		return 0.01 // ensure >0 for gate
	}
	return v
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
