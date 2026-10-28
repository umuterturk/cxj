package cli

import (
	"bytes"
	"strings"
	"testing"

	"example.com/cx/internal/ir"
)

// --- ambiguity surface (A2) --------------------------------------------------
//
// The engine reports when a query matched several owners. If the printer stays
// silent in text mode, the one output mode an agent is most likely to read hides
// the fact that the results describe one arbitrary owner of many.

func TestPrinterReferences_textShowsAmbiguity(t *testing.T) {
	reply := ir.RefReply{
		Results:   []ir.RefResult{{Path: "a/A.java", Line: 3, Kind: ir.RefKindUse, Target: "save"}},
		Resolved:  "com.example.pay.PayHandler.handle",
		Ambiguous: []string{"com.example.pay.PayHandler.handle", "com.example.pay.RefundHandler.handle"},
	}
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	if p.References(reply, Mode{}) != 0 {
		t.Fatal("exit")
	}
	got := buf.String()
	if !strings.Contains(got, "ambiguous 2:") {
		t.Fatalf("ambiguity not shown in text mode:\n%s", got)
	}
	if !strings.Contains(got, "RefundHandler.handle") {
		t.Fatalf("candidate not listed:\n%s", got)
	}
}

func TestPrinterReferences_compactJSONShowsAmbiguity(t *testing.T) {
	reply := ir.RefReply{
		Results:   []ir.RefResult{{Path: "a/A.java", Line: 3, Kind: ir.RefKindUse, Target: "save"}},
		Ambiguous: []string{"com.example.pay.PayHandler.handle", "com.example.pay.RefundHandler.handle"},
	}
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	if p.References(reply, Mode{Compact: true}) != 0 {
		t.Fatal("exit")
	}
	if !strings.Contains(buf.String(), "ambiguous") {
		t.Fatalf("ambiguity missing from compact json: %s", buf.String())
	}
}

func TestPrinterReferences_quietWhenUnambiguous(t *testing.T) {
	reply := ir.RefReply{
		Results: []ir.RefResult{{Path: "a/A.java", Line: 3, Kind: ir.RefKindUse, Target: "save"}},
	}
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	p.References(reply, Mode{})
	if strings.Contains(buf.String(), "ambiguous") {
		t.Fatalf("noise on a precise query:\n%s", buf.String())
	}
}

func TestPrinterReferences_ambiguityIsCapped(t *testing.T) {
	var alts []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		alts = append(alts, "com.example.pay."+n+".handle")
	}
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	p.References(ir.RefReply{Results: []ir.RefResult{{Path: "a/A.java", Line: 1}}, Ambiguous: alts}, Mode{})
	got := buf.String()
	if !strings.Contains(got, "ambiguous 7:") {
		t.Fatalf("count should be the true total:\n%s", got)
	}
	if strings.Contains(got, "g.handle") {
		t.Fatalf("candidate list not capped, defeats the token budget:\n%s", got)
	}
	if !strings.Contains(got, "+2") {
		t.Fatalf("hidden candidates not counted:\n%s", got)
	}
}
