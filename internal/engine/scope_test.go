package engine

import (
	"testing"

	"github.com/umuterturk/cxj/internal/scan"
)

func TestOpenScopeMainExcludesTestSymbols(t *testing.T) {
	files := map[string]string{
		"src/main/java/com/example/App.java":     "package com.example;\npublic class App { public void run() {} }\n",
		"src/test/java/com/example/AppTest.java": "package com.example;\npublic class AppTest { public void check() {} }\n",
	}
	root := writeTree(t, files)

	// CapsAll: this test is about scope, not about what a command reads.
	all, err := Open(root, Options{Caps: CapsAll})
	if err != nil {
		t.Fatal(err)
	}
	main, err := Open(root, Options{Caps: CapsAll, Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := all.syms.Lookup("AppTest"); !ok {
		t.Fatal("ScopeAll must include test symbol AppTest")
	}
	if _, ok := main.syms.Lookup("AppTest"); ok {
		t.Fatal("ScopeMain must exclude test symbol AppTest")
	}
	if _, ok := main.syms.Lookup("App"); !ok {
		t.Fatal("ScopeMain must still include main symbol App")
	}
	if !main.ScopeFilteredTests() {
		t.Fatal("ScopeFilteredTests must be true when ScopeMain dropped files")
	}
	if all.ScopeFilteredTests() {
		t.Fatal("ScopeFilteredTests must be false for ScopeAll")
	}
}

// TestBriefKeepsTestClasses verifies the fast brief path (CapsBrief, ScopeAll) still
// sees test facts, so the TestClasses count is not silently zeroed by P1.
func TestBriefKeepsTestClasses(t *testing.T) {
	files := map[string]string{
		"src/main/java/com/example/App.java":     "package com.example;\npublic class App {}\n",
		"src/test/java/com/example/AppTest.java": "package com.example;\npublic class AppTest {}\n",
	}
	brief := openBrief(t, files)
	if brief.TestClasses != 1 {
		t.Fatalf("Brief().TestClasses=%d want 1 (brief must not be scoped)", brief.TestClasses)
	}
	if brief.MainFiles != 1 {
		t.Fatalf("Brief().MainFiles=%d want 1", brief.MainFiles)
	}
}
