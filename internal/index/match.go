package index

import (
	"strings"

	"example.com/cx/internal/ir"
)

// The definition matcher.
//
// The ranking rules here are the ones A6/A7 established and the user sees: exact, then
// prefix, then fuzzy, then substring/camel-hump. This file is about paying for them
// without paying per definition. Before P7 a `find` on a 2,593-definition repository cost
// 4.7 ms and 74,000 allocations to print twenty lines, because every definition — not
// just the ones that could match — built a lowered copy of the query, a split copy of
// itself, and two concatenated spellings. Roughly 99 % of definitions match nothing, and
// they were billed the same as the ones that did.
//
// So the rules are unchanged and the byte traffic is not. `matchDefLegacy` is the
// previous implementation kept verbatim; it is what every non-ASCII definition still
// runs, and `match_parity_test.go` holds a third copy that the fast path is required to
// agree with over every definition in the fixture and in a real repository.

// matchPlan is the per-query work that must not happen per definition.
type matchPlan struct {
	q       string   // the query, trimmed, as typed
	ql      string   // strings.ToLower(q): the fold every tier compares against
	segs    []string // strings.Split(q, ".") as typed, for EqualFold
	maxDist int      // maxFuzzyDistance(q)
	noDot   bool     // ql has no '.': gates the dotted-tail tier
	ascii   bool     // q is pure ASCII, which licenses byte folding
}

func newMatchPlan(q string) matchPlan {
	ql := strings.ToLower(q)
	return matchPlan{
		q:       q,
		ql:      ql,
		segs:    strings.Split(q, "."),
		maxDist: maxFuzzyDistance(q),
		noDot:   !strings.Contains(ql, "."),
		ascii:   isASCII(q),
	}
}

// matchDef ranks one definition against the query.
//
// The tier order and the per-candidate order inside each tier are load-bearing: tier 1
// checks a spelling's exactness before moving on to its dotted tail, so a spelling that
// ends in the query can win 0.95 ahead of a later spelling that would have matched
// exactly. Preserving that is the difference between "ranked similarly" and "the same
// answer".
func matchDef(p matchPlan, d ir.Definition, ascii bool) (ir.MatchKind, float64) {
	if !ascii {
		// Byte folding is only provably the same function as strings.ToLower while
		// both sides are ASCII, and ToLower is not a byte map: it expands U+0130 to
		// "i" plus a combining dot, which puts an ASCII byte in the output that was
		// never in the input. Non-ASCII names take the path this matcher was
		// defined with instead of a near-equivalent one.
		return matchDefLegacy(p.q, d)
	}

	sp, n := spellingsOf(d)

	// Tier 1: exact, either as a whole or as the last dotted segment.
	for i := 0; i < n; i++ {
		s := sp[i]
		if foldEqual(s, p.ql) {
			return ir.MatchExact, 1.0
		}
		if p.noDot && hasDot(s) && foldEqualSeg(lastSeg(s), p.ql) {
			return ir.MatchExact, 0.95
		}
	}

	// Tier 2: prefix.
	for i := 0; i < n; i++ {
		if foldHasPrefix(sp[i], p.ql) {
			return ir.MatchPrefix, float64(len(p.q)) / float64(sp[i].len()+1)
		}
	}

	// Tier 3: bounded edit distance, best of any spelling.
	best := 99
	if len(p.segs) == 1 {
		// The common case. A one-segment query aligns against only the trailing
		// dotted part, and the spellings of one definition share that part, so the
		// expensive distance would be computed up to four times per definition with
		// identical results. Comparing the segments keeps that true even if a
		// definition ever spells its tail differently.
		seen := ""
		for i := 0; i < n; i++ {
			tail := lastSeg(sp[i])
			if i > 0 && tail == seen {
				continue
			}
			seen = tail
			if dist, ok := fuzzySeg(p.segs[0], tail); ok && dist < best {
				best = dist
			}
		}
	} else {
		for i := 0; i < n; i++ {
			if dist, ok := fuzzyJoin(p, sp[i]); ok && dist < best {
				best = dist
			}
		}
	}
	if best <= p.maxDist {
		return ir.MatchFuzzy, 1.0 / float64(best+2)
	}

	// Tier 4: last resort, so a query that names the middle of a symbol still finds
	// it (`find Controller` matched nothing at all before this tier existed, because
	// nothing is allowed to start with "Controller"). It ranks below every other
	// tier, so it can fill a gap but never overtake a better hit. The simple name is
	// the whole rule: matching the qualified name here would re-create the worst bug
	// in the tool's history, where `pay` hit the package `paymentapi` and matched the
	// entire repo.
	if d.Name != "" && len(p.q) >= minSubstringLen {
		if i := foldIndex(d.Name, p.ql); i >= 0 {
			return ir.MatchSubstring, substringScore(len(p.q), i, len(d.Name))
		}
	}
	if i, ok := camelHump(p.q, d.Name); ok {
		return ir.MatchSubstring, substringScore(len(p.q), i, len(d.Name))
	}
	return "", 0
}

