package engine

import (
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/scan"
)

// Query-side benchmarks.
//
// Everything Track P has measured so far is Open, and that was right: statelessness means
// Open is paid once per command, so it dominated everything else. These measure the other
// half, which is the half that decides whether batch mode (P4) can work at all — one Open
// serving 20 lookups is only cheap if a lookup is cheap — and which is paid N times inside
// a single process instead of once per process.
//
// They also make the shape of the index visible: index.Build concatenates facts into flat
// slices and keeps no maps, so every query is a full pass over every definition and edge in
// the workspace. The counts are reported on the benchmark line so a per-query number cannot
// be read without its denominator, which is how the fixture hid this: the fixture matches a
// real repository in file count, not in members per file.
//
//	go test ./internal/engine -run '^$' -bench 'Query|Batch' -benchtime=20x
//	CXJ_BENCH_REPO=/path/to/repo CXJ_BENCH_SYMBOL=com.acme.Foo.bar go test ... -bench Query

type queryBench struct {
	eng         *Engine
	sym         string
	defs, edges int
	occs        int
	springBeans int
}

// openQueryBench opens once, outside the timed region. Setup that is not excluded with
// ResetTimer is amortised into the per-query number, and at the -benchtime the CI job uses
// (3x) the amortised setup *is* the number.
func openQueryBench(b *testing.B) queryBench {
	b.Helper()
	root := benchRoot(b)
	sym := benchSymbol(b, root)
	eng, err := Open(root, Options{Caps: CapsAll, Scope: scan.ScopeMain})
	if err != nil {
		b.Fatal(err)
	}
	qb := queryBench{eng: eng, sym: sym}
	for _, f := range eng.facts {
		qb.defs += len(f.Definitions)
		qb.edges += len(f.Edges)
		qb.occs += len(f.Occurrences)
	}
	return qb
}

// reportSizes puts the corpus on the benchmark line. Called by each benchmark rather than
// by openQueryBench because with sub-benchmarks only the leaf lines are printed, and metrics
// reported on the parent are lost.
//
// The order matters and is not obvious: ResetTimer "deletes user-reported metrics", so
// reporting before it silently loses the denominators — which is how this helper was first
// written, and the benchmark lines came out unreadable without complaint.
func (qb queryBench) reportSizes(b *testing.B) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(qb.defs), "defs")
	b.ReportMetric(float64(qb.edges), "edges")
	b.ReportMetric(float64(qb.occs), "occ")
}

func simpleName(sym string) string {
	return sym[strings.LastIndexByte(sym, '.')+1:]
}

// sink keeps results alive so the compiler cannot talk us out of the work.
var sink any

func BenchmarkSearchQuery(b *testing.B) {
	qb := openQueryBench(b)
	// Three shapes an agent sends: a member's simple name, a type suffix, a broad query.
	for _, q := range []string{simpleName(qb.sym), "Controller", "Service"} {
		b.Run(q, func(b *testing.B) {
			qb.reportSizes(b)
			for i := 0; i < b.N; i++ {
				res := qb.eng.Search(q, 20)
				if len(res.Results) == 0 {
					b.Fatalf("query %q matched nothing: a miss measures a different path", q)
				}
				sink = res
			}
		})
	}
}

func BenchmarkLookupAllQuery(b *testing.B) {
	qb := openQueryBench(b)
	name := simpleName(qb.sym)
	qb.reportSizes(b)
	for i := 0; i < b.N; i++ {
		sink = qb.eng.syms.LookupAll(name)
	}
}

func BenchmarkTrailQuery(b *testing.B) {
	qb := openQueryBench(b)
	qb.reportSizes(b)
	for i := 0; i < b.N; i++ {
		sink = qb.eng.Trail(qb.sym)
	}
}

// BenchmarkCallersQuery is the expensive shape. References walks the definition list for
// definition sites, the definition list again for overrides, and the edge list for call
// sites, and implementors() walks the type hierarchy to make the override pass possible:
// four full passes per lookup, deduplicated through a freshly allocated map.
func BenchmarkCallersQuery(b *testing.B) {
	qb := openQueryBench(b)
	qb.reportSizes(b)
	for i := 0; i < b.N; i++ {
		sink = qb.eng.Callers(qb.sym)
	}
}

func BenchmarkCalleesQuery(b *testing.B) {
	qb := openQueryBench(b)
	qb.reportSizes(b)
	for i := 0; i < b.N; i++ {
		sink = qb.eng.Callees(qb.sym)
	}
}

func BenchmarkMapQuery(b *testing.B) {
	qb := openQueryBench(b)
	qb.reportSizes(b)
	for i := 0; i < b.N; i++ {
		sink = qb.eng.Map()
	}
}

// BenchmarkBatch20Lookups is the §3.3 budget line "batch of 20 lookups (one Open)" and the
// exit criterion P4 claims, measured before P4 exists: one full Open plus 20 mixed lookups
// (5 queries x 4 operations). Open is inside the timed region on purpose; this one is the
// whole cost of a batch, not the marginal cost of a lookup.
func BenchmarkBatch20Lookups(b *testing.B) {
	root := benchRoot(b)
	sym := benchSymbol(b, root)
	benchSize(b, root)
	queries := batchLookups(sym)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng, err := Open(root, Options{Caps: CapsAll, Scope: scan.ScopeMain})
		if err != nil {
			b.Fatal(err)
		}
		hits := 0
		for _, q := range queries {
			hits += len(eng.Search(q, 20).Results)
			if eng.Trail(q).Path != "" {
				hits++
			}
			hits += len(eng.Callers(q).Results)
			hits += len(eng.Callees(q).Results)
		}
		if hits == 0 {
			b.Fatal("a 20-lookup batch found nothing: it is measuring a miss path")
		}
		sink = hits
	}
}

// batchLookups builds 5 symbol-shaped queries from one that is known to resolve, so the
// batch measures hits rather than a room full of misses: the qualified member, its owner
// qualified, the owner's simple name, the member's simple name, and a broad lexical query.
func batchLookups(sym string) []string {
	i := strings.LastIndexByte(sym, '.')
	owner := sym[:i]
	j := strings.LastIndexByte(owner, '.')
	return []string{sym, owner, owner[j+1:], sym[i+1:], "Controller"}
}
