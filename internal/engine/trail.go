package engine

import (
	"strings"

	"example.com/cx/internal/ir"
)

const trailCap = 5

// Trail returns a capped one-hop neighborhood around symbol.
func (e *Engine) Trail(symbol string) ir.TrailReply {
	d, resolved, ok := e.resolveDef(symbol)
	if !ok {
		return ir.TrailReply{Symbol: symbol, Resolved: resolved}
	}
	query := d.Qualified
	if query == "" {
		query = d.Name
	}
	out := ir.TrailReply{
		Symbol:   d.Qualified,
		Kind:     string(d.Kind),
		Path:     d.Path,
		Line:     d.Span.Line,
		EndLine:  d.Span.EndLine,
		Resolved: resolved,
		HTTP:     []ir.TrailHit{},
		Injects:  []ir.TrailHit{},
		Callers:  []ir.TrailHit{},
		Callees:  []ir.TrailHit{},
		Supers:   []ir.TrailHit{},
		Subtypes: []ir.TrailHit{},
		Tests:    []ir.TrailHit{},
	}
	for _, ep := range e.spring.Endpoints("", "") {
		if symbolMatch(ep.Symbol, query, d.Name) {
			out.HTTP = appendHit(out.HTTP, ir.TrailHit{
				Path: ep.Path, Line: ep.Line, Symbol: ep.Symbol,
				Detail: ep.HTTPMethod + " " + ep.Route, Certainty: string(ep.Certainty),
			})
		}
	}
	for _, in := range e.spring.InjectionsFor(query) {
		out.Injects = appendHit(out.Injects, ir.TrailHit{
			Path: in.File, Line: in.Line, Symbol: in.Type, Certainty: string(ir.CertaintyCandidate),
		})
	}
	for _, r := range e.Callers(query).Results {
		out.Callers = appendHit(out.Callers, refHit(r))
	}
	for _, r := range e.Callees(query).Results {
		out.Callees = appendHit(out.Callees, refHit(r))
	}
	supers, subs := e.syms.Heritage(query)
	for _, ed := range supers {
		out.Supers = appendHit(out.Supers, ir.TrailHit{
			Path: ed.Path, Line: ed.Span.Line, Symbol: ed.To, Detail: ed.Kind, Certainty: string(ed.Certainty),
		})
	}
	for _, ed := range subs {
		out.Subtypes = appendHit(out.Subtypes, ir.TrailHit{
			Path: ed.Path, Line: ed.Span.Line, Symbol: ed.From, Detail: ed.Kind, Certainty: string(ed.Certainty),
		})
	}
	for _, td := range e.syms.TestTypes(ownerTypeName(d)) {
		if td.Qualified == d.Qualified {
			continue
		}
		out.Tests = appendHit(out.Tests, ir.TrailHit{
			Path: td.Path, Line: td.Span.Line, Symbol: td.Qualified,
		})
	}
	return out
}

func refHit(r ir.RefResult) ir.TrailHit {
	return ir.TrailHit{
		Path: r.Path, Line: r.Line, Symbol: r.Target, Certainty: string(r.Certainty),
	}
}

func appendHit(dst []ir.TrailHit, h ir.TrailHit) []ir.TrailHit {
	if len(dst) >= trailCap {
		return dst
	}
	return append(dst, h)
}

func symbolMatch(got, qualified, simple string) bool {
	if got == qualified || got == simple {
		return true
	}
	// Dot-boundary only. A bare Contains fallback was the single worst source of
	// wrong `trail` output: querying ...PaymentController.pay matched every
	// endpoint in the package, because the package is named `paymentapi` and that
	// contains `pay`. It also made the correct route disappear, since every
	// endpoint became a candidate and the trail cap then truncated the real one.
	return strings.HasPrefix(got, qualified+".") || strings.HasSuffix(got, "."+simple)
}

// ownerTypeName returns the simple name of the type a definition belongs to, so
// the tests section asks "what covers this class" instead of "what type name
// contains this method name".
func ownerTypeName(d ir.Definition) string {
	switch d.Kind {
	case ir.SymbolMethod, ir.SymbolConstructor, ir.SymbolField:
		if d.Owner != "" {
			if i := strings.LastIndexByte(d.Owner, '.'); i >= 0 {
				return d.Owner[i+1:]
			}
			return d.Owner
		}
	}
	return d.Name
}