// matchDefLegacy is matchDef as it stood before P7, kept verbatim as the definition of
// correct for anything byte folding does not cover. The only edit is that it takes the
// query from the plan instead of trimming it again.
func matchDefLegacy(q string, d ir.Definition) (ir.MatchKind, float64) {
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
	if score, ok := substringMatch(q, d.Name); ok {
		return ir.MatchSubstring, score
	}
	return "", 0
}

// spelling is one addressable form of a definition: a + "." + b, or just b when a is
// empty. Keeping the halves apart is what removes the concatenations; every comparison
// below walks the halves instead of joining them.
type spelling struct{ a, b string }

// joined materialises the spelling. Only fallbacks reach it, so it is allowed to
// allocate; the ranking path never calls it.
func (s spelling) joined() string {
	if s.a == "" {
		return s.b
	}
	return s.a + "." + s.b
}

func (s spelling) len() int {
	if s.a == "" {
		return len(s.b)
	}
	return len(s.a) + 1 + len(s.b)
}

// spellingsOf returns the ways a definition can be addressed, in the order the tiers are
// defined to consider them: the qualified name, the simple name, Owner.Name, and
// OwnerSimple.Name. Duplicates are dropped, which is a speed choice only: the tiers take
// their first match or their minimum, and a spelling weighed twice changes neither.
//
// It returns an array and a count rather than a slice because this runs against every
// definition in the workspace on every query, and a slice here is one allocation per
// definition — the exact cost this whole file exists to remove.
func spellingsOf(d ir.Definition) ([4]spelling, int) {
	var sp [4]spelling
	n := 0
	put := func(a, b string) {
		for i := 0; i < n; i++ {
			if sameJoin(sp[i].a, sp[i].b, a, b) {
				return
			}
		}
		sp[n] = spelling{a: a, b: b}
		n++
	}
	put("", d.Qualified)
	put("", d.Name)
	if d.Owner != "" {
		put(d.Owner, d.Name)
		simple := d.Owner
		if i := strings.LastIndexByte(simple, '.'); i >= 0 {
			simple = simple[i+1:]
		}
		put(simple, d.Name)
	}
	return sp, n
}

// sameJoin reports a1+"."+b1 == a2+"."+b2 without building either. A dotted spelling is
// never reported equal to a bare one even when the text would agree, which costs a
// redundant comparison and can never change an answer.
func sameJoin(a1, b1, a2, b2 string) bool {
	if (a1 == "") != (a2 == "") {
		return false
	}
	if a1 == "" {
		return b1 == b2
	}
	return len(a1) == len(a2) && len(b1) == len(b2) && a1 == a2 && b1 == b2
}

func hasDot(s spelling) bool {
	return s.a != "" || strings.IndexByte(s.b, '.') >= 0
}

// lastSeg is the text after the final '.' in the joined spelling. Every spelling of a
// definition ends in its Name, so this is the Name unless the halves carry a deeper dot.
func lastSeg(s spelling) string {
	if i := strings.LastIndexByte(s.b, '.'); i >= 0 {
		return s.b[i+1:]
	}
	return s.b
}

