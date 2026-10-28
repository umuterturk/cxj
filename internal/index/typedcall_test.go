package index

import (
	"sort"
	"strconv"
	"testing"

	"example.com/cx/internal/ir"
	"example.com/cx/internal/java"
)

// Typed call matching (FIX_PLAN A3). Before, `callers C.go` matched every
// call whose edge merely ended in ".go" — the query for one method answered
// with everyone's. Now a typed edge must agree with the queried owner or a
// direct subtype of it, and only genuinely unknown receivers keep the
// name-only fallback. The fixture is collision-shaped: two types with one
// shared member name is the only shape that can tell these laws apart.

func typedCallFixture(t *testing.T) *SymbolIndex {
	t.Helper()
	files := map[string]string{
		"a/Client.java": `package a;

interface Api {
    String go();
}
class Impl implements Api {
    public String go() { return "i"; }
}
class PayService {
    String pay() { return "p"; }
}
class Client {
    private final PayService svc;      // 13: typed field, NOT the query target
    private final Api api;             // 14
    Client(PayService svc, Api api) { this.svc = svc; this.api = api; }

    String use() {                     // 17
        svc.pay();                     // 18 typed wrong receiver: drops from go-queries
        api.go();                      // 19 typed receiver Api: Api.go only
        this.proxy();                  // 20 typed via this: Client.proxy
        api2().go();                   // 21 chained unknown: candidate everywhere
        return go();                   // 22 bare: implicit this, Client.go
    }
    String proxy() { return api.go(); } // 24 typed receiver Api: Api.go only
    String go() { return "own"; }       // 25
}
`,
		"a/Other.java": `package a;

class Other {
    private final PayService otherSvc;   // 4
    String use() { otherSvc.pay(); }     // 5 typed RIGHT receiver, other file
    String pay() { return "op"; }        // 6
}
`,
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	ex := java.NewExtractor()
	defer ex.Close()
	facts := make([]ir.FileFacts, 0, len(names))
	for _, name := range names {
		f, err := ex.Extract(name, []byte(files[name]))
		if err != nil {
			t.Fatalf("extract %s: %v", name, err)
		}
		facts = append(facts, f)
	}
	return Build(facts)
}

func useLines(t *testing.T, idx *SymbolIndex, symbol string) []string {
	t.Helper()
	var out []string
	for _, r := range idx.Callers(symbol) {
		out = append(out, r.Path+":"+strconv.Itoa(r.Line))
	}
	return out
}

func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	a := append([]string{}, got...)
	b := append([]string{}, want...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The exit law in one test: a typed call is attributed to ITS receiver's
// declared type and nobody else's methods, no matter what the member is
// called. And A3 drops wrong-receiver edges, never right-receiver ones
// across files.
func TestTypedCallersAttributedToDeclaredReceiver(t *testing.T) {
	idx := typedCallFixture(t)

	if got := useLines(t, idx, "a.PayService.pay"); !sameSet(got, "a/Client.java:18", "a/Other.java:5") {
		t.Errorf("callers(a.PayService.pay) = %v, want both typed payers", got)
	}
	// Client.go: only the unknown-receiver chain (candidate) and the bare
	// in-class call. svc.pay and every api.go are typed to other owners.
	if got := useLines(t, idx, "a.Client.go"); !sameSet(got, "a/Client.java:21", "a/Client.java:22") {
		t.Errorf("callers(a.Client.go) = %v", got)
	}
	// this.proxy() is a call on the object itself: it belongs to Client.proxy.
	if got := useLines(t, idx, "a.Client.proxy"); !sameSet(got, "a/Client.java:20") {
		t.Errorf("callers(a.Client.proxy) = %v", got)
	}
}

// An interface-typed call answers to the interface. Calls through a field
// declared as an interface are NOT attributed to a concrete implementation:
// cxj does not track runtime bindings, and claiming an interface caller for
// one chosen impl would re-create in miniature the guess A3 removes.
func TestInterfaceDispatchStaysWithTheInterface(t *testing.T) {
	idx := typedCallFixture(t)
	if got := useLines(t, idx, "a.Api.go"); !sameSet(got, "a/Client.java:19", "a/Client.java:21", "a/Client.java:24") {
		t.Errorf("callers(a.Api.go) = %v", got)
	}
	// Only the unknown-receiver chain still answers for Impl (name fallback).
	if got := useLines(t, idx, "a.Impl.go"); !sameSet(got, "a/Client.java:21") {
		t.Errorf("callers(a.Impl.go) = %v, want only the unknown-receiver candidate", got)
	}
}

// The subtype direction the law DOES accept: a call on a field declared as
// Impl (a subtype of Api) is a candidate caller of Api.go.
func TestSubtypeReceiverAnswersToSupertype(t *testing.T) {
	files := map[string]string{
		"a/Holder.java": `package a;
class Holder {
    private final Impl impl;
    String use() { return impl.go(); }
}
`,
		"a/Api.java": `package a;
interface Api { String go(); }
`,
		"a/Impl.java": `package a;
class Impl implements Api { public String go() { return "i"; } }
`,
	}
	ex := java.NewExtractor()
	defer ex.Close()
	var facts []ir.FileFacts
	for _, name := range []string{"a/Holder.java", "a/Api.java", "a/Impl.java"} {
		f, err := ex.Extract(name, []byte(files[name]))
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, f)
	}
	idx := Build(facts)
	if got := useLines(t, idx, "a.Api.go"); !sameSet(got, "a/Holder.java:4") {
		t.Errorf("subtype call not kept for the supertype query: %v", got)
	}
	// ...and equally for the impl itself, which is the declared type.
	if got := useLines(t, idx, "a.Impl.go"); !sameSet(got, "a/Holder.java:4") {
		t.Errorf("declared-type call lost: %v", got)
	}
}

// "Unknown" must never become "no": chained receivers keep the name-only
// candidate match, because a candidate is the truth about what is known.
func TestUnknownReceiverStaysCandidate(t *testing.T) {
	idx := typedCallFixture(t)
	for _, q := range []string{"a.Api.go", "a.Impl.go", "a.Client.go"} {
		if got := useLines(t, idx, q); !contains(got, "a/Client.java:21") {
			t.Errorf("unknown-receiver candidate dropped from callers(%s): %v", q, got)
		}
	}
}

// Static-style calls: the file never declares the type, so only a whole-index
// pass can know `Util.round` is a call on a known type — and it must stay
// attributed there, not to the caller's own class.
func TestStaticReceiverCompletedByIndex(t *testing.T) {
	files := map[string]string{
		"a/Caller.java": `package a;
class Caller {
    String go() { return round(); }        // 3 bare: implicit this
    String round2() { return Util.round(1); } // 4 static on a known type
}
`,
		"a/Util.java": `package a;
class Util {
    static int round(int i) { return i; }
}
`,
	}
	ex := java.NewExtractor()
	defer ex.Close()
	var facts []ir.FileFacts
	for _, name := range []string{"a/Caller.java", "a/Util.java"} {
		f, err := ex.Extract(name, []byte(files[name]))
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, f)
	}
	idx := Build(facts)
	if got := useLines(t, idx, "a.Util.round"); !sameSet(got, "a/Caller.java:4") {
		t.Errorf("callers(a.Util.round) = %v, want only the static call", got)
	}
}

// An unqualified query has no owner to compare against: every named edge
// stays in play exactly as before (the engine surfaces ambiguity itself).
func TestBareQueryKeepsLegacyBehavior(t *testing.T) {
	idx := typedCallFixture(t)
	if got := useLines(t, idx, "pay"); !sameSet(got, "a/Client.java:18", "a/Other.java:5") {
		t.Errorf("bare-name query changed shape: %v", got)
	}
}

// contains(hay, needle) lives in golden_test.go, same package.
