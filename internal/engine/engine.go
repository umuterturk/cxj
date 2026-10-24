// Package engine owns the in-RAM workspace snapshot served over HTTP.
package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/cx/internal/embed"
	"example.com/cx/internal/index"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
	"example.com/cx/internal/scan"
)

// Options configure engine opening.
type Options struct {
	Excludes   []string
	EmbedURL   string
	EmbedModel string
	Logger     *log.Logger
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
	store  *embed.Store
	prov   *embed.Provider
	chunks []embed.Chunk
	log    *log.Logger
}

// Open scans root, extracts Java + text indexes, and returns a ready lexical engine.
// When EmbedURL is set, semantic indexing starts in the background.
func Open(root string, opts Options) (*Engine, error) {
	res, err := scan.Scan(root, scan.Options{Excludes: opts.Excludes})
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	ext := java.NewExtractor()
	defer ext.Close()

	var facts []ir.FileFacts
	var textLines []index.TextLine
	var chunks []embed.Chunk
	srcByPath := map[string][]byte{}
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
			srcByPath[f.RelPath] = data
			chunks = append(chunks, embed.ChunksFromFacts(ff, data)...)
			continue
		}
		textLines = append(textLines, splitLines(f.RelPath, data)...)
	}

	e := &Engine{
		root:   root,
		files:  res.Files,
		stats:  res.Stats,
		gen:    1,
		sem:    ir.SemanticOff,
		syms:   index.Build(facts),
		text:   index.BuildText(textLines),
		store:  &embed.Store{},
		chunks: chunks,
		log:    logger,
	}
	_ = srcByPath

	if opts.EmbedURL != "" {
		model := opts.EmbedModel
		if model == "" {
			model = "qwen3-embedding:0.6b"
		}
		e.model = model
		e.prov = embed.NewProvider(opts.EmbedURL, model)
		e.setSemantic(ir.SemanticBuilding)
		go e.buildSemantic()
	}
	return e, nil
}

func (e *Engine) setSemantic(st ir.SemanticState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sem = st
	if st == ir.SemanticReady {
		e.semGen++
	}
}

func (e *Engine) buildSemantic() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := e.prov.Ping(ctx); err != nil {
		e.log.Printf("embedding provider unavailable: %v", err)
		e.setSemantic(ir.SemanticUnavailable)
		return
	}
	const batch = 8
	for i := 0; i < len(e.chunks); i += batch {
		if ctx.Err() != nil {
			e.setSemantic(ir.SemanticUnavailable)
			return
		}
		end := i + batch
		if end > len(e.chunks) {
			end = len(e.chunks)
		}
		part := e.chunks[i:end]
		inputs := make([]string, len(part))
		for j, c := range part {
			inputs[j] = c.Text
		}
		vecs, err := e.prov.Embed(ctx, inputs)
		if err != nil {
			e.log.Printf("embed batch failed: %v", err)
			e.setSemantic(ir.SemanticUnavailable)
			return
		}
		for j, v := range vecs {
			e.store.Add(part[j], v)
		}
	}
	e.setSemantic(ir.SemanticReady)
	e.log.Printf("semantic index ready: %d chunks", e.store.Len())
}

func splitLines(path string, data []byte) []index.TextLine {
	raw := string(data)
	parts := strings.Split(raw, "\n")
	out := make([]index.TextLine, 0, len(parts))
	for i, line := range parts {
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
	sem := e.sem
	e.mu.RUnlock()

	switch mode {
	case "lexical", "":
		e.mu.RLock()
		defer e.mu.RUnlock()
		merged := mergeLexical(e.syms.Search(q, limit), e.text.Search(q, limit), limit)
		return ir.SearchReply{Results: merged, SemanticState: sem}, nil
	case "semantic":
		if sem != ir.SemanticReady {
			return ir.SearchReply{}, fmt.Errorf("semantic_not_ready")
		}
		hits, err := e.semanticSearch(q, limit)
		if err != nil {
			return ir.SearchReply{}, err
		}
		return ir.SearchReply{Results: hits, SemanticState: sem}, nil
	case "hybrid":
		if sem != ir.SemanticReady {
			return ir.SearchReply{}, fmt.Errorf("semantic_not_ready")
		}
		e.mu.RLock()
		lex := mergeLexical(e.syms.Search(q, limit), e.text.Search(q, limit), limit)
		e.mu.RUnlock()
		semHits, err := e.semanticSearch(q, limit)
		if err != nil {
			return ir.SearchReply{}, err
		}
		var exact []ir.SearchResult
		for _, r := range lex {
			if r.Match == ir.MatchExact {
				exact = append(exact, r)
			}
		}
		merged := embed.RRF(exact, lex, semHits, limit)
		return ir.SearchReply{Results: merged, SemanticState: sem}, nil
	default:
		return ir.SearchReply{}, fmt.Errorf("unknown mode %q", mode)
	}
}

func (e *Engine) semanticSearch(q string, limit int) ([]ir.SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	vecs, err := e.prov.Embed(ctx, []string{q})
	if err != nil {
		return nil, fmt.Errorf("semantic_not_ready")
	}
	if len(vecs) == 0 {
		return []ir.SearchResult{}, nil
	}
	hits := e.store.Search(vecs[0], limit)
	return embed.ToResults(hits, ir.MatchSemantic), nil
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
