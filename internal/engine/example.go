package engine

import (
	"strconv"
	"strings"
)

// Example is one copy-pasteable command plus a short why.
type Example struct {
	Cmd string
	Why string
}

// Examples returns up to three workspace-grounded invocations for cmd.
func (e *Engine) Examples(cmd string) []Example {
	s := e.sample()
	switch cmd {
	case "brief":
		out := []Example{{Cmd: "cxj brief", Why: "kind, versions, main size, depth-1 pkgs, ingress, outbound peers"}}
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj trail " + quote(s.typ), Why: "one hop around a real type"})
		}
		if s.key != "" {
			out = append(out, Example{Cmd: "cxj keys", Why: "config names only; no values"})
		}
		return cap3(out)
	case "find":
		var out []Example
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj find " + quote(s.typ), Why: "exact/prefix/fuzzy symbol search"})
		}
		if s.method != "" {
			out = append(out, Example{Cmd: "cxj find " + quote(simple(s.method)), Why: "method name search"})
		}
		if s.route != "" {
			out = append(out, Example{Cmd: "cxj find " + quote(s.httpMethod+" "+s.route), Why: "plain-text route search"})
		}
		return orEmpty(out, Example{Cmd: "cxj find SomeType", Why: "no Java symbols in --root"})
	case "show":
		var out []Example
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj show " + quote(s.typ), Why: "type body, capped at 80 lines"})
		}
		if s.method != "" {
			out = append(out, Example{Cmd: "cxj show " + quote(s.method), Why: "method body only"})
		}
		return orEmpty(out, Example{Cmd: "cxj show SomeType", Why: "no Java symbols in --root"})
	case "outline":
		var out []Example
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj outline " + quote(s.typ), Why: "types and methods, no bodies"})
		}
		if s.path != "" {
			out = append(out, Example{Cmd: "cxj outline " + quote(s.path), Why: "same outline by file path"})
		}
		return orEmpty(out, Example{Cmd: "cxj outline SomeType", Why: "no Java files in --root"})
	case "map":
		return []Example{{Cmd: "cxj map", Why: "package → type → file"}}
	case "trail":
		var out []Example
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj trail " + quote(s.typ), Why: "http, injects, callers, callees, tests"})
		}
		if s.method != "" {
			out = append(out, Example{Cmd: "cxj trail " + quote(s.method), Why: "method-level neighborhood"})
		}
		return orEmpty(out, Example{Cmd: "cxj trail SomeType", Why: "no Java symbols in --root"})
	case "callers":
		var out []Example
		if s.called != "" {
			out = append(out, Example{Cmd: "cxj callers " + quote(s.called), Why: "use sites only (not the def)"})
		}
		if s.method != "" && s.method != s.called {
			out = append(out, Example{Cmd: "cxj callers " + quote(s.method), Why: "who calls this method"})
		}
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj callers " + quote(s.typ), Why: "types are often empty; prefer a method"})
		}
		return orEmpty(out, Example{Cmd: "cxj callers someMethod", Why: "no Java symbols in --root"})
	case "callees":
		var out []Example
		if s.method != "" {
			out = append(out, Example{Cmd: "cxj callees " + quote(s.method), Why: "outbound calls from this method"})
		}
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj callees " + quote(s.typ), Why: "outbound calls from this type"})
		}
		return orEmpty(out, Example{Cmd: "cxj callees someMethod", Why: "no Java symbols in --root"})
	case "references":
		var out []Example
		if s.typ != "" {
			out = append(out, Example{Cmd: "cxj references " + quote(s.typ), Why: "def, impl, and use sites"})
		}
		if s.method != "" {
			out = append(out, Example{Cmd: "cxj references " + quote(s.method), Why: "every mention of this method"})
		}
		return orEmpty(out, Example{Cmd: "cxj references SomeType", Why: "no Java symbols in --root"})
	case "keys":
		return []Example{{Cmd: "cxj keys", Why: "config key names only; secrets flagged, no values"}}
	case "endpoints":
		out := []Example{{Cmd: "cxj endpoints", Why: "all HTTP mappings"}}
		if s.route != "" {
			out = append(out, Example{
				Cmd: "cxj endpoints --method " + s.httpMethod + " --path " + quote(s.route),
				Why: "filter to one real route",
			})
		}
		return cap3(out)
	case "beans":
		if s.bean != "" {
			return []Example{{Cmd: "cxj beans --type " + quote(s.bean), Why: "who provides/injects this type"}}
		}
		return []Example{{Cmd: "cxj beans --type SomeRepo", Why: "no Spring injections in --root"}}
	case "config":
		if s.key != "" {
			out := []Example{{Cmd: "cxj config --key " + quote(s.key), Why: "resolved value; secrets redacted"}}
			if s.profile != "" {
				out = append(out, Example{Cmd: "cxj config --key " + quote(s.key) + " --profile " + quote(s.profile), Why: "same key in a named profile"})
			}
			out = append(out, Example{Cmd: "cxj keys", Why: "list names first if you are unsure"})
			return cap3(out)
		}
		return []Example{{Cmd: "cxj keys", Why: "no config keys in --root; start here"}}
	case "context":
		if s.typ != "" {
			return []Example{{Cmd: "cxj context --symbol " + quote(s.typ), Why: "bounded snippet; prefer show/trail"}}
		}
		return []Example{{Cmd: "cxj context --symbol SomeType", Why: "no Java symbols in --root"}}
	case "status":
		return []Example{
			{Cmd: "cxj brief", Why: "prefer this over status for orientation"},
			{Cmd: "cxj status", Why: "file counts and heap only"},
		}
	case "ask":
		var out []Example
		if s.called != "" {
			out = append(out, Example{Cmd: "cxj ask " + quote("who calls "+simple(s.called)), Why: "routes to callers"})
		} else if s.method != "" {
			out = append(out, Example{Cmd: "cxj ask " + quote("who calls "+simple(s.method)), Why: "routes to callers"})
		}
		out = append(out, Example{Cmd: "cxj ask " + quote("what is this project"), Why: "routes to brief"})
		return cap3(out)
	case "install":
		return []Example{
			{Cmd: "cxj install", Why: "project .agents/skills + .claude symlink"},
			{Cmd: "cxj install -g", Why: "global ~/.agents/skills + symlink; /usr/local/bin/cxj"},
		}
	case "version":
		return []Example{{Cmd: "cxj version", Why: "print the installed cxj version"}}
	case "update":
		return []Example{
			{Cmd: "cxj update --check", Why: "compare to the latest GitHub release"},
			{Cmd: "cxj update", Why: "replace this binary with the latest release"},
		}
	default:
		return []Example{{Cmd: "cxj brief", Why: "start here"}}
	}
}

