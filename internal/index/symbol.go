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

// Search finds definitions matching q, ranked by how good the match is:
// exact, then prefix, then fuzzy, then substring/camel-hump (see substringMatch).
// A capitalized query additionally prefers types over members (see ir.LexicalLess).
func (idx *SymbolIndex) Search(q string, limit int) []ir.SearchResult {
	q = strings.TrimSpace(q)
	if q == "" || limit <= 0 {
		return nil
	}
	hits := make([]ir.SearchResult, 0, 32)
	for _, d := range idx.defs {
		m, score := matchDef(q, d)
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

// fromMatches decides whether a call edge belongs to the queried symbol.
//
// Edge.From is always the fully-qualified enclosing method, so a qualified query
// needs only two rules: it is that method, or it is inside that type. The old
// version also accepted any owner *ending with* the queried simple name, which is
// what made `callees …PayHandler.handle` return 292 results from 20 files: every
// `handle` in the repo qualified. A bare, unqualified name still matches by simple
// name, because the user genuinely asked for "anything called handle" — the engine
// flags that case as ambiguous rather than pretending it is precise.
func fromMatches(from, symbol, simple string) bool {
	if from == symbol || from == simple {
		return true
	}
	if strings.Contains(symbol, ".") {
		return strings.HasPrefix(from, symbol+".")
	}
	if i := strings.LastIndexByte(from, '.'); i >= 0 {
		return from[i+1:] == simple
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
// When a name is owned by several types this returns the first one it finds, which
// is a guess; callers that can report uncertainty should use LookupAll instead and
// surface the alternatives rather than silently picking one.
func (idx *SymbolIndex) Lookup(symbol string) (ir.Definition, bool) {
	all := idx.LookupAll(symbol)
	if len(all) == 0 {
		return ir.Definition{}, false
	}
	return all[0], true
}

// LookupAll returns every definition a name could refer to, best guess first:
// an exact qualified match alone, otherwise types before members, then ordered by
// qualified name so results are stable. len(result) > 1 means the query is
// ambiguous, which is information the caller should show rather than hide.
func (idx *SymbolIndex) LookupAll(symbol string) []ir.Definition {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil
	}
	var exact, named []ir.Definition
	for _, d := range idx.defs {
		if d.Qualified == symbol {
			exact = append(exact, d)
			continue
		}
		if d.Name == symbol || strings.HasSuffix(d.Qualified, "."+symbol) {
			named = append(named, d)
		}
	}
	if len(exact) == 1 {
		return exact
	}
	cands := named
	if len(exact) > 1 {
		cands = exact
	} else if len(named) == 0 {
		return nil
	}
	out := make([]ir.Definition, len(cands))
	copy(out, cands)
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := isTypeKind(out[i].Kind), isTypeKind(out[j].Kind)
		if ti != tj {
			return ti
		}
		if out[i].Qualified != out[j].Qualified {
			return out[i].Qualified < out[j].Qualified
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func isTypeKind(k ir.SymbolKind) bool {
	switch k {
	case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
		return true
	}
	return false
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
	// Last resort, so a query that names the middle of a symbol still finds it:
	// `find Controller` matched nothing at all before this tier existed, because
	// nothing is allowed to start with "Controller". It ranks below every other
	// tier, so it can fill a gap but never overtake a better hit.
	if score, ok := substringMatch(q, d.Name); ok {
		return ir.MatchSubstring, score
	}
	return "", 0
}

// substringMatch matches a query inside a symbol's *simple* name, by plain
// substring or by camel humps. The simple name is the whole rule: matching the
// qualified name here would re-create the worst bug in the tool's history, where
// a query for `pay` hit the package `paymentapi` and matched the entire repo.
func substringMatch(q, name string) (float64, bool) {
	if q == "" || name == "" {
		return 0, false
	}
	// The length floor only applies to plain substrings: two letters in the middle
	// of a word match a large fraction of any repo, whereas two humps is already a
	// specific thing to ask for.
	if len(q) >= minSubstringLen {
		if i := strings.Index(strings.ToLower(name), strings.ToLower(q)); i >= 0 {
			return substringScore(len(q), i, len(name)), true
		}
	}
	if i, ok := camelHump(q, name); ok {
		return substringScore(len(q), i, len(name)), true
	}
	return 0, false
}

const minSubstringLen = 3

// substringScore prefers shorter names and earlier matches, which is what makes a
// good first hit: OrderHandler before OrderHandlerV2 before ShippingHandler.
func substringScore(qlen, at, nameLen int) float64 {
	return float64(qlen) / float64(nameLen+at+1)
}

// camelHump reports whether q reads as the humps of name, one letter per hump:
// OH → OrderHandler, OHV → OrderHandlerV2, but not OR → OrderHandler.
//
// One letter per hump is deliberate. The alternative (letting the tail of the
// query prefix-match the last hump) looks better on paper and cannot be
// explained: the plan's own example, PHCtrl → PaymentHandlerController, does not
// even work under it, because "ctrl" is not a prefix of "controller".
func camelHump(q, name string) (int, bool) {
	if len(q) < 2 {
		return 0, false
	}
	qi, last := 0, -1
	for i := 0; i < len(name) && qi < len(q); i++ {
		if !isHumpStart(name, i) {
			continue
		}
		if foldByte(name[i]) != foldByte(q[qi]) {
			return 0, false // humps must line up in order; skipping would be a guess
		}
		last = i
		qi++
	}
	return last, qi == len(q)
}

func isHumpStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	prev, c := s[i-1], s[i]
	switch {
	case c == '_':
		return true
	case isUpperASCII(c) && !isUpperASCII(prev):
		return true
	// HTTPServer: the last capital of a run starts the next hump.
	case isUpperASCII(c) && isUpperASCII(prev) && i+1 < len(s) && isLowerASCII(s[i+1]):
		return true
	}
	return false
}

func isUpperASCII(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLowerASCII(c byte) bool { return c >= 'a' && c <= 'z' }
func foldByte(c byte) byte {
	if isUpperASCII(c) {
		return c + ('a' - 'A')
	}
	return c
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

func matchRank(m ir.MatchKind) int { return ir.RankMatch(m) }

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
