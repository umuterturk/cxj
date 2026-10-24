// Package engine owns the in-RAM workspace snapshot served over HTTP.
package engine

import (
	"fmt"
	"sync"

	"example.com/cx/internal/ir"
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
}

// Open scans root and returns a ready lexical engine. Semantic stays off unless configured later.
func Open(root string) (*Engine, error) {
	res, err := scan.Scan(root)
	if err != nil {
		return nil, err
	}
	return &Engine{
		root:  root,
		files: res.Files,
		stats: res.Stats,
		gen:   1,
		sem:   ir.SemanticOff,
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

// Search implements server.Index. Phase 0 returns an empty result set.
func (e *Engine) Search(q, mode string, limit int) (ir.SearchReply, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	switch mode {
	case "lexical", "":
		return ir.SearchReply{Results: []ir.SearchResult{}, SemanticState: e.sem}, nil
	case "semantic", "hybrid":
		if e.sem != ir.SemanticReady {
			return ir.SearchReply{}, fmt.Errorf("semantic_not_ready")
		}
		return ir.SearchReply{Results: []ir.SearchResult{}, SemanticState: e.sem}, nil
	default:
		return ir.SearchReply{}, fmt.Errorf("unknown mode %q", mode)
	}
}

// Root returns the absolute workspace root.
func (e *Engine) Root() string { return e.root }
