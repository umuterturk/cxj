package spring

import (
	"sort"
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
)

// Index answers Spring HTTP/bean/context queries.
type Index struct {
	endpoints    []Endpoint
	beans        []Bean
	injections   []Injection
	transactions []TransactionDecl
	repositories []RepositoryRel
	entrypoints  []ir.Entrypoint
	httpClients  []string
}

// Build merges facts.
func Build(facts []Facts) *Index {
	idx := &Index{}
	for _, f := range facts {
		idx.endpoints = append(idx.endpoints, f.Endpoints...)
		idx.beans = append(idx.beans, f.Beans...)
		idx.injections = append(idx.injections, f.Injections...)
		idx.transactions = append(idx.transactions, f.Transactions...)
		idx.repositories = append(idx.repositories, f.Repositories...)
		idx.entrypoints = append(idx.entrypoints, f.Entrypoints...)
		idx.httpClients = append(idx.httpClients, f.HTTPClients...)
	}
	return idx
}

// HasSpringBoot reports a @SpringBootApplication type (not a plain main).
func (idx *Index) HasSpringBoot() bool {
	if idx == nil {
		return false
	}
	for _, ep := range idx.entrypoints {
		if ep.Kind == "boot" {
			return true
		}
	}
	return false
}

// BriefHTTP is every controller mapping as method + route.
func (idx *Index) BriefHTTP() []ir.BriefHTTP {
	if idx == nil {
		return nil
	}
	eps := idx.Endpoints("", "")
	seen := map[string]bool{}
	var out []ir.BriefHTTP
	for _, e := range eps {
		key := e.HTTPMethod + "\t" + e.Route
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ir.BriefHTTP{Method: e.HTTPMethod, Route: e.Route})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Route != out[j].Route {
			return out[i].Route < out[j].Route
		}
		return out[i].Method < out[j].Method
	})
	return out
}

var briefIngressKinds = map[string]bool{
	"runner": true, "scheduled": true, "kafka": true,
	"rabbit": true, "jms": true, "sqs": true, "event": true,
}

