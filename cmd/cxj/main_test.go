package main

import (
	"testing"

	"example.com/cx/internal/scan"
)

func TestResolveScope(t *testing.T) {
	cases := []struct {
		flag string
		def  scan.Scope
		want scan.Scope
		err  bool
	}{
		{"", scan.ScopeAll, scan.ScopeAll, false},
		{"", scan.ScopeMain, scan.ScopeMain, false}, // empty keeps command default
		{"auto", scan.ScopeMain, scan.ScopeMain, false},
		{"all", scan.ScopeMain, scan.ScopeAll, false},
		{"MAIN", scan.ScopeAll, scan.ScopeMain, false},
		{" test ", scan.ScopeAll, scan.ScopeTest, false},
		{"nope", scan.ScopeAll, scan.ScopeAll, true},
	}
	for _, c := range cases {
		got, err := resolveScope(c.flag, c.def)
		if c.err {
			if err == nil {
				t.Errorf("resolveScope(%q): expected error", c.flag)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveScope(%q): unexpected error %v", c.flag, err)
		}
		if got != c.want {
			t.Errorf("resolveScope(%q, def=%d)=%d want %d", c.flag, c.def, got, c.want)
		}
	}
}
