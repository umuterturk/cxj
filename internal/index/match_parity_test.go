package index

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/benchfixture"
	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/java"
)

// P7 rewrote the definition matcher so that ranking a definition that cannot match costs
// nearly nothing: a `find` was spending 4.7 ms and 74,000 allocations to print twenty
// lines, because every definition paid for a lowered query, a split copy of itself, and
// two concatenated spellings.
//
// A rewrite of a matcher is only acceptable if it changes cost and nothing else, so this
// file makes that checkable three ways rather than asserting it:
//
//   - differentially, against `matchDefLegacy`, the previous implementation kept in
//     production as the non-ASCII path, over every definition in three corpora and a
//     query battery generated from the corpora themselves;
//   - absolutely, against literal expected tiers and scores, so the fast path and the
//     legacy path cannot drift together and stay green;
//   - by routing, so a definition with a non-ASCII identifier is known to take the legacy
//     path rather than the byte-folding one it is not proven equivalent for.
//
// Run the real-repository corpus with:
//
//	CXJ_JAVA_DIR=/path/to/repo/src go test ./internal/index -run TestMatchDef -v

type matchVerdict struct {
	kind  ir.MatchKind
	score float64
}

func TestMatchDefMatchesLegacy(t *testing.T) {
	corpus := 0
	for _, c := range matchCorpora(t) {
		idx := c.build(t)
		if len(idx.defs) == 0 {
			t.Fatalf("%s: corpus produced no definitions", c.name)
		}
		queries := matchBattery(idx.defs)
		defs, sampled := sampleDefs(idx.defs, 5)
		checked := 0
		for _, q := range queries {
			p := newMatchPlan(q)
			for _, ds := range defs {
				d := ds.def
				// The ASCII decision is made the same way Search makes it: the query
				// licenses byte folding and the definition's flag permits it.
				fastKind, fastScore := matchDef(p, d, p.ascii && idx.ascii.has(ds.i))
				legKind, legScore := matchDefLegacy(q, d)
				checked++
				fast, leg := matchVerdict{fastKind, fastScore}, matchVerdict{legKind, legScore}
				if fast != leg {
					t.Errorf("%s: query %q on %s (kind %q owner %q): fast %s/%v, legacy %s/%v",
						c.name, q, d.Qualified, d.Kind, d.Owner, fast.kind, fast.score, leg.kind, leg.score)
					break
				}
			}
		}
		t.Logf("%s: %d comparisons (%d queries x %d of %d definitions)", c.name, checked, len(queries), len(defs), len(idx.defs))
		if sampled {
			t.Logf("%s: definitions sampled every 5th; the full corpus is not searched in CI", c.name)
		}
		corpus += checked
	}
	if corpus < 250_000 {
		t.Errorf("only %d comparisons: too few to claim the paths agree", corpus)
	}
}

// pin is one expected Search result: what the user is shown, in the order shown.
type pin struct {
	symbol string
	kind   ir.MatchKind
	score  float64
}

