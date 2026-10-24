package embed

import (
	"fmt"
	"strings"

	"example.com/cx/internal/ir"
)

// Chunk is one embeddable Java unit (class or method).
type Chunk struct {
	ID     string
	Path   string
	Line   int
	Symbol string
	Kind   ir.ResultKind
	Text   string
}

const maxChunkRunes = 1800

// ChunksFromFacts builds embedding texts from IR definitions and source.
func ChunksFromFacts(facts ir.FileFacts, src []byte) []Chunk {
	var out []Chunk
	for _, d := range facts.Definitions {
		switch d.Kind {
		case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum, ir.SymbolMethod, ir.SymbolConstructor:
		default:
			continue
		}
		snippet := extractSnippet(src, d)
		text := formatChunk(facts.Package, d, snippet)
		out = append(out, Chunk{
			ID:     fmt.Sprintf("%s:%d:%s", facts.Path, d.Span.Line, d.Qualified),
			Path:   facts.Path,
			Line:   d.Span.Line,
			Symbol: d.Qualified,
			Kind:   kindOf(d.Kind),
			Text:   truncateRunes(text, maxChunkRunes),
		})
	}
	return out
}

func kindOf(k ir.SymbolKind) ir.ResultKind {
	switch k {
	case ir.SymbolMethod, ir.SymbolConstructor:
		return ir.KindMethod
	default:
		return ir.KindClass
	}
}

func formatChunk(pkg string, d ir.Definition, snippet string) string {
	var b strings.Builder
	if pkg != "" {
		b.WriteString("package ")
		b.WriteString(pkg)
		b.WriteByte('\n')
	}
	b.WriteString(string(d.Kind))
	b.WriteByte(' ')
	b.WriteString(d.Qualified)
	if d.Signature != "" {
		b.WriteString(d.Signature)
	}
	b.WriteByte('\n')
	if snippet != "" {
		b.WriteString(snippet)
	}
	return b.String()
}

func extractSnippet(src []byte, d ir.Definition) string {
	start := int(d.Span.StartByte)
	end := int(d.Span.EndByte)
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	// Expand upward a bit to capture nearby comments/annotations.
	lineStart := start
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	extra := lineStart
	linesUp := 0
	for extra > 0 && linesUp < 6 {
		extra--
		if src[extra] == '\n' {
			linesUp++
		}
	}
	if linesUp == 6 {
		extra++ // after newline
	}
	if end < start {
		end = start
	}
	// Prefer definition span if method body end is unknown; widen slightly.
	widen := end + 200
	if widen > len(src) {
		widen = len(src)
	}
	// For methods, span is often just the name; try to extend to following '{' block end heuristically.
	if d.Kind == ir.SymbolMethod || d.Kind == ir.SymbolConstructor {
		if brace := findMethodBodyEnd(src, start); brace > end {
			widen = brace
		}
	}
	if d.Kind == ir.SymbolClass || d.Kind == ir.SymbolInterface || d.Kind == ir.SymbolEnum {
		if brace := findMatchingBrace(src, start); brace > end {
			// Cap class bodies.
			if brace-extra > maxChunkRunes*4 {
				brace = extra + maxChunkRunes*4
			}
			widen = brace
		}
	}
	return string(src[extra:widen])
}

func findMethodBodyEnd(src []byte, from int) int {
	i := from
	for i < len(src) && src[i] != '{' {
		i++
	}
	if i >= len(src) {
		return from
	}
	return findMatchingBrace(src, i)
}

func findMatchingBrace(src []byte, from int) int {
	i := from
	for i < len(src) && src[i] != '{' {
		i++
	}
	if i >= len(src) {
		return from
	}
	depth := 0
	for ; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(src)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
