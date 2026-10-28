// Package index provides in-RAM symbol and reference lookup over IR facts.
package index

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"example.com/cx/internal/ir"
)

// SymbolIndex holds definitions and candidate edges for lexical search.
type SymbolIndex struct {
	defs  []ir.Definition
	edges []ir.Edge
	occs  []ir.Occurrence
}

// Build constructs an index from extracted file facts.
func Build(files []ir.FileFacts) *SymbolIndex {
	idx := &SymbolIndex{}
	for _, f := range files {
		idx.defs = append(idx.defs, f.Definitions...)
		idx.edges = append(idx.edges, f.Edges...)
		idx.occs = append(idx.occs, f.Occurrences...)
	}
	return idx
}

// Search finds definitions matching q (exact preferred over prefix).
func (idx *SymbolIndex) Search(q string, limit int) []ir.SearchResult {
	q = strings.TrimSpace(q)
	if q == "" || limit <= 0 {
		return nil
	}
	type hit struct {
		def   ir.Definition
		match ir.MatchKind
		score float64
	}
	var hits []hit
	for _, d := range idx.defs {
		if m, score := matchDef(q, d); m != "" {
			hits = append(hits, hit{def: d, match: m, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].match != hits[j].match {
			return matchRank(hits[i].match) < matchRank(hits[j].match)
		}
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		a, b := hits[i].def, hits[j].def
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Span.Line != b.Span.Line {
			return a.Span.Line < b.Span.Line
		}
		return a.Qualified < b.Qualified
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]ir.SearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, ir.SearchResult{
			Path:    h.def.Path,
			Line:    h.def.Span.Line,
			Symbol:  h.def.Qualified,
			Kind:    kindOf(h.def.Kind),
			Match:   h.match,
			Score:   h.score,
			Snippet: h.def.Signature,
		})
	}
	return out
}

