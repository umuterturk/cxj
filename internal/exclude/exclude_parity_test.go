package exclude

import (
	"path"
	"strings"
	"testing"
)

// Differential parity for the Match rewrite (FIX_PLAN P3): the legacy
// implementation is pasted here as it was, and the new one must agree with it
// on every path in a large generated corpus under several pattern sets. The
// rewrite collapsed the bare-pattern loop into a set lookup and moved the
// prefix loop inside the glob patterns only; both claims are exactly the kind
// that "look equivalent" and are not, so they are pinned against the old code
// rather than argued.

// --- legacy implementation, verbatim from before the P3 rewrite ---------------

type legacyMatcher struct {
	patterns []pattern
}

func newLegacyMatcher(cxIgnoreLines []string, flagPatterns []string) *legacyMatcher {
	m := &legacyMatcher{}
	add := func(raw string) {
		raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		raw = strings.TrimSuffix(raw, "/")
		if raw == "" {
			return
		}
		m.patterns = append(m.patterns, pattern{raw: raw, segs: strings.Split(raw, "/")})
	}
	for _, d := range DefaultDirs {
		add(d)
		add(d + "/**")
		add("**/" + d)
		add("**/" + d + "/**")
	}
	for _, line := range cxIgnoreLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		add(line)
	}
	for _, p := range flagPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		add(p)
	}
	return m
}

func (m *legacyMatcher) Match(rel string) bool {
	rel = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(rel, "\\", "/")), "/")
	if rel == "." || rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for i := 1; i <= len(parts); i++ {
		candidate := strings.Join(parts[:i], "/")
		for _, p := range m.patterns {
			if legacyMatchPattern(p, candidate) {
				return true
			}
		}
	}
	return false
}

func legacyMatchPattern(p pattern, rel string) bool {
	if !strings.Contains(p.raw, "/") && !strings.ContainsAny(p.raw, "*?[") {
		for _, seg := range strings.Split(rel, "/") {
			if seg == p.raw {
				return true
			}
		}
		return false
	}
	return matchSegs(p.segs, strings.Split(rel, "/"))
}

// --- corpus --------------------------------------------------------------------

func parityPaths() []string {
	words := []string{"src", "main", "java", "com", "acme", "pay", "build", "target",
		".git", "node_modules", "out", ".gradle", "docs", "generated", "Foo.java",
		"a", "b", "x.tmp", "README.md", "..", "weird?name", "star*dir", "[ab]c"}
	var paths []string
	// Fixed edge cases first.
	paths = append(paths,
		"", ".", "..", "a", "/a", "//a//b", "a/./b", "a/../b",
		"build", "build/Foo.java", "x/build", "x/build/y/z", ".git/config",
		"a/b/c/d/e/f/g/h/i/j/Foo.java", "src\\main\\java\\Foo.java",
		"docs/x.tmp", "generated/Foo.java", "docs/guide/README.md", "*.java",
	)
	// Deterministic cartesian product over segment alphabets, depth 1..3.
	for _, a := range words {
		paths = append(paths, a)
		for _, b := range words {
			paths = append(paths, a+"/"+b)
			for _, c := range []string{"Foo.java", "out", "build", "x.tmp", "d"} {
				paths = append(paths, a+"/"+b+"/"+c)
			}
		}
	}
	return paths
}

func TestMatcherParityAfterRewrite(t *testing.T) {
	sets := []struct {
		name  string
		cx    []string
		flags []string
	}{
		{"defaults only", nil, nil},
		{"bare custom", []string{"generated", "# comment", ""}, nil},
		{"glob customs", []string{"src/docs/**", "**/generated"}, []string{"*.tmp", "docs/*.md"}},
		{"hard globs", nil, []string{"a/?x", "**/b/*/c", "x/y/**", "[ab]c/**"}},
	}
	paths := parityPaths()
	for _, s := range sets {
		nw := New(s.cx, s.flags)
		old := newLegacyMatcher(s.cx, s.flags)
		for _, p := range paths {
			if got, want := nw.Match(p), old.Match(p); got != want {
				t.Fatalf("%s: Match(%q)=%v, legacy=%v", s.name, p, got, want)
			}
		}
	}
}
