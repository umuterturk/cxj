// Package engine owns a one-shot in-RAM workspace snapshot for a CLI invocation.
package engine

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

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
	// BriefOnly skips symbol/text/binding indexes and spring-extracts only main sources.
	// Safe for Brief(); other commands need a full Open.
	BriefOnly bool
	// Scope selects which source tree to index. Zero value (ScopeAll) matches
	// historical behavior. Main-only is the default for navigational commands.
	Scope scan.Scope
}

// Engine is an immutable in-RAM snapshot of the workspace.
type Engine struct {
	root    string
	stats   scan.Stats
	gen     int
	facts   []ir.FileFacts
	syms    *index.SymbolIndex
	text    *index.TextIndex
	spring  *spring.Index
	cfg     *config.Index
	javaVer string
	bootVer string
}

type fileSlot struct {
	facts    *ir.FileFacts
	spring   *spring.Facts
	cfg      *config.FileFacts
	bindings []config.Binding
	text     []index.TextLine
	ver      *versionHit
}

// Open scans root and builds lexical, Spring, and config indexes.
func Open(root string, opts Options) (*Engine, error) {
	res, err := scan.Scan(root, scan.Options{Excludes: opts.Excludes, Scope: opts.Scope})
	if err != nil {
		return nil, err
	}

	n := len(res.Files)
	slots := make([]fileSlot, n)
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > n && n > 0 {
		workers = n
	}

	jobs := make(chan int, workers*2)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			javaExt := java.NewExtractor()
			defer javaExt.Close()
			springExt := spring.NewExtractor()
			defer springExt.Close()
			var bindExt *config.BindingExtractor
			if !opts.BriefOnly {
				bindExt = config.NewBindingExtractor()
				defer bindExt.Close()
			}
			for i := range jobs {
				f := res.Files[i]
				base := filepath.Base(f.RelPath)
				if opts.BriefOnly && !f.IsJava {
					if !config.IsConfigFile(f.RelPath) && !isBuildFileName(base) {
						continue
					}
				}
				data, err := os.ReadFile(f.AbsPath)
				if err != nil {
					continue
				}
				var slot fileSlot
				if j, b := parseBuildVersions(base, data); j != "" || b != "" {
					slot.ver = &versionHit{
						depth: strings.Count(f.RelPath, "/"),
						java:  j,
						boot:  b,
					}
				}
				if f.IsJava {
					ff, err := javaExt.Extract(f.RelPath, data)
					if err != nil {
						continue
					}
					slot.facts = &ff
					// Brief only needs spring facts from main sources (HTTP/outbounds/runners).
					if !opts.BriefOnly || !isTestPath(f.RelPath) {
						sf := springExt.Extract(f.RelPath, data)
						slot.spring = &sf
					}
					if bindExt != nil {
						slot.bindings = bindExt.ExtractBindings(f.RelPath, data)
					}
					slots[i] = slot
					continue
				}
				if cfg, err := config.ParseIfConfig(f.RelPath, data); err == nil && cfg != nil {
					slot.cfg = cfg
				}
				if !opts.BriefOnly {
					slot.text = splitLinesRedacted(f.RelPath, data)
				}
				slots[i] = slot
			}
		}()
	}
	for i := range res.Files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	facts := make([]ir.FileFacts, 0, n)
	springFacts := make([]spring.Facts, 0, n)
	cfgFiles := make([]config.FileFacts, 0, 32)
	var bindings []config.Binding
	var textLines []index.TextLine
	var verHits []versionHit
	for i := range slots {
		s := &slots[i]
		if s.ver != nil {
			verHits = append(verHits, *s.ver)
		}
		if s.facts != nil {
			facts = append(facts, *s.facts)
		}
		if s.spring != nil {
			springFacts = append(springFacts, *s.spring)
		}
		if s.cfg != nil {
			cfgFiles = append(cfgFiles, *s.cfg)
		}
		if len(s.bindings) > 0 {
			bindings = append(bindings, s.bindings...)
		}
		if len(s.text) > 0 {
			textLines = append(textLines, s.text...)
		}
	}

	javaVer, bootVer := pickVersions(verHits)
	eng := &Engine{
		root:    root,
		stats:   res.Stats,
		gen:     1,
		facts:   facts,
		spring:  spring.Build(springFacts),
		cfg:     config.Build(cfgFiles, bindings),
		javaVer: javaVer,
		bootVer: bootVer,
	}
	if !opts.BriefOnly {
		eng.syms = index.Build(facts)
		eng.text = index.BuildText(textLines)
	}
	return eng, nil
}

