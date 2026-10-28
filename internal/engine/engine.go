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

	"github.com/umuterturk/cxj/internal/config"
	"github.com/umuterturk/cxj/internal/index"
	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/java"
	"github.com/umuterturk/cxj/internal/scan"
	"github.com/umuterturk/cxj/internal/spring"
)

// Options configure engine opening.
type Options struct {
	Excludes []string
	Logger   *log.Logger
	// Caps selects which sub-indexes Open builds. Use CapsFor(command) rather than
	// listing bits by hand: what a command reads belongs in one table.
	// The zero value builds nothing, which returns empty results rather than
	// failing, so a command that forgets to ask is a bug you can see.
	Caps Capabilities
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
			// One grammar load per worker per extractor, so a command that reads no
			// Java pays for no parser: `keys` used to load three.
			var javaExt *java.Extractor
			if opts.Caps.Has(CapJava) {
				javaExt = java.NewExtractor()
				defer javaExt.Close()
			}
			var springExt *spring.Extractor
			if opts.Caps.Has(CapSpring) || opts.Caps.Has(CapSpringMain) {
				springExt = spring.NewExtractor()
				defer springExt.Close()
			}
			var bindExt *config.BindingExtractor
			if opts.Caps.Has(CapBindings) {
				bindExt = config.NewBindingExtractor()
				defer bindExt.Close()
			}
			for i := range jobs {
				f := res.Files[i]
				// Zero work means the file is never opened at all. That is the point:
				// skipping the indexing step but still reading 850 files would win
				// almost nothing.
				work := opts.workFor(f.RelPath, f.IsJava)
				if work == 0 {
					res.Files[i].Data = nil // nobody will read these bytes: release now
					continue
				}
				// The scanner validated this file by reading it, so the bytes come
				// along for the ride; the read here is only the fallback for a
				// caller that produced File without Data. Before P3 every file was
				// opened twice, once serially and once here. Dropping the slot's
				// reference as it is taken lets the content go incrementally
				// instead of at the end of the scan — carried bytes are the price
				// of the single read, and this is how that price stays small.
				data := f.Data
				res.Files[i].Data = nil
				if data == nil {
					b, err := os.ReadFile(f.AbsPath)
					if err != nil {
						continue
					}
					data = b
				}
				var slot fileSlot
				if work&workVersions != 0 {
					if j, b := parseBuildVersions(filepath.Base(f.RelPath), data); j != "" || b != "" {
						slot.ver = &versionHit{
							depth: strings.Count(f.RelPath, "/"),
							java:  j,
							boot:  b,
						}
					}
				}
				if f.IsJava {
					if work&workJava != 0 {
						ff, err := javaExt.Extract(f.RelPath, data)
						if err != nil {
							continue
						}
						slot.facts = &ff
					}
					if work&workSpring != 0 {
						sf := springExt.Extract(f.RelPath, data)
						slot.spring = &sf
					}
					if work&workBindings != 0 {
						slot.bindings = bindExt.ExtractBindings(f.RelPath, data)
					}
				} else {
					if work&workConfig != 0 {
						if cfg, err := config.ParseIfConfig(f.RelPath, data); err == nil && cfg != nil {
							slot.cfg = cfg
						}
					}
					if work&workText != 0 {
						slot.text = splitLinesRedacted(f.RelPath, data)
					}
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
	// Every index is built, empty or not: a nil *SymbolIndex panics on the first
	// method call, and a command that misdeclares its capabilities should return
	// nothing, not crash. Building from an empty slice costs nothing, so the saving
	// here comes from not extracting, not from skipping the constructor.
	var symFacts []ir.FileFacts
	if opts.Caps.Has(CapSymbolIndex) {
		symFacts = facts
	}
	// The four sub-indexes are independent and the inputs are already collected,
	// so they build concurrently and the serial tail of Open is one build, not
	// their sum (P3). They are also small — a measured 10 ms worst case at
	// CapsAll — which is the honest end of this phase: the serial fraction was
	// never here, it was the matcher and the double read in scan.
	var (
		syms      *index.SymbolIndex
		textIdx   *index.TextIndex
		springIdx *spring.Index
		cfgIdx    *config.Index
		idxWG     sync.WaitGroup
	)
	idxWG.Add(4)
	go func() { defer idxWG.Done(); syms = index.Build(symFacts) }()
	go func() { defer idxWG.Done(); textIdx = index.BuildText(textLines) }()
	go func() { defer idxWG.Done(); springIdx = spring.Build(springFacts) }()
	go func() { defer idxWG.Done(); cfgIdx = config.Build(cfgFiles, bindings) }()
	idxWG.Wait()
	return &Engine{
		root:    root,
		stats:   res.Stats,
		gen:     1,
		facts:   facts,
		syms:    syms,
		text:    textIdx,
		spring:  springIdx,
		cfg:     cfgIdx,
		javaVer: javaVer,
		bootVer: bootVer,
	}, nil
}

func isBuildFileName(path string) bool {
	switch filepath.Base(path) {
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