// References returns uses/relations for symbol without inventing exact dispatch.
func (idx *SymbolIndex) References(symbol string) []ir.RefResult {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil
	}
	simple := symbol
	if i := strings.LastIndexByte(symbol, '.'); i >= 0 {
		simple = symbol[i+1:]
	}
	owner := ""
	if i := strings.LastIndexByte(symbol, '.'); i >= 0 {
		owner = symbol[:i]
	}

	var out []ir.RefResult
	seen := map[string]struct{}{}
	add := func(r ir.RefResult) {
		key := r.Path + "|" + strconv.Itoa(r.Line) + "|" + r.Target + "|" + string(r.Certainty)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}

	// Definition sites of the queried symbol.
	for _, d := range idx.defs {
		if defMatchesSymbol(d, symbol, owner, simple) {
			add(ir.RefResult{
				Path:      d.Path,
				Line:      d.Span.Line,
				Target:    d.Qualified,
				Kind:      ir.RefKindDef,
				Certainty: ir.CertaintyExact,
			})
		}
	}

	// Override / implement sites: same simple method name under a type that
	// implements/extends the owner — always candidate, never exact.
	if owner != "" {
		implTypes := idx.implementors(owner)
		for _, d := range idx.defs {
			if d.Kind != ir.SymbolMethod || d.Name != simple {
				continue
			}
			if _, ok := implTypes[d.Owner]; ok {
				add(ir.RefResult{
					Path:      d.Path,
					Line:      d.Span.Line,
					Target:    d.Qualified,
					Kind:      ir.RefKindImpl,
					Certainty: ir.CertaintyCandidate,
				})
			}
		}
	}

	// Call sites: match by method simple name; certainty stays candidate/unresolved.
	for _, e := range idx.edges {
		if e.Kind != "calls" {
			continue
		}
		if !callMatches(e.To, symbol, simple) {
			continue
		}
		add(ir.RefResult{
			Path:      e.Path,
			Line:      e.Span.Line,
			Target:    symbol,
			Kind:      ir.RefKindUse,
			Certainty: ir.CertaintyCandidate,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// Callers returns only use sites for symbol (no declaration or implementor rows).
func (idx *SymbolIndex) Callers(symbol string) []ir.RefResult {
	var out []ir.RefResult
	for _, r := range idx.References(symbol) {
		if r.Kind == ir.RefKindUse {
			out = append(out, r)
		}
	}
	return out
}

// Callees returns outbound call sites from symbol (type or method).
func (idx *SymbolIndex) Callees(symbol string) []ir.RefResult {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil
	}
	simple := symbol
	if i := strings.LastIndexByte(symbol, '.'); i >= 0 {
		simple = symbol[i+1:]
	}
	var out []ir.RefResult
	seen := map[string]struct{}{}
	for _, e := range idx.edges {
		if e.Kind != "calls" {
			continue
		}
		if !fromMatches(e.From, symbol, simple) {
			continue
		}
		r := ir.RefResult{
			Path: e.Path, Line: e.Span.Line, Target: e.To,
			Kind: ir.RefKindUse, Certainty: ir.CertaintyCandidate,
		}
		key := r.Path + "|" + strconv.Itoa(r.Line) + "|" + r.Target
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func fromMatches(from, symbol, simple string) bool {
	if from == symbol || from == simple {
		return true
	}
	if strings.HasPrefix(from, symbol+".") || strings.HasPrefix(from, simple+".") {
		return true
	}
	if strings.HasSuffix(from, "."+simple) || strings.HasSuffix(from, "."+symbol) {
		return true
	}
	if i := strings.LastIndexByte(from, '.'); i >= 0 && from[i+1:] == simple {
		return true
	}
	return false
}

// Heritage returns extends/implements edges for symbol.
func (idx *SymbolIndex) Heritage(symbol string) (supers, subtypes []ir.Edge) {
	simple := symbol
	if i := strings.LastIndexByte(symbol, '.'); i >= 0 {
		simple = symbol[i+1:]
	}
	match := func(name string) bool {
		return name == symbol || name == simple || strings.HasSuffix(name, "."+simple) || strings.HasSuffix(name, "."+symbol)
	}
	for _, e := range idx.edges {
		if e.Kind != "extends" && e.Kind != "implements" {
			continue
		}
		if match(e.From) {
			supers = append(supers, e)
		}
		if match(e.To) {
			subtypes = append(subtypes, e)
		}
	}
	return supers, subtypes
}

// TestTypes returns type definitions in test files that cover a type, matched by
// type name (PaymentController -> PaymentControllerTest), never by substring.
// Callers must pass a *type* name: handing it a method name used to drag in every
// class whose name merely contained it.
func (idx *SymbolIndex) TestTypes(typeName string) []ir.Definition {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return nil
	}
	var out []ir.Definition
	for _, d := range idx.defs {
		switch d.Kind {
		case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
		default:
			continue
		}
		if !isTestFile(d.Path) {
			continue
		}
		if testTypeMatches(d.Name, typeName) {
			out = append(out, d)
		}
	}
	return out
}

// testTypeMatches accepts the naming conventions that actually exist: a test class
// for T is T<qualifier>Test (PaymentControllerTest, PaymentControllerValidationTest),
// or TestT. Two rules do the work:
//
//   - camel-hump boundary after the type name, so `Pay` matches PayTest but not
//     PayoutTest — "out" continues the same word rather than starting a new one.
//     Without this, a short type name re-acquires exactly the substring bug A1
//     removed.
//   - the candidate must actually look like a test (ends Test/Tests/IT/Spec), so a
//     plain collaborator named PaymentControllerFactory is not a test of it.
//
// Accepted soft overlap: `PayHandlerTest` also matches a type literally named `Pay`.
// The tests section is a capped hint, not a claim, so this errs toward showing
// something adjacent. Pinning it here so the tradeoff stays visible.
func testTypeMatches(candidate, typeName string) bool {
	if candidate == "" || typeName == "" {
		return false
	}
	if candidate == typeName {
		return true
	}
	testish := func(s string) bool {
		return strings.HasSuffix(s, "Test") || strings.HasSuffix(s, "Tests") ||
			strings.HasSuffix(s, "IT") || strings.HasSuffix(s, "Spec")
	}
	if strings.HasPrefix(candidate, typeName) && testish(candidate) {
		rest := candidate[len(typeName):]
		if rest == "" {
			return true
		}
		r, _ := utf8.DecodeRuneInString(rest)
		return unicode.IsUpper(r)
	}
	// TestPaymentController
	if strings.HasPrefix(candidate, "Test") && strings.HasSuffix(candidate, typeName) && candidate != typeName {
		return true
	}
	return false
}

func isTestFile(p string) bool {
	return strings.Contains(p, "/src/test/") || strings.HasPrefix(p, "src/test/") ||
		strings.HasSuffix(p, "Test.java") || strings.HasSuffix(p, "Tests.java")
}

// Lookup finds a definition by qualified or simple name. Types win over members.
func (idx *SymbolIndex) Lookup(symbol string) (ir.Definition, bool) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return ir.Definition{}, false
	}
	var named []ir.Definition
	for _, d := range idx.defs {
		if d.Qualified == symbol {
			return d, true
		}
		if d.Name == symbol || strings.HasSuffix(d.Qualified, "."+symbol) {
			named = append(named, d)
		}
	}
	if len(named) == 0 {
		return ir.Definition{}, false
	}
	for _, d := range named {
		switch d.Kind {
		case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
			return d, true
		}
	}
	return named[0], true
}

func (idx *SymbolIndex) implementors(iface string) map[string]struct{} {
	out := map[string]struct{}{}
	simple := iface
	if i := strings.LastIndexByte(iface, '.'); i >= 0 {
		simple = iface[i+1:]
	}
	for _, e := range idx.edges {
		if e.Kind != "implements" && e.Kind != "extends" {
			continue
		}
		if e.To == iface || e.To == simple || strings.HasSuffix(e.To, "."+simple) {
			out[e.From] = struct{}{}
		}
	}
	return out
}

func defMatchesSymbol(d ir.Definition, symbol, owner, simple string) bool {
	if d.Qualified == symbol || d.Name == symbol {
		return true
	}
	if owner != "" && d.Name == simple && (d.Owner == owner || strings.HasSuffix(d.Owner, "."+owner) || strings.HasSuffix(owner, "."+d.Owner) || d.Owner == trimPkg(owner)) {
		return true
	}
	// Action.run vs Action.run when owner is simple type name
	if owner != "" && d.Name == simple {
		o := d.Owner
		if i := strings.LastIndexByte(o, '.'); i >= 0 {
			o = o[i+1:]
		}
		if o == owner || o == trimPkg(owner) {
			return true
		}
	}
	return false
}

func trimPkg(q string) string {
	if i := strings.LastIndexByte(q, '.'); i >= 0 {
		return q[i+1:]
	}
	return q
}

func callMatches(to, symbol, simple string) bool {
	if to == symbol || to == simple {
		return true
	}
	if strings.HasSuffix(to, "."+simple) {
		return true
	}
	// receiver.simple vs Type.simple — name equality only
	if i := strings.LastIndexByte(to, '.'); i >= 0 && to[i+1:] == simple {
		return true
	}
	return false
}

func matchDef(q string, d ir.Definition) (ir.MatchKind, float64) {
	candidates := []string{d.Qualified, d.Name}
	if d.Owner != "" {
		candidates = append(candidates, d.Owner+"."+d.Name)
		if i := strings.LastIndexByte(d.Owner, '.'); i >= 0 {
			candidates = append(candidates, d.Owner[i+1:]+"."+d.Name)
		}
	}
	ql := strings.ToLower(q)
	for _, c := range candidates {
		if strings.EqualFold(c, q) {
			return ir.MatchExact, 1.0
		}
		if strings.HasSuffix(strings.ToLower(c), "."+ql) && !strings.Contains(ql, ".") {
			return ir.MatchExact, 0.95
		}
	}
	for _, c := range candidates {
		cl := strings.ToLower(c)
		if strings.HasPrefix(cl, ql) {
			return ir.MatchPrefix, float64(len(q)) / float64(len(c)+1)
		}
	}
	bestDist := 99
	for _, c := range candidates {
		if dist, ok := fuzzyQualified(q, c); ok && dist < bestDist {
			bestDist = dist
		}
	}
	if bestDist <= maxFuzzyDistance(q) {
		return ir.MatchFuzzy, 1.0 / float64(bestDist+2)
	}
	return "", 0
}

func maxFuzzyDistance(q string) int {
	n := len(q)
	switch {
	case n < 4:
		return 0
	case n < 12:
		return 1
	default:
		return 2
	}
}

// fuzzyQualified compares dotted identifiers segment-wise with bounded edit distance.
func fuzzyQualified(query, candidate string) (int, bool) {
	qs := strings.Split(query, ".")
	cs := strings.Split(candidate, ".")
	if len(qs) > len(cs) {
		return 0, false
	}
	// Align query segments to the trailing end of candidate (pkg.Owner.method).
	offset := len(cs) - len(qs)
	total := 0
	for i, qseg := range qs {
		cseg := cs[offset+i]
		if strings.EqualFold(qseg, cseg) {
			continue
		}
		d := levenshtein(strings.ToLower(qseg), strings.ToLower(cseg), 2)
		if d < 0 {
			return 0, false
		}
		total += d
	}
	return total, true
}

// levenshtein returns edit distance or -1 if it would exceed limit.
func levenshtein(a, b string, limit int) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if abs(la-lb) > limit {
		return -1
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min(del, ins, sub)
			if cur[j] < rowMin {
				rowMin = cur[j]
			}
		}
		if rowMin > limit {
			return -1
		}
		prev, cur = cur, prev
	}
	if prev[lb] > limit {
		return -1
	}
	return prev[lb]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func matchRank(m ir.MatchKind) int {
	switch m {
	case ir.MatchExact:
		return 0
	case ir.MatchPrefix:
		return 1
	case ir.MatchFuzzy:
		return 2
	default:
		return 9
	}
}

func kindOf(k ir.SymbolKind) ir.ResultKind {
	switch k {
	case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
		return ir.KindClass
	case ir.SymbolMethod, ir.SymbolConstructor:
		return ir.KindMethod
	case ir.SymbolField:
		return ir.KindField
	default:
		return ir.KindText
	}
}