// TestSearchPinnedRanking is the absolute check: expected hits with tiers and scores
// written into this file, not computed from a second implementation. If both matchers
// were ever changed together to be wrong in the same way, the differential test above
// stays green and this one does not.
//
// The table was generated once by ranking the golden fixture with the pre-P7 matcher and
// pasted in as source; each row was then checked against the tier rules by hand. To see
// the rules do the work for a new case, rank the fixture with `searchWithLegacy`.
func TestSearchPinnedRanking(t *testing.T) {
	idx := buildGoldenIndex(t)
	cases := []struct {
		q    string
		want []pin
	}{
		{"pay", []pin{
			{"com.example.paymentapi.PayService.pay", ir.MatchExact, 0.95},
			{"com.example.paymentapi.PaymentController.pay", ir.MatchExact, 0.95},
			{"com.example.paymentapi.PayService", ir.MatchPrefix, 0.2727272727272727},
			{"com.example.paymentapi.PaymentController.payService", ir.MatchPrefix, 0.2727272727272727},
			{"com.example.pay.OrderHandler.payRepository", ir.MatchPrefix, 0.21428571428571427},
			{"com.example.paymentapi.PaymentController", ir.MatchPrefix, 0.16666666666666666},
			{"com.example.paymentapi.PaymentController.PaymentController", ir.MatchPrefix, 0.16666666666666666},
			{"com.example.paymentapi.PaymentController.audit", ir.MatchPrefix, 0.125},
			{"com.example.paymentapi.PaymentControllerTest.pays", ir.MatchPrefix, 0.6},
			{"com.example.paymentapi.PayoutTest", ir.MatchPrefix, 0.2727272727272727},
			{"com.example.paymentapi.PaymentControllerTest", ir.MatchPrefix, 0.13636363636363635},
			{"com.example.paymentapi.PayoutTest.checkPayout", ir.MatchPrefix, 0.13043478260869565},
		}},
		{"handle", []pin{
			{"com.example.pay.OrderHandler.handle", ir.MatchExact, 0.95},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchExact, 0.95},
			{"com.example.pay.ShippingHandler.handle", ir.MatchExact, 0.95},
			{"com.example.pay.BackofficeHarness.handler", ir.MatchPrefix, 0.75},
			{"com.example.pay.OrderHandler", ir.MatchSubstring, 0.3333333333333333},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchSubstring, 0.3333333333333333},
			{"com.example.pay.OrderHandlerV2", ir.MatchSubstring, 0.3},
			{"com.example.pay.BackofficeHarness.shippingHandler", ir.MatchSubstring, 0.25},
			{"com.example.pay.ShippingHandler", ir.MatchSubstring, 0.25},
		}},
		{"handler", []pin{
			{"com.example.pay.BackofficeHarness.handler", ir.MatchExact, 0.95},
			{"com.example.pay.OrderHandler.handle", ir.MatchFuzzy, 0.3333333333333333},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchFuzzy, 0.3333333333333333},
			{"com.example.pay.ShippingHandler.handle", ir.MatchFuzzy, 0.3333333333333333},
			{"com.example.pay.OrderHandler", ir.MatchSubstring, 0.3888888888888889},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchSubstring, 0.3888888888888889},
			{"com.example.pay.OrderHandlerV2", ir.MatchSubstring, 0.35},
			{"com.example.pay.BackofficeHarness.shippingHandler", ir.MatchSubstring, 0.2916666666666667},
			{"com.example.pay.ShippingHandler", ir.MatchSubstring, 0.2916666666666667},
		}},
		{"handleOrder", nil},
		{"OrderHandler", []pin{
			{"com.example.pay.OrderHandler", ir.MatchExact, 0.95},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchExact, 0.95},
			{"com.example.pay.OrderHandlerV2", ir.MatchPrefix, 0.8},
			{"com.example.pay.OrderHandler.handle", ir.MatchPrefix, 0.6},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchPrefix, 0.5454545454545454},
			{"com.example.pay.OrderHandlerV2.legacy", ir.MatchPrefix, 0.5454545454545454},
			{"com.example.pay.OrderHandler.normalize", ir.MatchPrefix, 0.5217391304347826},
			{"com.example.pay.OrderHandler.payRepository", ir.MatchPrefix, 0.4444444444444444},
		}},
		{"ord", []pin{
			{"com.example.pay.OrderHandler", ir.MatchPrefix, 0.23076923076923078},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchPrefix, 0.23076923076923078},
			{"com.example.pay.OrderHandlerV2", ir.MatchPrefix, 0.2},
			{"com.example.pay.OrderRepository", ir.MatchPrefix, 0.1875},
			{"com.example.pay.OrderHandler.handle", ir.MatchPrefix, 0.15},
			{"com.example.pay.OrderRepository.save", ir.MatchPrefix, 0.14285714285714285},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchPrefix, 0.13636363636363635},
			{"com.example.pay.OrderHandlerV2.legacy", ir.MatchPrefix, 0.13636363636363635},
			{"com.example.pay.OrderHandler.normalize", ir.MatchPrefix, 0.13043478260869565},
			{"com.example.pay.OrderHandler.payRepository", ir.MatchPrefix, 0.1111111111111111},
		}},
		{"Order", []pin{
			{"com.example.pay.OrderHandler", ir.MatchPrefix, 0.38461538461538464},
			{"com.example.pay.OrderHandlerV2", ir.MatchPrefix, 0.3333333333333333},
			{"com.example.pay.OrderRepository", ir.MatchPrefix, 0.3125},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchPrefix, 0.38461538461538464},
			{"com.example.pay.OrderHandler.handle", ir.MatchPrefix, 0.25},
			{"com.example.pay.OrderRepository.save", ir.MatchPrefix, 0.23809523809523808},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchPrefix, 0.22727272727272727},
			{"com.example.pay.OrderHandlerV2.legacy", ir.MatchPrefix, 0.22727272727272727},
			{"com.example.pay.OrderHandler.normalize", ir.MatchPrefix, 0.21739130434782608},
			{"com.example.pay.OrderHandler.payRepository", ir.MatchPrefix, 0.18518518518518517},
		}},
		{"OH", []pin{
			{"com.example.pay.OrderHandler", ir.MatchSubstring, 0.1111111111111111},
			{"com.example.pay.OrderHandlerV2", ir.MatchSubstring, 0.1},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchSubstring, 0.1111111111111111},
		}},
		{"oh", []pin{
			{"com.example.pay.OrderHandler", ir.MatchSubstring, 0.1111111111111111},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchSubstring, 0.1111111111111111},
			{"com.example.pay.OrderHandlerV2", ir.MatchSubstring, 0.1},
		}},
		{"v2", nil},
		{"V2", nil},
		{"OrderHandlerV2", []pin{
			{"com.example.pay.OrderHandlerV2", ir.MatchExact, 0.95},
			{"com.example.pay.OrderHandler", ir.MatchFuzzy, 0.25},
			{"com.example.pay.OrderHandler.OrderHandler", ir.MatchFuzzy, 0.25},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchPrefix, 0.6363636363636364},
			{"com.example.pay.OrderHandlerV2.legacy", ir.MatchPrefix, 0.6363636363636364},
		}},
		{"repository", []pin{
			{"com.example.pay.OrderHandler.payRepository", ir.MatchSubstring, 0.5882352941176471},
			{"com.example.pay.OrderRepository", ir.MatchSubstring, 0.47619047619047616},
		}},
		{"Repository", []pin{
			{"com.example.pay.OrderRepository", ir.MatchSubstring, 0.47619047619047616},
			{"com.example.pay.OrderHandler.payRepository", ir.MatchSubstring, 0.5882352941176471},
		}},
		{"controller", []pin{
			{"com.example.paymentapi.PaymentController", ir.MatchSubstring, 0.4},
			{"com.example.paymentapi.PaymentController.PaymentController", ir.MatchSubstring, 0.4},
			{"com.example.paymentapi.PaymentControllerTest", ir.MatchSubstring, 0.3448275862068966},
		}},
		{"Controller", []pin{
			{"com.example.paymentapi.PaymentController", ir.MatchSubstring, 0.4},
			{"com.example.paymentapi.PaymentController.PaymentController", ir.MatchSubstring, 0.4},
			{"com.example.paymentapi.PaymentControllerTest", ir.MatchSubstring, 0.3448275862068966},
		}},
		{"paymentapi", nil},
		{"com.example.paymentapi", []pin{
			{"com.example.paymentapi.PayService", ir.MatchPrefix, 0.6470588235294118},
			{"com.example.paymentapi.PayService.pay", ir.MatchPrefix, 0.5789473684210527},
			{"com.example.paymentapi.PaymentController", ir.MatchPrefix, 0.5365853658536586},
			{"com.example.paymentapi.PaymentController.pay", ir.MatchPrefix, 0.4888888888888889},
			{"com.example.paymentapi.PaymentController.audit", ir.MatchPrefix, 0.46808510638297873},
			{"com.example.paymentapi.PaymentController.payService", ir.MatchPrefix, 0.4230769230769231},
			{"com.example.paymentapi.PaymentController.PaymentController", ir.MatchPrefix, 0.3728813559322034},
			{"com.example.paymentapi.PayoutTest", ir.MatchPrefix, 0.6470588235294118},
			{"com.example.paymentapi.PaymentControllerTest", ir.MatchPrefix, 0.4888888888888889},
			{"com.example.paymentapi.PayoutTest.checkPayout", ir.MatchPrefix, 0.4782608695652174},
			{"com.example.paymentapi.PaymentControllerTest.pays", ir.MatchPrefix, 0.44},
		}},
		{"PaymentController.pay", []pin{
			{"com.example.paymentapi.PaymentController.pay", ir.MatchExact, 1},
			{"com.example.paymentapi.PaymentController.payService", ir.MatchPrefix, 0.7241379310344828},
			{"com.example.paymentapi.PaymentController.PaymentController", ir.MatchPrefix, 0.5833333333333334},
		}},
		{"OrderHandler.handle", []pin{
			{"com.example.pay.OrderHandler.handle", ir.MatchExact, 1},
			{"com.example.pay.OrderHandlerV2.handle", ir.MatchFuzzy, 0.25},
		}},
		{"Tiny", nil},
		{"missing", nil},
		{"ay", nil},
		{"getById", nil},
		{"IDLE", nil},
		{"e", nil},
	}
	for _, c := range cases {
		got := idx.Search(c.q, 20)
		if len(got) != len(c.want) {
			t.Errorf("query %q: %d hits, want %d", c.q, len(got), len(c.want))
			for i := range got {
				t.Logf("  got  %d %s %s %v", i, got[i].Symbol, got[i].Match, got[i].Score)
			}
			continue
		}
		for i := range got {
			if got[i].Symbol != c.want[i].symbol || got[i].Match != c.want[i].kind || got[i].Score != c.want[i].score {
				t.Errorf("query %q rank %d: %s/%s/%v, pinned %s/%s/%v",
					c.q, i, got[i].Symbol, got[i].Match, got[i].Score,
					c.want[i].symbol, c.want[i].kind, c.want[i].score)
			}
		}
	}
}

