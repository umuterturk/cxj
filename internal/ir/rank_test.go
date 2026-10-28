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
