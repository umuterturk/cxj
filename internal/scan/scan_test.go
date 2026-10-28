package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func mkTree(t *testing.T, rels ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScopeKeep(t *testing.T) {
	cases := []struct {
		scope Scope
		path  string
		keep  bool
	}{
		{ScopeAll, "src/main/java/A.java", true},
		{ScopeAll, "src/test/java/ATest.java", true},
		{ScopeMain, "src/main/java/A.java", true},
		{ScopeMain, "src/main/resources/application.yml", true},
		{ScopeMain, "src/test/java/ATest.java", false},
		{ScopeMain, "a/src/test/java/B.java", false},
		{ScopeTest, "src/test/java/ATest.java", true},
		{ScopeTest, "src/main/java/A.java", false},
	}
	for _, c := range cases {
		if got := c.scope.keep(c.path); got != c.keep {
			t.Errorf("Scope(%d).keep(%q)=%v want %v", c.scope, c.path, got, c.keep)
		}
	}
}

func TestScanScopeCounts(t *testing.T) {
	root := mkTree(t,
		"src/main/java/A.java",
		"src/main/java/B.java",
		"src/test/java/ATest.java",
	)
	cases := []struct {
		scope   Scope
		java    int
		skipped int
	}{
		{ScopeAll, 3, 0},
		{ScopeMain, 2, 1},
		{ScopeTest, 1, 2},
	}
	for _, c := range cases {
		res, err := Scan(root, Options{Scope: c.scope})
		if err != nil {
			t.Fatal(err)
		}
		if res.Stats.IndexedJavaFiles != c.java {
			t.Errorf("scope %d: IndexedJavaFiles=%d want %d", c.scope, res.Stats.IndexedJavaFiles, c.java)
		}
		if res.Stats.SkippedScope != c.skipped {
			t.Errorf("scope %d: SkippedScope=%d want %d", c.scope, res.Stats.SkippedScope, c.skipped)
		}
	}
}

// TestScanScopeDefaultIsAll guards the zero-value contract: an Options{} with no
// Scope set must keep scanning everything, so existing call sites are unchanged.
func TestScanScopeDefaultIsAll(t *testing.T) {
	root := mkTree(t, "src/main/java/A.java", "src/test/java/ATest.java")
	res, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.IndexedJavaFiles != 2 || res.Stats.SkippedScope != 0 {
		t.Fatalf("default scope: java=%d skipped=%d, want 2/0", res.Stats.IndexedJavaFiles, res.Stats.SkippedScope)
	}
}