// TestSearchCappedMatchesFullSort checks the end-to-end answer, not just the per-def
// verdict: Search keeps only `limit` results while scanning, so it must reproduce the
// full collect-sort-truncate pipeline exactly — including tie order, which is where a
// bounded top-k usually drifts. LexicalLess produces many genuine ties (same kind and
// score), so the fixture is a real stress test rather than a formality.
func TestSearchCappedMatchesFullSort(t *testing.T) {
	for _, c := range matchCorpora(t) {
		idx := c.build(t)
		all := matchBattery(idx.defs)
		queries := sampleQueries(all, 64)
		for _, q := range queries {
			for _, limit := range []int{1, 3, 20} {
				want := searchWithLegacy(idx, q, limit)
				got := idx.Search(q, limit)
				if len(got) != len(want) {
					t.Fatalf("%s: query %q limit %d: %d hits, want %d", c.name, q, limit, len(got), len(want))
				}
				for i := range got {
					if got[i] != want[i] {
						t.Errorf("%s: query %q limit %d rank %d: %+v, want %+v",
							c.name, q, limit, i, got[i], want[i])
					}
				}
			}
		}
	}
}

// TestNonASCIIDefinitionsTakeTheLegacyPath guards the one assumption the fast path cannot
// prove locally: byte folding equals strings.ToLower only while the bytes are ASCII.
// ToLower is not a byte map, so a non-ASCII identifier must never be ranked by folding.
func TestNonASCIIDefinitionsTakeTheLegacyPath(t *testing.T) {
	sources := map[string]string{
		"a/Unicorn.java": "package a;\n\npublic class Unicorn {\n    String name = \"x\";\n    void payNormal() {}\n}\n",
		"a/Turkish.java": "package a;\n\npublic class Ünicorn {\n    void pay() {}\n    void ödeme() {}\n}\n",
		"a/Greek.java":   "package a;\n\npublic class Signal {\n    void γειά() {}\n}\n",
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	ex := java.NewExtractor()
	defer ex.Close()
	facts := make([]ir.FileFacts, 0, len(names))
	for _, name := range names {
		f, err := ex.Extract(name, []byte(sources[name]))
		if err != nil {
			t.Fatalf("extract %s: %v", name, err)
		}
		facts = append(facts, f)
	}
	idx := Build(facts)

	asciiCount, nonASCII := 0, 0
	for i, d := range idx.defs {
		if idx.ascii.has(i) != defIsASCII(d) {
			t.Errorf("definition %d (%s): ascii flag %v, truth %v", i, d.Qualified, idx.ascii.has(i), defIsASCII(d))
		}
		if defIsASCII(d) {
			asciiCount++
		} else {
			nonASCII++
		}
	}
	if nonASCII == 0 {
		t.Fatal("the fixture produced no non-ASCII definitions, so the routing claim is untested")
	}
	if asciiCount == 0 {
		t.Fatal("the fixture produced no ASCII definitions alongside them")
	}

	// Whatever the routing decision, the answer must be the one the original gave.
	for _, q := range []string{"Unicorn", "Ünicorn", "pay", "ödeme", "ödem", "γειά", "name", "payNormal", "unicorn", "ünicorn", "ÜNICORN"} {
		want := searchWithLegacy(idx, q, 20)
		got := idx.Search(q, 20)
		if len(got) != len(want) {
			t.Fatalf("query %q: %d hits, legacy %d", q, len(got), len(want))
		}
		for i := range got {
			if got[i].Symbol != want[i].Symbol || got[i].Match != want[i].Match || got[i].Score != want[i].Score {
				t.Errorf("query %q hit %d: %s/%s/%v, legacy %s/%s/%v",
					q, i, got[i].Symbol, got[i].Match, got[i].Score, want[i].Symbol, want[i].Match, want[i].Score)
			}
		}
	}
}

// --- corpus and battery ------------------------------------------------------

type matchCorpus struct {
	name  string
	build func(t *testing.T) *SymbolIndex
}

// matchCorpora returns the corpora the two matchers are compared over. The golden fixture
// is the one with hand-written expectations about collisions; the synthetic fixture is
// the one CI can grow cheaply; a real repository is opt-in because it is the only corpus
// with the odd identifiers a generator would never produce:
//
//	CXJ_JAVA_DIR=/path/to/repo/src go test ./internal/index -run TestMatchDef -v
func matchCorpora(t *testing.T) []matchCorpus {
	t.Helper()
	out := []matchCorpus{
		{name: "golden", build: func(t *testing.T) *SymbolIndex { return buildGoldenIndex(t) }},
	}
	if dir := os.Getenv("CXJ_JAVA_DIR"); dir != "" {
		out = append(out, matchCorpus{name: "real:" + dir, build: javaTreeCorpus(dir)})
	} else {
		out = append(out, matchCorpus{name: "bench fixture", build: fixtureCorpus(8)})
	}
	return out
}

func fixtureCorpus(modules int) func(*testing.T) *SymbolIndex {
	return func(t *testing.T) *SymbolIndex {
		t.Helper()
		dir := t.TempDir()
		if _, err := benchfixture.Write(dir, benchfixture.Options{Modules: modules}); err != nil {
			t.Fatal(err)
		}
		return javaTreeCorpus(dir)(t)
	}
}

// javaTreeCorpus indexes a real directory tree with the real extractor, which is the
// only way a corpus can disagree with the fast path for the reasons it really would.
// The file cap keeps a laptop-verified corpus bounded; it is reported, not hidden.
func javaTreeCorpus(root string) func(*testing.T) *SymbolIndex {
	return func(t *testing.T) *SymbolIndex {
		t.Helper()
		if _, err := os.ReadDir(root); err != nil {
			t.Fatalf("cannot read %s: %v", root, err)
		}
		ex := java.NewExtractor()
		defer ex.Close()
		var facts []ir.FileFacts
		seen := 0
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".java") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			f, err := ex.Extract(p, data)
			if err != nil {
				return nil
			}
			facts = append(facts, f)
			seen++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if seen == 0 {
			t.Fatalf("no java files under %s", root)
		}
		idx := Build(facts)
		t.Logf("%s: indexed %d java files, %d definitions", root, seen, len(idx.defs))
		return idx
	}
}