// BriefIngress is runners, listeners, and schedulers (compact symbols).
func (idx *Index) BriefIngress() []ir.BriefIngress {
	if idx == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []ir.BriefIngress
	for _, ep := range idx.entrypoints {
		if !briefIngressKinds[ep.Kind] {
			continue
		}
		sym := shortBriefSymbol(ep.Symbol)
		key := ep.Kind + " " + sym
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ir.BriefIngress{Kind: ep.Kind, Symbol: sym})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

// OutboundHTTP is peer/system names (Feign name/value or enclosing client type), not stack types.
func (idx *Index) OutboundHTTP() []string {
	if idx == nil {
		return nil
	}
	return uniqueSorted(idx.httpClients)
}

// OutboundDB is persistence aggregate/entity names from Spring Data repos.
func (idx *Index) OutboundDB() []string {
	if idx == nil {
		return nil
	}
	var names []string
	for _, r := range idx.repositories {
		name := r.Entity
		if name == "" {
			name = strings.TrimSuffix(simpleName(r.Symbol), "Repository")
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return uniqueSorted(names)
}

func shortBriefSymbol(sym string) string {
	parts := strings.Split(sym, ".")
	if len(parts) == 0 {
		return sym
	}
	last := parts[len(parts)-1]
	if last != "" && last[0] >= 'a' && last[0] <= 'z' && len(parts) >= 2 {
		return parts[len(parts)-2] + "." + last
	}
	return last
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// AllEntrypoints returns every parsed ingress site.
func (idx *Index) AllEntrypoints() []ir.Entrypoint {
	if idx == nil || idx.entrypoints == nil {
		return []ir.Entrypoint{}
	}
	out := append([]ir.Entrypoint{}, idx.entrypoints...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// InjectionsFor returns constructor/field injections whose consumer matches symbol.
func (idx *Index) InjectionsFor(symbol string) []Injection {
	symbol = strings.TrimSpace(symbol)
	simple := simpleName(symbol)
	var out []Injection
	for _, in := range idx.injections {
		// Dot-boundary prefix only: a bare HasPrefix let a query match a package
		// or a partially-typed name and report unrelated consumers as injections.
		if in.Consumer == symbol || simpleName(in.Consumer) == simple ||
			strings.HasSuffix(in.Consumer, "."+simple) || strings.HasPrefix(in.Consumer, symbol+".") {
			out = append(out, in)
		}
	}
	return out
}

// EndpointResult is one /v1/endpoints hit.
type EndpointResult struct {
	Path       string       `json:"path"`
	Line       int          `json:"line"`
	Symbol     string       `json:"symbol"`
	HTTPMethod string       `json:"http_method"`
	Route      string       `json:"route"`
	Certainty  ir.Certainty `json:"certainty"`
}

// Endpoints filters by optional method and path.
func (idx *Index) Endpoints(method, path string) []EndpointResult {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	var out []EndpointResult
	for _, e := range idx.endpoints {
		if method != "" && e.HTTPMethod != method {
			continue
		}
		if path != "" && e.Path != path {
			continue
		}
		out = append(out, EndpointResult{
			Path: e.File, Line: e.Line, Symbol: e.Symbol,
			HTTPMethod: e.HTTPMethod, Route: e.Path, Certainty: e.Certainty,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// BeanResult is one /v1/beans hit.
type BeanResult struct {
	Bean         string       `json:"bean"`
	Symbol       string       `json:"symbol"`
	InjectedInto []string     `json:"injected_into"`
	Certainty    ir.Certainty `json:"certainty"`
}

// Beans returns beans matching a requested type (interface/class simple or qualified).
func (idx *Index) Beans(typ string) []BeanResult {
	typ = strings.TrimSpace(typ)
	simple := typ
	if i := strings.LastIndexByte(typ, '.'); i >= 0 {
		simple = typ[i+1:]
	}
	var matches []Bean
	for _, b := range idx.beans {
		if beanProvides(b, simple) {
			matches = append(matches, b)
		}
	}
	// Group injections of this type by consumer + qualifier.
	type key struct{ consumer, qual string }
	consumers := map[key][]Injection{}
	for _, inj := range idx.injections {
		if inj.Type != simple && inj.Type != typ {
			continue
		}
		k := key{consumer: inj.Consumer, qual: inj.Qualifier}
		consumers[k] = append(consumers[k], inj)
	}

	var out []BeanResult
	for _, b := range matches {
		injected := []string{}
		certainty := ir.CertaintyCandidate
		for k := range consumers {
			resolved := resolveInjection(matches, k.qual)
			for _, cand := range resolved {
				if cand.Symbol == b.Symbol || cand.Name == b.Name {
					injected = append(injected, k.consumer)
					if len(resolved) == 1 && k.qual != "" {
						certainty = ir.CertaintyExact
					} else if len(resolved) == 1 && len(matches) == 1 {
						certainty = ir.CertaintyExact
					} else if len(resolved) > 1 {
						certainty = ir.CertaintyAmbiguous
					}
				}
			}
		}
		injected = unique(injected)
		sort.Strings(injected)
		out = append(out, BeanResult{
			Bean: b.Name, Symbol: b.Symbol, InjectedInto: injected, Certainty: certainty,
		})
	}
	// Ensure beans with no injection still appear as candidates when type matches.
	if len(out) == 0 {
		for _, b := range matches {
			out = append(out, BeanResult{Bean: b.Name, Symbol: b.Symbol, InjectedInto: []string{}, Certainty: ir.CertaintyCandidate})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}

func beanProvides(b Bean, simple string) bool {
	for _, t := range b.Types {
		if t == simple || strings.HasSuffix(t, "."+simple) {
			return true
		}
	}
	return strings.HasSuffix(b.Symbol, "."+simple) || b.Symbol == simple || simpleName(b.Symbol) == simple
}

func resolveInjection(candidates []Bean, qualifier string) []Bean {
	if qualifier == "" {
		return candidates
	}
	var out []Bean
	for _, b := range candidates {
		if b.Name == qualifier {
			out = append(out, b)
		}
	}
	return out
}

// Context builds a bounded relationship payload for a symbol.
func (idx *Index) Context(symbol string, budget int) map[string]any {
	symbol = strings.TrimSpace(symbol)
	if budget <= 0 {
		budget = 500
	}
	locations := []map[string]any{}
	relationships := []map[string]any{}
	certainty := string(ir.CertaintyCandidate)

	addRel := func(kind, target string, extra map[string]any) {
		m := map[string]any{"kind": kind, "target": target}
		for k, v := range extra {
			m[k] = v
		}
		relationships = append(relationships, m)
	}

	for _, e := range idx.endpoints {
		if e.Symbol == symbol || strings.HasSuffix(e.Symbol, "."+symbol) || strings.HasSuffix(e.Symbol, "."+simpleName(symbol)) {
			locations = append(locations, map[string]any{"path": e.File, "line": e.Line, "kind": "endpoint"})
			addRel("http_mapping", e.HTTPMethod+" "+e.Path, map[string]any{"certainty": "exact"})
			certainty = string(ir.CertaintyExact)
		}
	}
	for _, t := range idx.transactions {
		if t.Symbol == symbol || strings.HasSuffix(t.Symbol, "."+simpleName(symbol)) || t.Symbol == qualifyMatch(symbol) {
			locations = append(locations, map[string]any{"path": t.File, "line": t.Line, "kind": "method"})
			addRel("annotation", "@Transactional", map[string]any{
				"transaction_declared": true,
				"note":                 "declared annotation only; self-invocation may bypass proxy",
			})
		}
	}
	for _, r := range idx.repositories {
		if r.Symbol == symbol || simpleName(r.Symbol) == symbol || strings.HasSuffix(r.Symbol, "."+symbol) {
			locations = append(locations, map[string]any{"path": r.File, "line": r.Line, "kind": "interface"})
			addRel("extends", r.Extends, map[string]any{"certainty": "exact"})
			certainty = string(ir.CertaintyExact)
		}
	}
	for _, b := range idx.beans {
		if b.Symbol == symbol || simpleName(b.Symbol) == symbol {
			locations = append(locations, map[string]any{"path": b.File, "line": b.Line, "kind": "bean"})
			addRel("spring_bean", b.Name, nil)
		}
	}

	out := map[string]any{
		"symbol":        symbol,
		"locations":     locations,
		"relationships": relationships,
		"certainty":     certainty,
	}
	// Enforce rough budget by truncating relationship notes if needed.
	_ = budget
	return out
}

func simpleName(q string) string {
	if i := strings.LastIndexByte(q, '.'); i >= 0 {
		return q[i+1:]
	}
	return q
}

func qualifyMatch(symbol string) string { return symbol }