func isBuildFileName(base string) bool {
	switch base {
	case "pom.xml", "build.gradle", "build.gradle.kts":
		return true
	default:
		return false
	}
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
	return ir.SearchReply{Results: mergeLexical(q, e.syms.Search(q, limit), e.text.Search(q, limit), limit)}
}

// mergeLexical interleaves symbol hits with raw text hits using the same ranking
// the symbol index used, so nothing the index decided gets undone on the way out.
func mergeLexical(q string, sym, text []ir.SearchResult, limit int) []ir.SearchResult {
	all := make([]ir.SearchResult, 0, len(sym)+len(text))
	all = append(all, sym...)
	all = append(all, text...)
	wantsType := ir.WantsTypeName(q)
	sort.SliceStable(all, func(i, j int) bool { return ir.LexicalLess(all[i], all[j], wantsType) })
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		return []ir.SearchResult{}
	}
	return all
}

// resolveOwners maps a user query to the definitions it can mean.
//
// It returns the best fully-qualified name to ask the index about, plus the full
// candidate set when the query matched more than one definition. Returning the
// qualified name matters beyond tidiness: the index matchers are strict about
// qualified queries and only fall back to simple-name matching for genuinely
// unqualified input, so resolving here is what keeps a precise query precise.
//
// The candidate list includes the definition actually answered about, not just the
// losers. A reader shown "ambiguous: RefundHandler.handle" while reading results
// for PayHandler.handle would wrongly conclude those two are different queries.
func (e *Engine) resolveOwners(symbol string) (string, []string) {
	symbol = strings.TrimSpace(symbol)
	all := e.syms.LookupAll(symbol)
	if len(all) <= 1 {
		if len(all) == 1 {
			return all[0].Qualified, nil
		}
		return symbol, nil
	}
	cands := make([]string, 0, len(all))
	for _, d := range all {
		cands = append(cands, d.Qualified)
	}
	return all[0].Qualified, cands
}

// References looks up symbol uses. The query is resolved to a definition first; if
// nothing is found it retries through lexical/fuzzy search — never inventing exact
// dispatch.
func (e *Engine) References(symbol string) ir.RefReply {
	qualified, alts := e.resolveOwners(symbol)
	results := e.syms.References(qualified)
	resolved := ""
	if qualified != symbol && len(results) > 0 {
		resolved = qualified
	}
	if len(results) == 0 {
		if name, ok := resolveSymbolName(e.syms.Search(symbol, 5)); ok && !strings.EqualFold(name, symbol) {
			resolved = name
			results = e.syms.References(name)
		}
	}
	if results == nil {
		results = []ir.RefResult{}
	}
	return ir.RefReply{Results: results, Resolved: resolved, Ambiguous: alts}
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

// ScopeFilteredTests reports whether the Open dropped files due to a non-all
// scope. Used to hint the user toward --scope all on a miss.
func (e *Engine) ScopeFilteredTests() bool { return e.stats.SkippedScope > 0 }

// Callees lists outbound call sites from symbol, scoped to that owner.
func (e *Engine) Callees(symbol string) ir.RefReply {
	qualified, alts := e.resolveOwners(symbol)
	results := e.syms.Callees(qualified)
	resolved := ""
	if qualified != symbol && len(results) > 0 {
		resolved = qualified
	}
	if len(results) == 0 {
		if name, ok := resolveSymbolName(e.syms.Search(symbol, 5)); ok && !strings.EqualFold(name, symbol) {
			resolved = name
			results = e.syms.Callees(name)
		}
	}
	if results == nil {
		results = []ir.RefResult{}
	}
	return ir.RefReply{Results: results, Resolved: resolved, Ambiguous: alts}
}

// Keys lists config key names without values.
func (e *Engine) Keys() ir.KeysReply {
	return e.cfg.Keys()
}