// sampledDef keeps a definition together with its position, because the ASCII decision
// Search makes is per position, not per content.
type sampledDef struct {
	i   int
	def ir.Definition
}

// sampleDefs takes every stride-th definition. A full sweep is what CI cannot afford;
// the stride still reaches every file, because definitions arrive grouped by file.
func sampleDefs(defs []ir.Definition, stride int) ([]sampledDef, bool) {
	if len(defs) <= 600 {
		out := make([]sampledDef, len(defs))
		for i, d := range defs {
			out[i] = sampledDef{i: i, def: d}
		}
		return out, false
	}
	out := make([]sampledDef, 0, len(defs)/stride+1)
	for i := 0; i < len(defs); i += stride {
		out = append(out, sampledDef{i: i, def: defs[i]})
	}
	return out, true
}

// matchBattery builds queries from the corpus rather than inventing them, because the
// interesting questions are "does the misspelling of *this* name still rank the same",
// and a hand-written battery stops being collision-shaped the moment the corpus changes.
func matchBattery(defs []ir.Definition) []string {
	queries := []string{
		"", " ", ".", "..", "a", "A", "com", "com.example", "get", "pay", "Pay", "PAY",
		"pAy", "OH", "XYZ", "Controller", "controller", "CONTROLLER", "_", "$", "0",
		"ı", "İ", "Ünicode", "é", "ö", "com.ex.Tiny.missing", "..pay", "pay..",
	}
	step := len(defs)/40 + 1
	for i := 0; i < len(defs); i += step {
		queries = append(queries, nameVariants(defs[i])...)
	}
	return dedupe(queries)
}

