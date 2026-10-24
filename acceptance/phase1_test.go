package acceptance

import (
	"strings"
	"testing"
)

func TestP1FindsJavaClassAndMethodWithRealLocation(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/main/java/com/acme/InvoiceLedger.java", `package com.acme;
public class InvoiceLedger {
    public void reconcileInvoice() {}
}
`)
	e := open(t, root)
	methods := e.search(t, "InvoiceLedger.reconcileInvoice")
	if !containsSymbol(methods, "reconcileInvoice") {
		t.Fatalf("method missing: %#v", methods)
	}
	requirePath(t, methods, "InvoiceLedger.java")
	classes := e.search(t, "InvoiceLedger")
	if !containsSymbol(classes, "InvoiceLedger") {
		t.Fatalf("class missing: %#v", classes)
	}
}

func TestP1DoesNotConfuseOverloadedMethods(t *testing.T) {
	root := t.TempDir()
	put(t, root, "TwoSignatures.java", `class TwoSignatures {
    void schedule(int count) {}
    void schedule(String label) {}
}
`)
	e := open(t, root)
	r := e.search(t, "TwoSignatures.schedule")
	lines := map[int]bool{}
	for _, m := range r {
		if strings.Contains(m.Symbol, "schedule") {
			lines[m.Line] = true
		}
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 overload locations, got %#v", r)
	}
}

func TestP1ReferenceResolutionMustNotFabricateExactTarget(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Actions.java", `interface Action { void run(); }
class First implements Action { public void run() {} }
class Second implements Action { public void run() {} }
class Caller { void call(Action action) { action.run(); } }
`)
	e := open(t, root)
	refs := e.references(t, "Action.run")
	for _, r := range refs.Results {
		if r.Certainty == "exact" && (strings.Contains(r.Target, "First.run") || strings.Contains(r.Target, "Second.run")) {
			t.Fatalf("interface dispatch was falsely resolved to a concrete implementation: %+v", r)
		}
	}
}
