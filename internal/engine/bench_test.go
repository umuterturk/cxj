package engine

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/cx/internal/benchfixture"
	"example.com/cx/internal/scan"
)

// Benchmarks for Track P. They measure the thing statelessness forces us to pay
// on every single agent call: a cold Open, then one query.
//
// The workspace is the shared synthetic fixture (see internal/benchfixture), so
// numbers are reproducible on any machine and in CI without shipping a real repo.
// To measure a real repository instead:
//
//	CXJ_BENCH_REPO=/path/to/repo CXJ_BENCH_SYMBOL=com.acme.FooController.get \
//	  go test ./internal/engine -run '^$' -bench Benchmark -benchtime=3x
//
// Run across core counts to see the Amdahl ceiling and the CPU cost of
// over-subscription on P/E laptops:
//
//	for p in 1 4 8; do GOMAXPROCS=$p go test ./internal/engine -run '^$' -bench Benchmark -benchtime=3x; done

// benchRoot returns the workspace to benchmark.
func benchRoot(b *testing.B) string {
	b.Helper()
	if repo := os.Getenv("CXJ_BENCH_REPO"); repo != "" {
		abs, err := filepath.Abs(repo)
		if err != nil {
			b.Fatal(err)
		}
		if err := scan.ValidateRoot(abs); err != nil {
			b.Fatal(err)
		}
		return abs
	}
	return fixtureRoot(b)
}

var (
	fixtureOnce   string
	fixtureErr    error
	fixtureLoaded bool
)

// TestMain cleans up the shared benchmark fixture. It is built once per process and
// reused by every benchmark, so per-benchmark cleanup would delete it mid-run.
func TestMain(m *testing.M) {
	code := m.Run()
	if fixtureOnce != "" {
		os.RemoveAll(fixtureOnce)
	}
	os.Exit(code)
}

func fixtureRoot(b *testing.B) string {
	b.Helper()
	if !fixtureLoaded {
		fixtureLoaded = true
		fixtureOnce, fixtureErr = os.MkdirTemp("", "cxj-bench-fixture-")
		if fixtureErr == nil {
			_, fixtureErr = benchfixture.Write(fixtureOnce, benchfixture.Options{})
		}
		if fixtureErr != nil {
			b.Fatal(fixtureErr)
		}
	}
	return fixtureOnce
}

// benchSize puts the workspace size on every benchmark line, so a number can never
// be read without the denominator it was measured against.
func benchSize(b *testing.B, root string) {
	b.Helper()
	n := 0
	filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(d.Name()) == ".java" {
			n++
		}
		return nil
	})
	b.ReportMetric(float64(n), "java")
}

// benchSymbol returns a symbol that must resolve. A benchmark that silently queries
// a missing symbol would measure a miss path and report a flattering number, so
// every candidate is verified before it is used.
func benchSymbol(b *testing.B, root string) string {
	b.Helper()
	// Caps here cover both probes below (Trail and Search). Setup only, never timed.
	eng, err := Open(root, Options{Caps: CapsTrail | CapsSearch, Scope: scan.ScopeMain})
	if err != nil {
		b.Fatal(err)
	}
	probe := func(sym string) string {
		if sym == "" {
			return ""
		}
		if eng.Trail(sym).Path != "" {
			return sym
		}
		return ""
	}
	if sym := probe(os.Getenv("CXJ_BENCH_SYMBOL")); sym != "" {
		return sym
	}
	if os.Getenv("CXJ_BENCH_REPO") == "" {
		if sym := probe(benchfixture.EntrySymbol); sym != "" {
			return sym
		}
		b.Fatal("fixture entry symbol does not resolve: " + benchfixture.EntrySymbol)
	}
	// Real repo with no symbol given: find any controller-ish definition that
	// resolves. Search only matches exact/prefix/fuzzy, so try name prefixes.
	for _, q := range []string{"Controller", "Handler", "Service", "Resource"} {
		for _, r := range eng.Search(q, 50).Results {
			if sym := probe(r.Symbol); sym != "" {
				return sym
			}
		}
	}
	b.Fatal("no resolvable symbol; set CXJ_BENCH_SYMBOL=com.acme.Thing.method")
	return ""
}

// BenchmarkOpen is the floor every command pays: walk + parse + all sub-indexes.
func BenchmarkOpen(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Open(root, Options{Caps: CapsAll}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOpenMain is P1's claim: main scope skips the src/test parse work
// entirely, not just the indexing of it. CapsAll makes it the worst case at main
// scope, i.e. a command that reads every sub-index.
func BenchmarkOpenMain(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Open(root, Options{Caps: CapsAll, Scope: scan.ScopeMain}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBrief is the cheap orientation card (brief's capability set, full scope
// by design: it reports test-class counts).
func BenchmarkBrief(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng, err := Open(root, Options{Caps: CapsBrief})
		if err != nil {
			b.Fatal(err)
		}
		eng.Brief()
	}
}

// BenchmarkTrail is the hot navigational path end to end: main-scoped Open plus one
// symbol resolution, i.e. what `cxj trail` really costs an agent.
func BenchmarkTrail(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	sym := benchSymbol(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng, err := Open(root, Options{Caps: CapsFor("trail"), Scope: scan.ScopeMain})
		if err != nil {
			b.Fatal(err)
		}
		if eng.Trail(sym).Path == "" {
			b.Fatalf("trail missed %s", sym)
		}
	}
}

// BenchmarkCallees is the invocation-dense query: it walks the edge index, which is
// where the serial sort/merge fraction shows up.
func BenchmarkCallees(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	sym := benchSymbol(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng, err := Open(root, Options{Caps: CapsFor("callees"), Scope: scan.ScopeMain})
		if err != nil {
			b.Fatal(err)
		}
		eng.Callees(sym)
	}
}

// BenchmarkEndpoints is P2's claim: a spring-only query no longer pays for the Java
// parse, the binding parse, or the text index. Before P2 this opened everything.
func BenchmarkEndpoints(b *testing.B) {
	root := benchRoot(b)
	benchSize(b, root)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng, err := Open(root, Options{Caps: CapsFor("endpoints"), Scope: scan.ScopeMain})
		if err != nil {
			b.Fatal(err)
		}
		eng.Endpoints("", "")
	}
}