// foldEqual reports strings.EqualFold(joined, q) for an ASCII query, where EqualFold,
// ToLower equality and byte folding all agree.
func foldEqual(s spelling, lower string) bool {
	if s.len() != len(lower) {
		return false
	}
	i := 0
	if s.a != "" {
		for k := 0; k < len(s.a); k++ {
			if foldByte(s.a[k]) != lower[i] {
				return false
			}
			i++
		}
		if lower[i] != '.' {
			return false
		}
		i++
	}
	for k := 0; k < len(s.b); k++ {
		if foldByte(s.b[k]) != lower[i] {
			return false
		}
		i++
	}
	return true
}

// foldHasPrefix is strings.HasPrefix(strings.ToLower(joined), lower) for an ASCII query.
func foldHasPrefix(s spelling, lower string) bool {
	if s.len() < len(lower) {
		return false
	}
	i := 0
	if s.a != "" {
		for k := 0; k < len(s.a) && i < len(lower); k++ {
			if foldByte(s.a[k]) != lower[i] {
				return false
			}
			i++
		}
		if i < len(lower) {
			if lower[i] != '.' {
				return false
			}
			i++
		}
	}
	for k := 0; k < len(s.b) && i < len(lower); k++ {
		if foldByte(s.b[k]) != lower[i] {
			return false
		}
		i++
	}
	return true
}

// foldEqualSeg is strings.ToLower(seg) == lower for an ASCII pair.
func foldEqualSeg(seg, lower string) bool {
	if len(seg) != len(lower) {
		return false
	}
	for i := 0; i < len(seg); i++ {
		if foldByte(seg[i]) != lower[i] {
			return false
		}
	}
	return true
}

// foldIndex is strings.Index(strings.ToLower(name), lower) for an ASCII pair. Offsets
// are preserved, which matters because the match position feeds the score. Candidate
// starts are located with IndexByte, which is assembly-optimized, on both cases of the
// needle's first byte; only positions that clear that filter are compared byte by byte.
func foldIndex(name, lower string) int {
	if lower == "" || len(name) < len(lower) {
		return -1
	}
	last := len(name) - len(lower) // last start index at which a match could still fit
	head := lower[0]
	// foldByte only maps A-Z onto a-z, so those are the two bytes that can fold to
	// the needle's first byte.
	alt := byte(0)
	switch {
	case isLowerASCII(head):
		alt = head - ('a' - 'A')
	case isUpperASCII(head):
		alt = head + ('a' - 'A')
	}
	for i := 0; i <= last; {
		rest := name[i:]
		until := last - i + 1 // starts at or beyond this offset cannot match
		skip := until
		if j := strings.IndexByte(rest, head); j >= 0 && j < skip {
			skip = j
		}
		if alt != 0 && head != alt {
			if j := strings.IndexByte(rest, alt); j >= 0 && j < skip {
				skip = j
			}
		}
		if skip == until {
			return -1
		}
		i += skip
		if foldEqualAt(name, i, lower) {
			return i
		}
		i++
	}
	return -1
}

// foldEqualAt reports name[i:i+len(lower)] == lower, folding name's bytes.
func foldEqualAt(name string, i int, lower string) bool {
	for k := 0; k < len(lower); k++ {
		if foldByte(name[i+k]) != lower[k] {
			return false
		}
	}
	return true
}

// segments writes the dotted parts of a spelling into dst, matching strings.Split's
// behaviour on empty parts, and reports false when the name is deeper than dst fits.
func segments(s spelling, dst []string) ([]string, bool) {
	n := 0
	push := func(part string) bool {
		if n == len(dst) {
			return false
		}
		dst[n] = part
		n++
		return true
	}
	if s.a != "" {
		a := s.a
		for {
			i := strings.IndexByte(a, '.')
			if i < 0 {
				break
			}
			if !push(a[:i]) {
				return nil, false
			}
			a = a[i+1:]
		}
		if !push(a) {
			return nil, false
		}
	}
	b := s.b
	for {
		i := strings.IndexByte(b, '.')
		if i < 0 {
			break
		}
		if !push(b[:i]) {
			return nil, false
		}
		b = b[i+1:]
	}
	if !push(b) {
		return nil, false
	}
	return dst[:n], true
}

