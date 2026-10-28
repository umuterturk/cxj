// Package exclude provides one canonical path matcher for scan, index, and watch.
package exclude

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DefaultDirs are always excluded at every directory level.
var DefaultDirs = []string{".git", "node_modules", ".idea", "target", "build", "out", ".gradle"}

// Matcher decides whether a forward-slash relative path is excluded.
//
// Match runs on every path the scanner walks — thousands per second — and the
// first implementation showed it: it re-Split the candidate for every bare
// pattern and re-parsed every glob through path.Match, for every prefix of
// every path, which cost more than reading the files. The rewrite keeps the
// old code as the definition of correct (see legacyMatch in exclude_parity_test)
// and preserves its exact semantics: a path matches when any prefix of it
// matches any pattern, bare patterns match any segment, and a trailing **
// matches the directory itself.
//
// What makes it fast is what the defaults actually are. All four spellings of
// every default directory ("build", "build/**", "**/build", "**/build/**")
// match exactly the paths that have that segment somewhere — so the whole
// 28-pattern default set collapses to one set lookup per segment. Only
// user-supplied globs (from .cxignore or --exclude) still run the prefix
// loop, and typically there are none.
type Matcher struct {
	bare  map[string]bool // bare patterns: match any path with that segment
	globs []pattern       // patterns with separators or wildcards, in add order
}

type pattern struct {
	raw  string
	segs []string
}

// New builds a matcher from default dirs, .cxignore lines, and --exclude flags.
func New(cxIgnoreLines []string, flagPatterns []string) *Matcher {
	m := &Matcher{}
	for _, d := range DefaultDirs {
		m.add(d)
		m.add(d + "/**")
		m.add("**/" + d)
		m.add("**/" + d + "/**")
	}
	for _, line := range cxIgnoreLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.add(line)
	}
	for _, p := range flagPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m.add(p)
	}
	return m
}

// LoadCxIgnore reads root/.cxignore if present.
func LoadCxIgnore(root string) ([]string, error) {
	f, err := os.Open(filepath.Join(root, ".cxignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func (m *Matcher) add(raw string) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" {
		return
	}
	// Same bare test the legacy matcher applied per call, applied once here.
	if !strings.Contains(raw, "/") && !strings.ContainsAny(raw, "*?[") {
		if m.bare == nil {
			m.bare = map[string]bool{}
		}
		m.bare[raw] = true
		return
	}
	m.globs = append(m.globs, pattern{raw: raw, segs: strings.Split(raw, "/")})
}

// Match reports whether rel (forward slashes) should be excluded.
func (m *Matcher) Match(rel string) bool {
	rel = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(rel, "\\", "/")), "/")
	if rel == "." || rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, seg := range parts {
		if m.bare[seg] {
			return true
		}
	}
	// Prefix loop, but only the few patterns need it.
	for _, p := range m.globs {
		for i := 1; i <= len(parts); i++ {
			if matchSegs(p.segs, parts[:i]) {
				return true
			}
		}
	}
	return false
}

func matchSegs(pat, pathSegs []string) bool {
	var dfs func(pi, si int) bool
	dfs = func(pi, si int) bool {
		for {
			if pi == len(pat) {
				return si == len(pathSegs)
			}
			if pat[pi] == "**" {
				if pi == len(pat)-1 {
					return true
				}
				for k := si; k <= len(pathSegs); k++ {
					if dfs(pi+1, k) {
						return true
					}
				}
				return false
			}
			if si == len(pathSegs) {
				return false
			}
			ok, err := path.Match(pat[pi], pathSegs[si])
			if err != nil || !ok {
				return false
			}
			pi++
			si++
		}
	}
	return dfs(0, 0)
}
