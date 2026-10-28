package cli

import (
	"bytes"
	"strings"
	"testing"

	"example.com/cx/internal/ir"
)

// --- trail cap marker (A5) ---------------------------------------------------
//
// The trail cap is the one place cxj knowingly shows less than it knows. The
// marker is what separates "these are all the callers" from "here is a window
// of a hundred and six".

func trailReply() ir.TrailReply {
	hits := make([]ir.TrailHit, 5)
	for i := range hits {
		hits[i] = ir.TrailHit{Path: "src/main/java/a/Client.java", Line: 10 + i, Symbol: "use", Certainty: "candidate"}
	}
	return ir.TrailReply{
		Symbol: "com.x.Target.go", Kind: "method", Path: "src/main/java/a/Target.java", Line: 3, EndLine: 5,
		Totals:  ir.TrailTotals{Callers: 106, Tests: 2},
		Callers: hits,
		HTTP:    []ir.TrailHit{}, Injects: []ir.TrailHit{}, Callees: []ir.TrailHit{},
		Supers: []ir.TrailHit{}, Subtypes: []ir.TrailHit{},
		Tests: []ir.TrailHit{
			{Path: "src/test/java/a/TargetTest.java", Line: 8, Symbol: "com.x.TargetTest"},
			{Path: "src/test/java/a/TargetMoreTest.java", Line: 9, Symbol: "com.x.TargetMoreTest"},
		},
	}
}

func TestPrinterTrail_truncationMarker(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	if p.Trail(trailReply(), Mode{}) != 0 {
		t.Fatal("exit")
	}
	got := buf.String()
	if !strings.Contains(got, "callers … (+101 more)") {
		t.Fatalf("missing capped-section marker:\n%s", got)
	}
	// A full section gets no marker — the marker means truncation, not company.
	if strings.Contains(got, "tests …") {
		t.Fatalf("uncapped section got a marker:\n%s", got)
	}
	// The label rides along because sections share one stream.
	var marker string
	for _, l := range strings.Split(got, "\n") {
		if strings.Contains(l, "(+101 more)") {
			marker = l
		}
	}
	if !strings.HasPrefix(marker, "callers ") {
		t.Fatalf("marker lost its label: %q", marker)
	}
}

func TestPrinterTrail_verboseShowsCertainty(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Out: &buf}
	if p.Trail(trailReply(), Mode{Verbose: true}) != 0 {
		t.Fatal("exit")
	}
	if got := buf.String(); !strings.Contains(got, "[candidate]") {
		t.Fatalf("--verbose must show certainty:\n%s", got)
	}
	var plain bytes.Buffer
	q := &Printer{Out: &plain}
	q.Trail(trailReply(), Mode{})
	if strings.Contains(plain.String(), "[candidate]") {
		t.Fatal("default text must stay as compact as it was")
	}
}
