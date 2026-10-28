package engine

import (
	"testing"

	"example.com/cx/internal/scan"
)

// TestTrailCountsWhatItTruncates is the A5 contract: a capped section must say
// how much it cut. Before this, five callers and five-hundred callers printed
// identically, and the reader had no way to know they were looking at a
// window. The counts are per section, and the cap itself must not move.
func TestTrailCountsWhatItTruncates(t *testing.T) {
	calls := ""
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		calls += "    public String " + n + "() { return target.go(); }\n"
	}
	files := map[string]string{
		"src/main/java/com/x/Target.java": `package com.x;

public class Target {
    public String go() { return "x"; }
}
`,
		"src/main/java/com/x/Client.java": `package com.x;

public class Client {
    private final Target target = new Target();
` + calls + `}
`,
	}
	eng, err := Open(writeTree(t, files), Options{Caps: CapsFor("trail"), Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}
	rep := eng.Trail("com.x.Target.go")
	if len(rep.Callers) != trailCap {
		t.Fatalf("callers rendered = %d, want the cap (%d)", len(rep.Callers), trailCap)
	}
	if rep.Totals.Callers != 7 {
		t.Errorf("totals.callers = %d, want 7 seen", rep.Totals.Callers)
	}
	// Uncapped sections report their real size, and the marker math
	// (total - rendered) comes out right on both sides of the cap.
	if rep.Totals.HTTP != 0 || len(rep.HTTP) != 0 {
		t.Errorf("http = %d/%d, want 0/0", len(rep.HTTP), rep.Totals.HTTP)
	}
	if rep.Totals.Callees != len(rep.Callees) {
		t.Errorf("callees total %d != rendered %d under the cap", rep.Totals.Callees, len(rep.Callees))
	}
}
