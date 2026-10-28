package engine

import (
	"sort"
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
)

// Brief is the compact workspace orientation card.
// Packages are entrypoint roots (exact counts) plus depth-1 children with subtree type counts
// (use map for the full tree).
func (e *Engine) Brief() ir.BriefReply {
	mainTypes := map[string]int{}
	mainFiles := 0
	types := 0
	testClasses := 0
	for _, f := range e.facts {
		n := 0
		for _, d := range f.Definitions {
			switch d.Kind {
			case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
				n++
				if isTestPath(d.Path) || isTestName(d.Name) {
					testClasses++
				}
			}
		}
		if isTestPath(f.Path) {
			continue
		}
		mainFiles++
		types += n
		if f.Package != "" && n > 0 {
			mainTypes[f.Package] += n
		}
	}

	kind := "java"
	if e.spring != nil && e.spring.HasSpringBoot() {
		kind = "spring"
	}

	roots := briefRoots(e.spring.AllEntrypoints(), mainTypes)
	pkgs := selectBriefPackages(mainTypes, roots, "main")
	if pkgs == nil {
		pkgs = []ir.BriefPackage{}
	}

	var profiles []string
	if e.cfg != nil {
		profiles = e.cfg.Profiles()
	}

	return ir.BriefReply{
		Kind:         kind,
		Java:         e.javaVer,
		SpringBoot:   e.bootVer,
		MainFiles:    mainFiles,
		Types:        types,
		TestClasses:  testClasses,
		Packages:     pkgs,
		Profiles:     profiles,
		HTTP:         e.spring.BriefHTTP(),
		Ingress:      e.spring.BriefIngress(),
		OutboundHTTP: e.spring.OutboundHTTP(),
		OutboundDB:   e.spring.OutboundDB(),
	}
}

// briefRoots prefers Spring Boot application packages; otherwise the shallowest main packages.
func briefRoots(entry []ir.Entrypoint, mainTypes map[string]int) []string {
	seen := map[string]bool{}
	var roots []string
	for _, ep := range entry {
		if ep.Kind != "boot" {
			continue
		}
		pkg := packageOfSymbol(ep.Symbol)
		if pkg == "" || seen[pkg] {
			continue
		}
		seen[pkg] = true
		roots = append(roots, pkg)
	}
	if len(roots) > 0 {
		sort.Strings(roots)
		return roots
	}
	return inferShallowRoots(mainTypes)
}

func inferShallowRoots(pkgTypes map[string]int) []string {
	if len(pkgTypes) == 0 {
		return nil
	}
	minSeg := -1
	for p := range pkgTypes {
		n := packageSegments(p)
		if minSeg < 0 || n < minSeg {
			minSeg = n
		}
	}
	var roots []string
	for p := range pkgTypes {
		if packageSegments(p) == minSeg {
			roots = append(roots, p)
		}
	}
	sort.Strings(roots)
	return roots
}

func selectBriefPackages(pkgTypes map[string]int, roots []string, scope string) []ir.BriefPackage {
	if len(roots) == 0 || len(pkgTypes) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, root := range roots {
		if n, ok := pkgTypes[root]; ok {
			counts[root] = n // exact root package only
		}
		prefix := root + "."
		for p, n := range pkgTypes {
			if !strings.HasPrefix(p, prefix) {
				continue
			}
			rest := p[len(prefix):]
			if rest == "" {
				continue
			}
			seg := rest
			if i := strings.IndexByte(rest, '.'); i >= 0 {
				seg = rest[:i]
			}
			counts[root+"."+seg] += n // subtree rollup under depth-1
		}
	}
	names := make([]string, 0, len(counts))
	for p := range counts {
		names = append(names, p)
	}
	sort.Strings(names)
	out := make([]ir.BriefPackage, 0, len(names))
	for _, p := range names {
		out = append(out, ir.BriefPackage{Name: p, Scope: scope, Types: counts[p]})
	}
	return out
}

func packageOfSymbol(sym string) string {
	i := strings.LastIndex(sym, ".")
	if i <= 0 {
		return ""
	}
	return sym[:i]
}

func packageSegments(pkg string) int {
	if pkg == "" {
		return 0
	}
	return strings.Count(pkg, ".") + 1
}

func isTestPath(p string) bool { return ir.IsTestPath(p) }

func isTestName(name string) bool {
	return strings.HasSuffix(name, "Test") || strings.HasSuffix(name, "Tests")
}
