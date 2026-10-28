package index

import (
	"sort"
	"strings"
	"unicode"

	"example.com/cx/internal/ir"
)

// TextLine is one indexed UTF-8 text line.
type TextLine struct {
	Path string
	Line int
	Text string
}

// TextIndex is a simple line corpus for lexical text search.
type TextIndex struct {
	lines []TextLine
	// lows holds strings.ToLower(lines[i].Text), computed once per Open. Search is
	// then exactly strings.Contains on it, which is what the per-query ToLower did —
	// same bytes, same answer, including for non-ASCII lines — just paid once
	// instead of once per line per query. Contains is assembly-optimized; the
	// byte-folded scan it replaces is not, and was the single largest cost left in
	// a `find` after the matcher rewrite.
	lows []string
}

// BuildText indexes non-Java text files as lines.
func BuildText(lines []TextLine) *TextIndex {
	lows := make([]string, len(lines))
	for i, ln := range lines {
		lows[i] = strings.ToLower(ln.Text)
	}
	return &TextIndex{lines: lines, lows: lows}
}

// Search returns text hits for query (case-insensitive substring).
func (t *TextIndex) Search(q string, limit int) []ir.SearchResult {
	q = strings.TrimSpace(q)
	if q == "" || t == nil || limit <= 0 {
		return nil
	}
	lower := strings.ToLower(q)
	less := func(a, b ir.SearchResult) bool {
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	}
	top := newTopHits(limit, less)
	for i, ln := range t.lines {
		if !strings.Contains(t.lows[i], lower) {
			continue
		}
		snippet := ln.Text
		if len(snippet) > 160 {
			snippet = snippet[:160]
		}
		top.add(ir.SearchResult{
			Path:    ln.Path,
			Line:    ln.Line,
			Symbol:  "",
			Kind:    ir.KindText,
			Match:   ir.MatchText,
			Score:   float64(len(q)) / float64(len(ln.Text)+1),
			Snippet: strings.TrimRightFunc(snippet, unicode.IsSpace),
		})
	}
	return top.out()
}

// topHits keeps the best `limit` results as they are produced, instead of collecting
// every hit in the workspace to sort and then throw most of it away. A query like
// `pay` prefix-matches a large share of a real repository, and the full-collection
// sort was a measurable part of that query's cost.
//
// The result is identical to append-all + sort.SliceStable + truncate: an insertion
// goes at the first position it is strictly less than, so ties land *after* the ties
// already held — which is exactly what "stable" preserves — and a full list only
// admits a candidate that beats its current last element.
type topHits struct {
	hits  []ir.SearchResult
	limit int
	less  func(a, b ir.SearchResult) bool
}

func newTopHits(limit int, less func(a, b ir.SearchResult) bool) topHits {
	return topHits{limit: limit, less: less}
}

func (t *topHits) add(h ir.SearchResult) {
	// First position strictly worse than h; ties keep their place ahead of h.
	pos := sort.Search(len(t.hits), func(i int) bool { return t.less(h, t.hits[i]) })
	if pos == len(t.hits) {
		if len(t.hits) < t.limit {
			t.hits = append(t.hits, h)
		}
		return
	}
	// pos < len(hits) ≤ limit: h belongs in the top list. Insert at pos; if the list
	// is already full, the shift falls off the last element, which h just beat.
	t.hits = append(t.hits, ir.SearchResult{})
	copy(t.hits[pos+1:], t.hits[pos:])
	t.hits[pos] = h
	if len(t.hits) > t.limit {
		t.hits = t.hits[:t.limit]
	}
}

func (t *topHits) out() []ir.SearchResult {
	if t.hits == nil {
		return nil
	}
	return t.hits
}