// fuzzyJoin is fuzzyQualified with the query split once per Search and the candidate
// walked in place of split. The alignment rule — query segments line up against the
// trailing end of the candidate — is unchanged.
func fuzzyJoin(p matchPlan, s spelling) (int, bool) {
	qs := p.segs
	// The overwhelmingly common query is a single word. It aligns against only the
	// trailing segment, so the spelling can be judged from its last dotted part
	// directly; splitting every qualified name in the workspace to reach that one
	// segment was the largest per-definition cost left after the rewrite.
	if len(qs) == 1 {
		return fuzzySeg(qs[0], lastSeg(s))
	}
	var buf [24]string
	cs, ok := segments(s, buf[:])
	if !ok {
		return fuzzyQualified(p.q, s.joined())
	}
	if len(qs) > len(cs) {
		return 0, false
	}
	offset := len(cs) - len(qs)
	total := 0
	for i, qseg := range qs {
		cseg := cs[offset+i]
		if strings.EqualFold(qseg, cseg) {
			continue
		}
		d, ok := fuzzySeg(qseg, cseg)
		if !ok {
			return 0, false
		}
		total += d
	}
	return total, true
}

// fuzzySeg is one aligned segment pair of fuzzyQualified: equal, or within the
// bounded edit distance, or out.
func fuzzySeg(qseg, cseg string) (int, bool) {
	if foldASCIIEqual(qseg, cseg) {
		return 0, true
	}
	// levenshtein's own first guard, applied before it builds two row slices.
	// Exact here because both sides are ASCII, so lowering changes no lengths.
	if abs(len(qseg)-len(cseg)) > 2 {
		return 0, false
	}
	d := levenshteinFold(qseg, cseg, 2)
	if d < 0 {
		return 0, false
	}
	return d, true
}

// foldASCIIEqual is strings.EqualFold for a pair known to be ASCII.
func foldASCIIEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if foldByte(a[i]) != foldByte(b[i]) {
			return false
		}
	}
	return true
}

// levenshteinFold is the bounded edit distance over ASCII-folded bytes, with its rows on
// the stack for name lengths Java actually has. Comparing folded bytes is the same DP as
// comparing lowered strings while both sides are ASCII, which is the only case the fast
// path reaches it from; anything longer or non-ASCII takes the original.
func levenshteinFold(a, b string, limit int) int {
	la, lb := len(a), len(b)
	if abs(la-lb) > limit {
		return -1
	}
	const rows = 96
	if lb+1 > rows {
		return levenshtein(strings.ToLower(a), strings.ToLower(b), limit)
	}
	var prev, cur [rows]int
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if foldByte(a[i-1]) == foldByte(b[j-1]) {
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

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiFlags answers "is definition i pure ASCII", computed once per index.
//
// Ranking needs this for every definition on every query, and discovering it by scanning
// the names is a large part of the work the fast path exists to remove — so it is
// decided at Build, when every name is already being touched, and costs a bit per
// definition thereafter.
type asciiFlags struct{ bits []uint64 }

func newASCIIFlags(defs []ir.Definition) asciiFlags {
	a := asciiFlags{bits: make([]uint64, (len(defs)+63)/64)}
	for i, d := range defs {
		if isASCII(d.Qualified) && isASCII(d.Name) && isASCII(d.Owner) {
			a.bits[i/64] |= 1 << (uint(i) % 64)
		}
	}
	return a
}

// has reports whether definition i is ASCII. An index assembled without Build has no
// flags and answers no, which routes every definition through the legacy matcher: slower,
// and never wrong.
func (a asciiFlags) has(i int) bool {
	if i < 0 || i/64 >= len(a.bits) {
		return false
	}
	return a.bits[i/64]&(1<<(uint(i)%64)) != 0
}
