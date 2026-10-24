// Package engine owns the in-RAM workspace snapshot served over HTTP.
package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"example.com/cx/internal/index"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
	"example.com/cx/internal/scan"
)

// Options configure engine opening.
type Options struct {
	Excludes []string
}

// Engine is a single-writer, immutable-read snapshot of the workspace.
type Engine struct {
	mu     sync.RWMutex
	root   string
	files  []scan.File
	stats  scan.Stats
	gen    int
	sem    ir.SemanticState
	semGen int
	model  string
	syms   *index.SymbolIndex
	text   *index.TextIndex
}

// Open scans root, extracts Java + text indexes, and returns a ready lexical engine.
func Open(root string, opts Options) (*Engine, error) {
	res, err := scan.Scan(root, scan.Options{Excludes: opts.Excludes})
	if err != nil {
		return nil, err
	}
	ext := java.NewExtractor()
	defer ext.Close()

	var facts []ir.FileFacts
	var textLines []index.TextLine
	for _, f := range res.Files {
		data, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		if f.IsJava {
			ff, err := ext.Extract(f.RelPath, data)
			if err != nil {
				continue
			}
			facts = append(facts, ff)
			continue
		}
		textLines = append(textLines, splitLines(f.RelPath, data)...)
	}

	return &Engine{
		root:  root,
		files: res.Files,
		stats: res.Stats,
		gen:   1,
		sem:   ir.SemanticOff,
		syms:  index.Build(facts),
		text:  index.BuildText(textLines),
	}, nil
}

func splitLines(path string, data []byte) []index.TextLine {
	raw := string(data)
	parts := strings.Split(raw, "\n")
	out := make([]index.TextLine, 0, len(parts))
	for i, line := range parts {
		// Keep last empty line only if file ended without newline? Skip empty trailing.
		if line == "" && i == len(parts)-1 {
			continue
		}
		out = append(out, index.TextLine{Path: path, Line: i + 1, Text: line})
	}
	return out
}

// Status implements server.Index.
func (e *Engine) Status() ir.Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return ir.Status{
		Status:             "ready",
		IndexedFiles:       e.stats.IndexedFiles,
		IndexedJavaFiles:   e.stats.IndexedJavaFiles,
		Generation:         e.gen,
		SemanticState:      e.sem,
		SemanticGeneration: e.semGen,
		Model:              e.model,
	}
}

// Search implements server.Index.
func (e *Engine) Search(q, mode string, limit int) (ir.SearchReply, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	switch mode {
	case "lexical", "":
		symHits := e.syms.Search(q, limit)
		textHits := e.text.Search(q, limit)
		merged := mergeLexical(symHits, textHits, limit)
		return ir.SearchReply{Results: merged, SemanticState: e.sem}, nil
	case "semantic", "hybrid":
		if e.sem != ir.SemanticReady {
			return ir.SearchReply{}, fmt.Errorf("semantic_not_ready")
		}
		return ir.SearchReply{Results: []ir.SearchResult{}, SemanticState: e.sem}, nil
	default:
		return ir.SearchReply{}, fmt.Errorf("unknown mode %q", mode)
	}
}

func mergeLexical(sym, text []ir.SearchResult, limit int) []ir.SearchResult {
	all := make([]ir.SearchResult, 0, len(sym)+len(text))
	all = append(all, sym...)
	all = append(all, text...)
	sort.SliceStable(all, func(i, j int) bool {
		if rankMatch(all[i].Match) != rankMatch(all[j].Match) {
			return rankMatch(all[i].Match) < rankMatch(all[j].Match)
		}
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		if all[i].Path != all[j].Path {
			return all[i].Path < all[j].Path
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Symbol < all[j].Symbol
	})
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		return []ir.SearchResult{}
	}
	return all
}

func rankMatch(m ir.MatchKind) int {
	switch m {
	case ir.MatchExact:
		return 0
	case ir.MatchPrefix:
		return 1
	case ir.MatchFuzzy:
		return 2
	case ir.MatchText:
		return 3
	default:
		return 9
	}
}

// References implements server.Index.
func (e *Engine) References(symbol string) ir.RefReply {
	e.mu.RLock()
	defer e.mu.RUnlock()
	results := e.syms.References(symbol)
	if results == nil {
		results = []ir.RefResult{}
	}
	return ir.RefReply{Results: results}
}

// Root returns the absolute workspace root.
func (e *Engine) Root() string { return e.root }
