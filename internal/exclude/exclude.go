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
type Matcher struct {
	patterns []pattern
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

// Match reports whether rel (forward slashes) should be excluded.
func (m *Matcher) Match(rel string) bool {
	rel = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(rel, "\\", "/")), "/")
	if rel == "." || rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	// Check full path and each directory prefix.
	for i := 1; i <= len(parts); i++ {
		candidate := strings.Join(parts[:i], "/")
		for _, p := range m.patterns {
			if matchPattern(p, candidate) {
				return true
			}
		}
	}
	return false
}

func (m *Matcher) add(raw string) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimSuffix(raw, "/")
	if raw == "" {
		return
	}
	m.patterns = append(m.patterns, pattern{raw: raw, segs: strings.Split(raw, "/")})
}

func matchPattern(p pattern, rel string) bool {
	// Bare segment name excludes that directory (and anything under it via prefix checks).
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
