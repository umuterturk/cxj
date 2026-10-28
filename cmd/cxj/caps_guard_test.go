package main

import (
	"os"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/engine"
)

// The capability table is a per-command decision about what work to skip, which means
// it can go stale in the one direction that produces wrong answers: someone adds a
// command, wires it into the switch, and never tells the table what it reads. The
// fallback for an unknown command is CapsAll, which is deliberately correct-but-slow, so
// nothing breaks and the omission is invisible.
//
// This test closes that loop where the command list actually lives. main.go is scanned
// rather than a second list being maintained, because a second list drifts the same way
// the two private copies of the match-tier table did (FIX_PLAN A-RC10).
func TestEveryCommandDeclaresItsCapabilities(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	commands := switchCommands(t, string(src))

	// Commands that never open a workspace snapshot.
	local := map[string]bool{
		"install": true, "version": true, "update": true,
		"--version": true, "-version": true,
		"help": true, "-h": true, "--help": true,
	}
	for _, cmd := range commands {
		if local[cmd] {
			continue
		}
		if !engine.KnowsCommand(cmd) {
			t.Errorf("command %q opens a workspace but is missing from engine's capability table;"+
				" it will fall back to building everything (slow) and no test will notice", cmd)
		}
	}
}

// TestCapsForIsNotBlanketAll: a table that answers CapsAll for every command satisfies
// the test above while delivering none of P2.
func TestCapsForIsNotBlanketAll(t *testing.T) {
	var trimmed int
	for _, cmd := range engine.Commands() {
		if engine.CapsFor(cmd) != engine.CapsAll {
			trimmed++
		}
	}
	if trimmed < 10 {
		t.Errorf("only %d commands declare a trimmed capability set; the table is not doing its job", trimmed)
	}
}

func switchCommands(t *testing.T, src string) []string {
	t.Helper()
	// Bounded to main() so the scan cannot wander into the switches of other
	// functions and start reporting flags and scope names as commands.
	start := strings.Index(src, "func main() {")
	if start < 0 {
		t.Fatal("could not find main() in main.go; this guard needs to follow the refactor, not be deleted")
	}
	body := src[start:]
	end := strings.Index(body, "\n}\n")
	if end < 0 {
		t.Fatal("could not find the end of main()")
	}
	body = body[:end]
	if i := strings.Index(body, "switch os.Args[1] {"); i < 0 {
		t.Fatal("main() no longer switches on os.Args[1]; the command list moved and this" +
			" guard needs to follow it")
	}
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "case ") {
			out = append(out, quoted(trimmed)...)
		}
	}
	// A guard that stops matching quietly passes forever, so pin the size of the set.
	if len(out) < 20 {
		t.Fatalf("extracted only %d commands (%v): the scan is not seeing the whole switch", len(out), out)
	}
	return out
}

func quoted(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '"')
		if i < 0 {
			return out
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '"')
		if j < 0 {
			return out
		}
		out = append(out, s[:j])
		s = s[j+1:]
	}
}
