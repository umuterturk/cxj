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
}

// BuildText indexes non-Java text files as lines.
func BuildText(lines []TextLine) *TextIndex {
	return &TextIndex{lines: lines}
}

// Search returns text hits for query (case-insensitive substring).
func (t *TextIndex) Search(q string, limit int) []ir.SearchResult {
	q = strings.TrimSpace(q)
	if q == "" || t == nil || limit <= 0 {
		return nil
	}
	lower := strings.ToLower(q)
	var hits []ir.SearchResult
	for _, ln := range t.lines {
		if !strings.Contains(strings.ToLower(ln.Text), lower) {
			continue
		}
		snippet := ln.Text
		if len(snippet) > 160 {
			snippet = snippet[:160]
		}
		hits = append(hits, ir.SearchResult{
			Path:    ln.Path,
			Line:    ln.Line,
			Symbol:  "",
			Kind:    ir.KindText,
			Match:   ir.MatchText,
			Score:   float64(len(q)) / float64(len(ln.Text)+1),
			Snippet: strings.TrimRightFunc(snippet, unicode.IsSpace),
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