type sample struct {
	typ, method, called, path, key, profile, httpMethod, route, bean string
}

func (e *Engine) sample() sample {
	var s sample
	for _, ep := range e.spring.AllEntrypoints() {
		if s.typ == "" {
			s.typ = typeName(ep.Symbol)
		}
		if s.method == "" && isMethodSymbol(ep.Symbol) {
			s.method = ep.Symbol
		}
		if ep.Kind == "http" && s.route == "" && ep.Detail != "" {
			s.httpMethod, s.route, _ = strings.Cut(ep.Detail, " ")
		}
	}
	for _, ep := range e.spring.Endpoints("", "") {
		if s.route == "" {
			s.httpMethod, s.route = ep.HTTPMethod, ep.Route
		}
		if s.method == "" && isMethodSymbol(ep.Symbol) {
			s.method = ep.Symbol
		}
		if s.typ == "" {
			s.typ = typeName(ep.Symbol)
		}
		break
	}
	for _, p := range e.Map().Packages {
		for _, t := range p.Types {
			if isTestName(t.Name) || isTestPath(t.Path) {
				continue
			}
			if s.typ == "" {
				s.typ = t.Name
			}
			if s.path == "" {
				s.path = t.Path
			}
			if s.typ != "" && s.path != "" {
				break
			}
		}
		if s.typ != "" && s.path != "" {
			break
		}
	}
	if s.typ != "" {
		if d, _, ok := e.resolveDef(s.typ); ok {
			if s.path == "" {
				s.path = d.Path
			}
			ol := e.Outline(s.typ)
			for _, t := range ol.Types {
				for _, m := range t.Members {
					if m.Kind != "method" {
						continue
					}
					qual := t.Name + "." + m.Name
					if s.method == "" {
						s.method = qual
					}
					if s.called == "" && len(e.Callers(qual).Results) > 0 {
						s.called = qual
					}
					if s.method != "" && s.called != "" {
						break
					}
				}
			}
		}
		for _, in := range e.spring.InjectionsFor(s.typ) {
			if in.Type != "" {
				s.bean = in.Type
				break
			}
		}
	}
	if s.called == "" {
		s.called = s.method
	}
	for _, k := range e.Keys().Keys {
		if k.Secret {
			continue
		}
		s.key = k.Key
		if len(k.Profiles) > 0 && k.Profiles[0] != "" && k.Profiles[0] != "default" {
			s.profile = k.Profiles[0]
		}
		break
	}
	if s.profile == "" {
		for _, p := range e.Keys().Profiles {
			if p != "" && p != "default" {
				s.profile = p
				break
			}
		}
	}
	return s
}

func typeName(sym string) string {
	sym = strings.TrimSpace(sym)
	if sym == "" {
		return ""
	}
	if isMethodSymbol(sym) {
		if i := strings.LastIndexByte(sym, '.'); i > 0 {
			return simple(sym[:i])
		}
	}
	return simple(sym)
}

func isMethodSymbol(sym string) bool {
	i := strings.LastIndexByte(sym, '.')
	if i < 0 || i+1 >= len(sym) {
		return false
	}
	c := sym[i+1]
	return c >= 'a' && c <= 'z'
}

func simple(sym string) string {
	if i := strings.LastIndexByte(sym, '.'); i >= 0 {
		return sym[i+1:]
	}
	return sym
}

func quote(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " \t'\"") {
		return strconv.Quote(s)
	}
	return s
}

func cap3(in []Example) []Example {
	if len(in) > 3 {
		return in[:3]
	}
	return in
}

func orEmpty(in []Example, fallback Example) []Example {
	if len(in) == 0 {
		return []Example{fallback}
	}
	return cap3(in)
}

// StaticExamples is used when a command does not open the index.
func StaticExamples(cmd string) []Example {
	switch cmd {
	case "install":
		return []Example{
			{Cmd: "cxj install", Why: "project .agents/skills + .claude symlink"},
			{Cmd: "cxj install -g", Why: "global ~/.agents/skills + symlink; /usr/local/bin/cxj"},
		}
	case "version":
		return []Example{{Cmd: "cxj version", Why: "print the installed cxj version"}}
	case "update":
		return []Example{
			{Cmd: "cxj update --check", Why: "compare to the latest GitHub release"},
			{Cmd: "cxj update", Why: "replace this binary with the latest release"},
		}
	default:
		return []Example{{Cmd: "cxj brief", Why: "start here"}}
	}
}
