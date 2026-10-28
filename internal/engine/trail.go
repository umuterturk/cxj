package engine

import (
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
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
	var http, injects, callers, callees, supers, subs, tests trailSection
	for _, ep := range e.spring.Endpoints("", "") {
		if symbolMatch(ep.Symbol, query, d.Name) {
			http.add(ir.TrailHit{
				Path: ep.Path, Line: ep.Line, Symbol: ep.Symbol,
				Detail: ep.HTTPMethod + " " + ep.Route, Certainty: string(ep.Certainty),
			})
		}
	}
	for _, in := range e.spring.InjectionsFor(query) {
		injects.add(ir.TrailHit{
			Path: in.File, Line: in.Line, Symbol: in.Type, Certainty: string(ir.CertaintyCandidate),
		})
	}
	for _, r := range e.Callers(query).Results {
		callers.add(refHit(r))
	}
	for _, r := range e.Callees(query).Results {
		callees.add(refHit(r))
	}
	hs, hsSubs := e.syms.Heritage(query)
	for _, ed := range hs {
		supers.add(ir.TrailHit{
			Path: ed.Path, Line: ed.Span.Line, Symbol: ed.To, Detail: ed.Kind, Certainty: string(ed.Certainty),
		})
	}
	for _, ed := range hsSubs {
		subs.add(ir.TrailHit{
			Path: ed.Path, Line: ed.Span.Line, Symbol: ed.From, Detail: ed.Kind, Certainty: string(ed.Certainty),
		})
	}
	for _, td := range e.syms.TestTypes(ownerTypeName(d)) {
		if td.Qualified == d.Qualified {
			continue
		}
		tests.add(ir.TrailHit{
			Path: td.Path, Line: td.Span.Line, Symbol: td.Qualified,
		})
	}
	return ir.TrailReply{
		Symbol:   d.Qualified,
		Kind:     string(d.Kind),
		Path:     d.Path,
		Line:     d.Span.Line,
		EndLine:  d.Span.EndLine,
		Resolved: resolved,
		Totals: ir.TrailTotals{
			HTTP: http.total, Injects: injects.total, Callers: callers.total,
			Callees: callees.total, Supers: supers.total, Subtypes: subs.total,
			Tests: tests.total,
		},
		HTTP:     http.list(),
		Injects:  injects.list(),
		Callers:  callers.list(),
		Callees:  callees.list(),
		Supers:   supers.list(),
		Subtypes: subs.list(),
		Tests:    tests.list(),
	}
}

// trailSection collects hits up to the cap while counting everything it saw, so
// the reply can say how much it truncated (FIX_PLAN A5): a full section and a
// capped one must not look identical.
type trailSection struct {
	hits  []ir.TrailHit
	total int
}

func (s *trailSection) add(h ir.TrailHit) {
	s.total++
	if len(s.hits) < trailCap {
		s.hits = append(s.hits, h)
	}
}

// list never returns nil: the JSON contract is an array, and `null` sections
// make every consumer branch.
func (s *trailSection) list() []ir.TrailHit {
	if s.hits == nil {
		return []ir.TrailHit{}
	}
	return s.hits
}

func refHit(r ir.RefResult) ir.TrailHit {
	return ir.TrailHit{
		Path: r.Path, Line: r.Line, Symbol: r.Target, Certainty: string(r.Certainty),
	}
}

func symbolMatch(got, qualified, simple string) bool {
	if got == qualified || got == simple {
		return true
	}
	// Dot-boundary only, and only in the owning direction: a class's card lists
	// its own routes, a method's card lists its own. A bare Contains fallback was
	// the single worst source of wrong `trail` output: querying
	// ...PaymentController.pay matched every endpoint in the package, because the
	// package is named `paymentapi` and that contains `pay`. It also made the
	// correct route disappear, since every endpoint became a candidate and the
	// trail cap then truncated the real one. A simple-name SUFFIX fallback was
	// the same bug wearing a tie: `trail …CardPaymentClient.pay` (a method with
	// no route) listed POST /v1/payments, because ...PaymentController.pay ends
	// in ".pay". Two cards for two methods cannot both own one route; a name
	// collision is settled by the ambiguity warning, never by guessing.
	return strings.HasPrefix(got, qualified+".")
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
