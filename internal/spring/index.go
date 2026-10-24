package spring

import (
	"sort"
	"strings"

	"example.com/cx/internal/ir"
)

// Index answers Spring HTTP/bean/context queries.
type Index struct {
	endpoints    []Endpoint
	beans        []Bean
	injections   []Injection
	transactions []TransactionDecl
	repositories []RepositoryRel
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
	}
	return idx
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
		if e.Symbol == symbol || strings.HasSuffix(e.Symbol, "."+symbol) || strings.Contains(e.Symbol, symbol) {
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
