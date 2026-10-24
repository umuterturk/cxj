// Package engine owns the in-RAM workspace snapshot served over HTTP.
package engine

import (
	"fmt"
	"os"
	"sync"

	"example.com/cx/internal/index"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
	"example.com/cx/internal/scan"
)

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
}

// Open scans root, extracts Java facts, and returns a ready lexical engine.
func Open(root string) (*Engine, error) {
	res, err := scan.Scan(root)
	if err != nil {
		return nil, err
	}
	ext := java.NewExtractor()
	defer ext.Close()

	var facts []ir.FileFacts
	for _, f := range res.Files {
		if !f.IsJava {
			continue
		}
		data, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		ff, err := ext.Extract(f.RelPath, data)
		if err != nil {
			continue
		}
		facts = append(facts, ff)
	}

	return &Engine{
		root:  root,
		files: res.Files,
		stats: res.Stats,
		gen:   1,
		sem:   ir.SemanticOff,
		syms:  index.Build(facts),
	}, nil
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
		results := e.syms.Search(q, limit)
		if results == nil {
			results = []ir.SearchResult{}
		}
		return ir.SearchReply{Results: results, SemanticState: e.sem}, nil
	case "semantic", "hybrid":
		if e.sem != ir.SemanticReady {
			return ir.SearchReply{}, fmt.Errorf("semantic_not_ready")
		}
		return ir.SearchReply{Results: []ir.SearchResult{}, SemanticState: e.sem}, nil
	default:
		return ir.SearchReply{}, fmt.Errorf("unknown mode %q", mode)
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
