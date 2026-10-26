package engine

import (
	"sort"
	"strings"

	"example.com/cx/internal/ir"
)

// Brief is the compact workspace orientation card.
// Packages are limited to entrypoint roots + one child segment (use map for the full tree),
// split by main vs test source roots.
func (e *Engine) Brief() ir.BriefReply {
	mainTypes := map[string]int{}
	testTypes := map[string]int{}
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
		types += n
		if f.Package == "" || n == 0 {
			continue
		}
		if isTestPath(f.Path) {
			testTypes[f.Package] += n
		} else {
			mainTypes[f.Package] += n
		}
	}

	entry := e.spring.AllEntrypoints()
	kind := "java"
	if len(entry) > 0 {
		kind = "spring"
	}
	if entry == nil {
		entry = []ir.Entrypoint{}
	}

	roots := briefRoots(entry, mainTypes)
	if len(roots) == 0 {
		roots = inferShallowRoots(testTypes)
	}
	outPkgs := append(
		selectBriefPackages(mainTypes, roots, "main"),
		selectBriefPackages(testTypes, roots, "test")...,
	)

	return ir.BriefReply{
		Kind:         kind,
		JavaFiles:    e.stats.IndexedJavaFiles,
		Types:        types,
		Packages:     outPkgs,
		PackageTotal: len(mainTypes) + len(testTypes),
		Entrypoints:  entry,
		Profiles:     e.cfg.Profiles(),
		TestClasses:  testClasses,
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
	keep := map[string]bool{}
	for _, root := range roots {
		if _, ok := pkgTypes[root]; ok {
			keep[root] = true
		}
		prefix := root + "."
		for p := range pkgTypes {
			if !strings.HasPrefix(p, prefix) {
				continue
			}
			rest := p[len(prefix):]
			if rest != "" && !strings.Contains(rest, ".") {
				keep[p] = true
			}
		}
	}
	names := make([]string, 0, len(keep))
	for p := range keep {
		names = append(names, p)
	}
	sort.Strings(names)
	out := make([]ir.BriefPackage, 0, len(names))
	for _, p := range names {
		out = append(out, ir.BriefPackage{Name: p, Scope: scope, Types: pkgTypes[p]})
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

func isTestPath(p string) bool {
	return strings.Contains(p, "/src/test/") || strings.HasPrefix(p, "src/test/")
}

func isTestName(name string) bool {
	return strings.HasSuffix(name, "Test") || strings.HasSuffix(name, "Tests")
}