// nameVariants are the shapes each tier is actually reached by: whole names, dotted
// names, owners, truncations, one-letter mutations, case flips and hump initials.
func nameVariants(d ir.Definition) []string {
	out := []string{d.Name, d.Qualified, d.Owner}
	n := d.Name
	if n != "" {
		out = append(out, strings.ToUpper(n), strings.ToLower(n))
		if len(n) > 2 {
			out = append(out, n[:len(n)-1], n[:1]+"z"+n[2:], n[1:])
		}
		if humps := humpInitials(n); humps != "" {
			out = append(out, humps, strings.ToLower(humps))
		}
	}
	if d.Owner != "" {
		if i := strings.LastIndexByte(d.Owner, '.'); i >= 0 {
			out = append(out, d.Owner[i+1:]+"."+d.Name)
		}
	}
	return out
}

func humpInitials(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if isUpperASCII(name[i]) && (i == 0 || !isUpperASCII(name[i-1])) {
			b.WriteByte(name[i])
		}
	}
	return b.String()
}

func defIsASCII(d ir.Definition) bool {
	return isASCII(d.Qualified) && isASCII(d.Name) && isASCII(d.Owner)
}

// searchWithLegacy ranks the whole corpus with the legacy matcher and then sorts and
// truncates the slow obvious way: collect everything, sort.SliceStable, cut. It is the
// reference the capped top-hits lists are required to reproduce exactly, so it must not
// share any truncation logic with them.
func searchWithLegacy(idx *SymbolIndex, q string, limit int) []ir.SearchResult {
	q = strings.TrimSpace(q)
	if q == "" || limit <= 0 {
		return nil
	}
	p := newMatchPlan(q)
	var hits []ir.SearchResult
	for _, d := range idx.defs {
		m, score := matchDefLegacy(p.q, d)
		if m == "" {
			continue
		}
		hits = append(hits, ir.SearchResult{
			Path:    d.Path,
			Line:    d.Span.Line,
			Symbol:  d.Qualified,
			Kind:    kindOf(d.Kind),
			Match:   m,
			Score:   score,
			Snippet: d.Signature,
		})
	}
	wantsType := ir.WantsTypeName(q)
	sort.SliceStable(hits, func(i, j int) bool { return ir.LexicalLess(hits[i], hits[j], wantsType) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// TestFoldIndexEquivalence locks foldIndex against its definition —
// strings.Index(ToLower(s), lower) — over deterministic pseudo-random ASCII strings.
// The differential parity test reaches it through matchDef, which only ever queries
// with real name shapes; this one hits the byte-level edges (needle at the very end,
// repeated near-misses, needle longer than haystack) that names rarely produce.
func TestFoldIndexEquivalence(t *testing.T) {
	alphabet := "abcdefghijABCDEFGHIJ._01"
	rnd := rand.New(rand.NewSource(7))
	needles := []string{"a", "j", "ab", "ja", "abc", "z", ".", "_", "0", "01", "hai", "j01",
		"abcdefghij", "ABCDEFGHI", "a.a", "a_a", "j.j", "0.1", "haihai"}
	for n := 0; n < 4000; n++ {
		s := make([]byte, rnd.Intn(40))
		for i := range s {
			s[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		for _, q := range needles {
			lower := strings.ToLower(q)
			want := strings.Index(strings.ToLower(string(s)), lower)
			got := foldIndex(string(s), lower)
			if got != want {
				t.Fatalf("foldIndex(%q, %q) = %d, want %d", s, lower, got, want)
			}
		}
	}
}

// sampleQueries thins a battery to at most n entries, keeping every entry when it is
// already small and otherwise keeping the first and last so hand-written edge cases at
// both ends of the list survive the thinning.
func sampleQueries(qs []string, n int) []string {
	if len(qs) <= n {
		return qs
	}
	out := make([]string, 0, n)
	stride := len(qs) / n
	for i := 0; i < len(qs); i += stride {
		out = append(out, qs[i])
	}
	out[len(out)-1] = qs[len(qs)-1]
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
