package java

import (
	"testing"

	"github.com/umuterturk/cxj/internal/ir"
)

// Receiver typing (FIX_PLAN A3): every call edge carries the declared simple
// type of its receiver when the file itself declares one, and stays empty
// when it genuinely does not. The empty cases are as load-bearing as the
// typed ones — a guessed type is the bug A3 removes.

func TestReceiverTyping(t *testing.T) {
	src := []byte(`package com.x;

public class Client extends Base {
    private final PayService payService;          // field, declared before use
    Helper helper;

    public Client(PayService payService) {
        this.payService = payService;
    }

    public String go(String body) {
        PayResult local = payService.pay(body);   // field
        String s = local.text();                  // local
        this.helper.bang();                       // explicit this.field
        bare();                                   // implicit this
        super.top();                              // super
        new Widget().spin();                      // construction
        for (Tag tag : tags) {                    // enhanced-for variable
            tag.ding();
        }
        getSvc().nope();                          // chained: unknown
        Util.staticCall();                        // capitalized: left for the index
        String up = body.toUpperCase();           // parameter
        lateField.read();                         // field declared AFTER the method
        return lambda(x -> x.gone());             // lambda parameter: unknown
    }
    private String lateField;                     // field declared after the method
}
`)
	f, err := NewExtractor().Extract("a/Client.java", src)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{} // edge To -> RecvType
	for _, e := range f.Edges {
		if e.Kind == "calls" {
			got[e.To] = e.RecvType
		}
	}
	want := map[string]string{
		"payService.pay":    "PayService", // field
		"local.text":        "PayResult",  // local
		"this.helper.bang":  "Helper",     // this.field — declared type of the field
		"bare":              "Client",     // implicit this (supers covered by subtype rule)
		"super.top":         "Client",     // same reading: candidate tier, subtype rule
		"new Widget().spin": "Widget",     // construction
		"tag.ding":          "Tag",        // enhanced-for
		"getSvc().nope":     "",           // chained: genuinely unknown
		"Util.staticCall":   "",           // extractor defers; index completes
		"x.gone":            "",           // lambda parameter: unknown
		"body.toUpperCase":  "String",     // parameter
		"lateField.read":    "String",     // field declared after the method is still seen
	}
	for to, wt := range want {
		if gt, ok := got[to]; !ok {
			t.Errorf("no call edge for %q (edges: %v)", to, got)
		} else if gt != wt {
			t.Errorf("RecvType(%q) = %q, want %q", to, gt, wt)
		}
	}
}

// The constructor parameter and the field share the name payService: inside
// go() the name resolves through the field scope (params are method-scoped,
// the map above pins the go() edge), and a capitalized first letter is kept
// out of the extractor's reach on purpose.
func TestReceiverTypingScopes(t *testing.T) {
	src := []byte(`package com.x;
class A {
    Widget w;
    void outer(Widget w) {
        new B().inner();
        w.tick();
    }
}
class B {
    Widget w;
    void inner() { w.tick(); }
}
`)
	f, err := NewExtractor().Extract("a/Scope.java", src)
	if err != nil {
		t.Fatal(err)
	}
	byFrom := map[string]string{} // From -> RecvType of w.tick
	n := 0
	for _, e := range f.Edges {
		if e.Kind == "calls" && e.To == "w.tick" {
			byFrom[e.From] = e.RecvType
			n++
		}
	}
	if n != 2 {
		t.Fatalf("w.tick edges = %d, want 2: %v", n, byFrom)
	}
	for from, recvType := range byFrom {
		if recvType != "Widget" {
			t.Errorf("RecvType of w.tick from %s = %q, want Widget", from, recvType)
		}
	}
}

// Occurrence/edge certainty is untouched by typing: typed receivers are still
// candidates — the declared type narrows WHO may be called, it does not prove
// virtual dispatch.
func TestTypedEdgesStayCandidate(t *testing.T) {
	src := []byte(`package com.x;
class C {
    PayService svc;
    void go() { svc.pay(); }
}
`)
	f, err := NewExtractor().Extract("a/C.java", src)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range f.Edges {
		if e.Kind == "calls" && e.To == "svc.pay" {
			found = true
			if e.RecvType != "PayService" {
				t.Fatalf("RecvType = %q, want PayService", e.RecvType)
			}
			if e.Certainty != ir.CertaintyCandidate {
				t.Fatalf("certainty = %q, want candidate", e.Certainty)
			}
		}
	}
	if !found {
		t.Fatal("svc.pay edge missing")
	}
}
