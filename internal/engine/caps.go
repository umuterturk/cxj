package engine

import (
	"sort"

	"example.com/cx/internal/config"
)

// Capabilities selects which sub-indexes Open builds. Every command needs a
// different subset, and before this existed every command paid for all of them:
// `cxj keys` parsed every Java file with three separate Tree-sitter grammars
// (definitions, Spring annotations, config bindings) and split every resource file
// into lines, to print config key names.
//
// The law is that nothing is cached or persisted, so the only way to be cheap on the
// second command is to do less work on the first. That makes "what does this command
// actually read" the most valuable fact in the process, so it is stated once here and
// derived from the command name rather than passed around.
type Capabilities uint16

const (
	// CapJava runs the Java extractor: definitions, call edges, occurrences.
	// Backs Engine.facts, which is what map/outline read.
	CapJava Capabilities = 1 << iota
	// CapSymbolIndex builds the symbol index over those facts. Split from CapJava
	// because `brief` reads the per-file facts (type counts, packages) and never
	// queries symbols; building the index for it is pure waste on the tool's most
	// common first call.
	CapSymbolIndex
	// CapSpring runs the Spring extractor over every in-scope Java file.
	CapSpring
	// CapSpringMain runs it over main sources only. This is what `brief` reads:
	// its HTTP/ingress/outbound sections describe the deployed app, and test
	// configs must not leak into them. It is a separate capability rather than a
	// filter at the call site because it changes what `brief` *says*, not just
	// how long it takes.
	CapSpringMain
	// CapConfig parses application.yml/properties into the config index.
	CapConfig
	// CapBindings runs the binding extractor over Java sources. It is a third
	// parse, paid for only by the commands that resolve config values.
	CapBindings
	// CapText line-indexes non-Java files for text search. `find` is the only
	// reader, and the work is proportional to every non-Java byte in the repo.
	CapText
	// CapVersions reads pom.xml/build.gradle for the Java and Spring Boot versions.
	CapVersions
)

// Command presets. Adding a command means adding a line to commandCaps; forgetting
// falls through to CapsAll, which is correct and merely slow.
const (
	// brief reads per-file facts and spring annotations from main sources only, plus
	// config profiles and the build-file versions. It never queries the symbol index.
	CapsBrief     = CapJava | CapSpringMain | CapConfig | CapVersions
	CapsSymbols   = CapJava | CapSymbolIndex
	CapsSearch    = CapJava | CapSymbolIndex | CapText
	CapsTrail     = CapJava | CapSymbolIndex | CapSpring
	CapsSpring    = CapSpring
	CapsConfigCmd = CapConfig | CapBindings
	CapsAll       = CapJava | CapSymbolIndex | CapSpring | CapConfig | CapBindings | CapText | CapVersions
)

func (c Capabilities) Has(want Capabilities) bool { return c&want == want }

// CapsFor is the single source of truth for what a command reads.
//
// Unknown and empty commands get everything on purpose: a new command that nobody
// registered returns correct output on its first run instead of an empty index, and
// the missing line shows up as a slow command rather than a wrong answer.
// TestEveryCommandDeclaresItsCapabilities guards the other direction, in the
// package that owns the command list.
func CapsFor(cmd string) Capabilities {
	if c, ok := commandCaps[cmd]; ok {
		return c
	}
	return CapsAll
}

// KnowsCommand reports whether cmd has an entry in the table.
func KnowsCommand(cmd string) bool {
	_, ok := commandCaps[cmd]
	return ok
}

// Commands lists the registered command names, sorted.
func Commands() []string {
	out := make([]string, 0, len(commandCaps))
	for c := range commandCaps {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

var commandCaps = map[string]Capabilities{
	// Full scope by design: TestClasses would read zero on main-only, and
	// TestBriefKeepsTestClasses guards that.
	"brief": CapsBrief,
	// Full scope: a text hit in src/test is a real answer for a search.
	"find":       CapsSearch,
	"show":       CapsSymbols,
	"outline":    CapsSymbols,
	"map":        CapsSymbols,
	"references": CapsSymbols,
	"callers":    CapsSymbols,
	"callees":    CapsSymbols,
	"trail":      CapsTrail,
	// Spring annotations carry their own file/line/symbol, so no Java parse.
	"endpoints": CapsSpring,
	"beans":     CapsSpring,
	"context":   CapsSpring,
	"keys":      CapsConfigCmd,
	"config":    CapsConfigCmd,
	// Composes several commands in one process.
	"ask": CapsAll,
	// Counters come off the scan, and nothing else is read.
	"status": 0,
}

// fileWork is the per-file slice of that plan. Resolving to zero is the cheap
// exit: the file is skipped before ReadFile, so neither the open nor the parse
// happens. Skipping the indexing step but still reading 850 files would win almost
// nothing.
type fileWork uint8

const (
	workJava fileWork = 1 << iota
	workSpring
	workBindings
	workConfig
	workText
	workVersions
)

func (o Options) workFor(relPath string, isJava bool) fileWork {
	if !isJava {
		var w fileWork
		if o.Caps.Has(CapVersions) && isBuildFileName(relPath) {
			w |= workVersions
		}
		// ParseIfConfig gates on IsConfigFile itself, so gating here is the same
		// decision made before opening the file instead of after.
		if o.Caps.Has(CapConfig) && config.IsConfigFile(relPath) {
			w |= workConfig
		}
		if o.Caps.Has(CapText) {
			w |= workText
		}
		return w
	}

	var w fileWork
	if o.Caps.Has(CapJava) {
		w |= workJava
	}
	switch {
	case o.Caps.Has(CapSpring):
		w |= workSpring
	case o.Caps.Has(CapSpringMain) && !isTestPath(relPath):
		w |= workSpring
	}
	if o.Caps.Has(CapBindings) {
		w |= workBindings
	}
	return w
}
