package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
)

// Show prints the source span of a symbol, hard-capped at maxLines.
func (e *Engine) Show(symbol string, maxLines int) ir.ShowReply {
	if maxLines <= 0 {
		maxLines = ir.DefaultShowLines
	}
	d, resolved, ok := e.resolveDef(symbol)
	if !ok {
		return ir.ShowReply{Resolved: resolved}
	}
	body, truncated, endLine := readSpan(e.root, d.Path, d.Span, maxLines)
	return ir.ShowReply{
		Path:      d.Path,
		Line:      d.Span.Line,
		EndLine:   endLine,
		Symbol:    d.Qualified,
		Kind:      string(d.Kind),
		Truncated: truncated,
		Body:      body,
		Resolved:  resolved,
	}
}

// Outline returns the structural summary of a file (by path or by symbol).
func (e *Engine) Outline(query string) ir.OutlineReply {
	query = strings.TrimSpace(query)
	if query == "" {
		return ir.OutlineReply{}
	}
	facts, ok := e.lookupFacts(query)
	if !ok {
		if d, _, found := e.resolveDef(query); found {
			facts, ok = e.lookupFacts(d.Path)
		}
	}
	if !ok {
		return ir.OutlineReply{}
	}
	return outlineFacts(facts)
}

// Map returns a compact package → type → file index.
func (e *Engine) Map() ir.MapReply {
	type key struct{ pkg, name, path string }
	seen := map[key]ir.MapType{}
	for _, f := range e.facts {
		for _, d := range f.Definitions {
			switch d.Kind {
			case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
			default:
				continue
			}
			k := key{pkg: f.Package, name: d.Name, path: d.Path}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = ir.MapType{Name: d.Name, Kind: string(d.Kind), Path: d.Path, Line: d.Span.Line}
		}
	}
	byPkg := map[string][]ir.MapType{}
	for k, t := range seen {
		byPkg[k.pkg] = append(byPkg[k.pkg], t)
	}
	pkgs := make([]string, 0, len(byPkg))
	for p := range byPkg {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	out := ir.MapReply{Packages: make([]ir.MapPackage, 0, len(pkgs))}
	for _, p := range pkgs {
		types := byPkg[p]
		sort.Slice(types, func(i, j int) bool {
			if types[i].Name != types[j].Name {
				return types[i].Name < types[j].Name
			}
			return types[i].Path < types[j].Path
		})
		out.Packages = append(out.Packages, ir.MapPackage{Name: p, Types: types})
	}
	return out
}

func (e *Engine) resolveDef(symbol string) (ir.Definition, string, bool) {
	if d, ok := e.syms.Lookup(symbol); ok {
		resolved := ""
		if !strings.EqualFold(d.Name, symbol) && !strings.EqualFold(d.Qualified, symbol) &&
			!strings.HasSuffix(d.Qualified, "."+symbol) {
			resolved = d.Name
		}
		return d, resolved, true
	}
	if name, ok := resolveSymbolName(e.syms.Search(symbol, 5)); ok {
		if d, ok := e.syms.Lookup(name); ok {
			return d, name, true
		}
	}
	return ir.Definition{}, "", false
}

func (e *Engine) lookupFacts(query string) (ir.FileFacts, bool) {
	norm := strings.ReplaceAll(query, "\\", "/")
	for _, f := range e.facts {
		if f.Path == norm || strings.HasSuffix(f.Path, "/"+norm) || strings.HasSuffix(norm, "/"+f.Path) {
			return f, true
		}
		if filepath.Base(f.Path) == filepath.Base(norm) && strings.Contains(norm, ".") {
			if strings.HasSuffix(norm, filepath.Base(f.Path)) {
				return f, true
			}
		}
	}
	return ir.FileFacts{}, false
}

func outlineFacts(f ir.FileFacts) ir.OutlineReply {
	var types []ir.OutlineType
	byOwner := map[string][]ir.OutlineMember{}
	for _, d := range f.Definitions {
		switch d.Kind {
		case ir.SymbolMethod, ir.SymbolConstructor, ir.SymbolField:
			byOwner[d.Owner] = append(byOwner[d.Owner], ir.OutlineMember{
				Name:      d.Name,
				Kind:      string(d.Kind),
				Signature: d.Signature,
				Line:      d.Span.Line,
			})
		}
	}
	for _, d := range f.Definitions {
		switch d.Kind {
		case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
			members := byOwner[d.Qualified]
			if members == nil {
				members = []ir.OutlineMember{}
			}
			types = append(types, ir.OutlineType{
				Name:    d.Name,
				Kind:    string(d.Kind),
				Line:    d.Span.Line,
				EndLine: d.Span.EndLine,
				Members: members,
			})
		}
	}
	if types == nil {
		types = []ir.OutlineType{}
	}
	return ir.OutlineReply{Path: f.Path, Package: f.Package, Types: types}
}

func readSpan(root, rel string, span ir.Span, maxLines int) (body string, truncated bool, endLine int) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false, span.EndLine
	}
	start, end := int(span.StartByte), int(span.EndByte)
	if start < 0 {
		start = 0
	}
	if end > len(data) || end <= start {
		end = len(data)
	}
	if start >= len(data) {
		return "", false, span.EndLine
	}
	chunk := string(data[start:end])
	lines := strings.Split(chunk, "\n")
	endLine = span.EndLine
	if endLine <= 0 {
		endLine = span.Line + len(lines) - 1
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	return strings.Join(lines, "\n"), truncated, endLine
}
