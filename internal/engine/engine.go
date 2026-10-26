// Package engine owns a one-shot in-RAM workspace snapshot for a CLI invocation.
package engine

import (
	"log"
	"os"
	"runtime"
	"sort"
	"strings"

	"example.com/cx/internal/config"
	"example.com/cx/internal/index"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
	"example.com/cx/internal/scan"
	"example.com/cx/internal/spring"
)

// Options configure engine opening.
type Options struct {
	Excludes []string
	Logger   *log.Logger
}

// Engine is an immutable in-RAM snapshot of the workspace.
type Engine struct {
	root   string
	stats  scan.Stats
	gen    int
	facts  []ir.FileFacts
	syms   *index.SymbolIndex
	text   *index.TextIndex
	spring *spring.Index
	cfg    *config.Index
}

// Open scans root and builds lexical, Spring, and config indexes.
func Open(root string, opts Options) (*Engine, error) {
	res, err := scan.Scan(root, scan.Options{Excludes: opts.Excludes})
	if err != nil {
		return nil, err
	}
	ext := java.NewExtractor()
	defer ext.Close()
	springExt := spring.NewExtractor()
	defer springExt.Close()
	bindExt := config.NewBindingExtractor()
	defer bindExt.Close()

	var facts []ir.FileFacts
	var springFacts []spring.Facts
	var cfgFiles []config.FileFacts
	var bindings []config.Binding
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
			springFacts = append(springFacts, springExt.Extract(f.RelPath, data))
			bindings = append(bindings, bindExt.ExtractBindings(f.RelPath, data)...)
			continue
		}
		if cfg, err := config.ParseIfConfig(f.RelPath, data); err == nil && cfg != nil {
			cfgFiles = append(cfgFiles, *cfg)
		}
		textLines = append(textLines, splitLinesRedacted(f.RelPath, data)...)
	}

	return &Engine{
		root:   root,
		stats:  res.Stats,
		gen:    1,
		facts:  facts,
		syms:   index.Build(facts),
		text:   index.BuildText(textLines),
		spring: spring.Build(springFacts),
		cfg:    config.Build(cfgFiles, bindings),
	}, nil
}

func splitLinesRedacted(path string, data []byte) []index.TextLine {
	raw := string(data)
	parts := strings.Split(raw, "\n")
	out := make([]index.TextLine, 0, len(parts))
	for i, line := range parts {
		if line == "" && i == len(parts)-1 {
			continue
		}
		out = append(out, index.TextLine{Path: path, Line: i + 1, Text: config.RedactText(line)})
	}
	return out
}

// Status reports index counters and current Go heap.
func (e *Engine) Status() ir.Status {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return ir.Status{
		Status:           "ready",
		IndexedFiles:     e.stats.IndexedFiles,
		IndexedJavaFiles: e.stats.IndexedJavaFiles,
		Generation:       e.gen,
		GoHeapAllocBytes: mem.HeapAlloc,
	}
}

// Search runs lexical symbol + text search.
func (e *Engine) Search(q string, limit int) ir.SearchReply {
	if limit <= 0 {
		limit = ir.DefaultFindLimit
	}
	if limit > 50 {
		limit = 50
	}
	return ir.SearchReply{Results: mergeLexical(e.syms.Search(q, limit), e.text.Search(q, limit), limit)}
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

// References looks up symbol uses. If the name has no exact hits, resolve it
// through lexical/fuzzy search and retry — do not invent exact dispatch.
func (e *Engine) References(symbol string) ir.RefReply {
	results := e.syms.References(symbol)
	resolved := ""
	if len(results) == 0 {
		if name, ok := resolveSymbolName(e.syms.Search(symbol, 5)); ok && !strings.EqualFold(name, symbol) {
			resolved = name
			results = e.syms.References(name)
		}
	}
	if results == nil {
		results = []ir.RefResult{}
	}
	return ir.RefReply{Results: results, Resolved: resolved}
}

// Callers is References filtered to use sites only.
func (e *Engine) Callers(symbol string) ir.RefReply {
	reply := e.References(symbol)
	var uses []ir.RefResult
	for _, r := range reply.Results {
		if r.Kind == ir.RefKindUse {
			uses = append(uses, r)
		}
	}
	if uses == nil {
		uses = []ir.RefResult{}
	}
	reply.Results = uses
	return reply
}

func resolveSymbolName(hits []ir.SearchResult) (string, bool) {
	for _, h := range hits {
		if h.Kind == ir.KindText || h.Kind == ir.KindConfig {
			continue
		}
		name := h.Symbol
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		if name != "" {
			return name, true
		}
	}
	return "", false
}

// Endpoints lists Spring HTTP mappings.
func (e *Engine) Endpoints(method, path string) any {
	results := e.spring.Endpoints(method, path)
	if results == nil {
		results = []spring.EndpointResult{}
	}
	return map[string]any{"results": results}
}

// Beans lists Spring beans for a type.
func (e *Engine) Beans(typ string) any {
	results := e.spring.Beans(typ)
	if results == nil {
		results = []spring.BeanResult{}
	}
	return map[string]any{"results": results}
}

// Context returns bounded symbol context.
func (e *Engine) Context(symbol string, budget int) any {
	return e.spring.Context(symbol, budget)
}

// Config resolves a configuration key.
func (e *Engine) Config(key, profile string) any {
	return e.cfg.Query(key, profile)
}

// Root returns the workspace root used for this snapshot.
func (e *Engine) Root() string { return e.root }

// Callees lists outbound call sites from symbol.
func (e *Engine) Callees(symbol string) ir.RefReply {
	results := e.syms.Callees(symbol)
	resolved := ""
	if len(results) == 0 {
		if name, ok := resolveSymbolName(e.syms.Search(symbol, 5)); ok && !strings.EqualFold(name, symbol) {
			resolved = name
			results = e.syms.Callees(name)
		}
	}
	if results == nil {
		results = []ir.RefResult{}
	}
	return ir.RefReply{Results: results, Resolved: resolved}
}

// Keys lists config key names without values.
func (e *Engine) Keys() ir.KeysReply {
	return e.cfg.Keys()
}
