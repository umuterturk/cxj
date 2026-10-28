package ir

import "testing"

// TestWantsTypeName locks the gate on the kind preference. The dotted case matters
// most: `find PaymentController.pay` already names one member, so treating it as a
// type query would sink the very symbol the reader asked for.
func TestWantsTypeName(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		{"Handler", true},
		{"handler", false},
		{"", false},
		{" pay", false},
		{"PaymentController.pay", false},
		{"OH", true},
		{"_internal", false},
	}
	for _, c := range cases {
		if got := WantsTypeName(c.q); got != c.want {
			t.Errorf("WantsTypeName(%q)=%v want %v", c.q, got, c.want)
		}
	}
}

// TestRankMatchIsTotal: every kind the engine can emit has its own tier, and text
// sits last. A kind missing from the table falls to the default and silently sorts
// below grep hits, which is how substring behaved for a whole pass of this code.
func TestRankMatchIsTotal(t *testing.T) {
	all := []MatchKind{MatchExact, MatchPrefix, MatchFuzzy, MatchSubstring, MatchText}
	for i := 1; i < len(all); i++ {
		if RankMatch(all[i-1]) >= RankMatch(all[i]) {
			t.Errorf("%s should outrank %s (%d vs %d)", all[i-1], all[i], RankMatch(all[i-1]), RankMatch(all[i]))
		}
	}
	if RankMatch(MatchKind("bogus")) <= RankMatch(MatchText) {
		t.Error("unknown kinds should rank worst, not tied with text")
	}
}

// TestLexicalLessPrefersTypes is the A7 contract at the one place both search
// surfaces share, so the index and the merge cannot drift apart again.
func TestLexicalLessPrefersTypes(t *testing.T) {
	class := SearchResult{Path: "a/PayHandler.java", Symbol: "com.x.PayHandler", Kind: KindClass, Match: MatchSubstring, Score: 0.3}
	field := SearchResult{Path: "a/PayHandlerTest.java", Symbol: "com.x.PayHandlerTest.handler", Kind: KindField, Match: MatchExact, Score: 1}

	if !LexicalLess(class, field, true) {
		t.Error("capitalized query should rank the class first")
	}
	if !LexicalLess(field, class, false) {
		t.Error("lowercase query should still rank the exact field first")
	}
	// A member whose name really is capitalized (a constructor) is not penalised
	// for case; the type still leads it, but on the kind tier, not the penalty.
	ctor := SearchResult{Path: "a/PayHandler.java", Line: 21, Symbol: "com.x.PayHandler.PayHandler", Kind: KindMethod, Match: MatchSubstring, Score: 0.5}
	if !LexicalLess(class, ctor, true) {
		t.Error("the class should rank above its own constructor")
	}
	// Text hits never beat a symbol, whatever the score.
	line := SearchResult{Path: "a/demo.html", Line: 1, Symbol: "", Kind: KindText, Match: MatchText, Score: 0.9}
	if !LexicalLess(class, line, false) {
		t.Error("a raw text line should not outrank a symbol hit")
	}
}

// TestLexicalLessSourceTier is the A4 contract: the main tree ranks above the
// test tree above match kind. A test hit is a real hit — this is ranking, not
// correctness — but the page a reader sees first should be production code,
// and before this key a test class at MatchExact sat above the main class the
// query was really about.
func TestLexicalLessSourceTier(t *testing.T) {
	main := SearchResult{Path: "src/main/java/com/x/Pay.java", Symbol: "com.x.Pay", Kind: KindClass, Match: MatchSubstring, Score: 0.2}
	test := SearchResult{Path: "module/src/test/java/com/x/Pay.java", Symbol: "com.x.Pay", Kind: KindClass, Match: MatchExact, Score: 1}

	// Even MatchExact-in-test loses to MatchSubstring-in-main.
	if !LexicalLess(main, test, false) {
		t.Error("main should rank above test regardless of match kind")
	}
	if LexicalLess(test, main, false) {
		t.Error("the ordering must not flip on the reverse comparison")
	}
	// The wantsType penalty still outranks the source tier: a lowercase member
	// answering a type query is a wrong-kind answer, worse than a right-kind
	// answer from the test tree.
	testClass := SearchResult{Path: "src/test/java/com/x/Handler.java", Symbol: "com.x.Handler", Kind: KindClass, Match: MatchExact, Score: 1}
	mainField := SearchResult{Path: "src/main/java/com/x/Pay.java", Symbol: "com.x.Pay.handler", Kind: KindField, Match: MatchExact, Score: 1}
	if !LexicalLess(testClass, mainField, true) {
		t.Error("the type-before-member penalty should outrank the source tier")
	}
	// Same tree: match kind decides, unchanged.
	main2 := SearchResult{Path: "src/main/java/com/x/other/Pay.java", Symbol: "com.x.other.Pay", Kind: KindClass, Match: MatchExact, Score: 1}
	if !LexicalLess(main2, main, false) {
		t.Error("inside one tree, match kind must still decide")
	}
}

func TestIsTestPath(t *testing.T) {
	yes := []string{"src/test/java/Foo.java", "module/src/test/java/Foo.java", "src/test/resources/app.yml"}
	no := []string{"src/main/java/Foo.java", "src/testme/java/Foo.java", "a/src/test.java", "Foo.java"}
	for _, p := range yes {
		if !IsTestPath(p) {
			t.Errorf("IsTestPath(%q)=false, want true", p)
		}
	}
	for _, p := range no {
		if IsTestPath(p) {
			t.Errorf("IsTestPath(%q)=true, want false", p)
		}
	}
}
