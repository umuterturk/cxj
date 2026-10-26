package engine

import (
	"sort"
	"strings"

	"example.com/cx/internal/ir"
)

// Brief is the compact workspace orientation card.
func (e *Engine) Brief() ir.BriefReply {
	pkgTypes := map[string]int{}
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
		pkgTypes[f.Package] += n
	}
	pkgs := make([]string, 0, len(pkgTypes))
	for p := range pkgTypes {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	outPkgs := make([]ir.BriefPackage, 0, len(pkgs))
	for _, p := range pkgs {
		outPkgs = append(outPkgs, ir.BriefPackage{Name: p, Types: pkgTypes[p]})
	}

	entry := e.spring.AllEntrypoints()
	kind := "java"
	if len(entry) > 0 {
		kind = "spring"
	}
	if entry == nil {
		entry = []ir.Entrypoint{}
	}
	return ir.BriefReply{
		Kind:        kind,
		JavaFiles:   e.stats.IndexedJavaFiles,
		Types:       types,
		Packages:    outPkgs,
		Entrypoints: entry,
		Profiles:    e.cfg.Profiles(),
		TestClasses: testClasses,
	}
}

func isTestPath(p string) bool {
	return strings.Contains(p, "/src/test/") || strings.HasPrefix(p, "src/test/")
}

func isTestName(name string) bool {
	return strings.HasSuffix(name, "Test") || strings.HasSuffix(name, "Tests")
}
