package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"example.com/cx/internal/engine"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/scan"
)

// Batch is the amortization answer to the cold-Open tax (FIX_PLAN P4): a stateless
// process is a stateless *per invocation* process, and an agent that wants twenty
// answers from one workspace should not pay twenty Opens. `cxj batch` reads JSONL
// queries from stdin, opens the workspace once, answers every query, prints JSONL,
// and exits. Nothing persists; the second `cxj batch` is as cold as the first.
//
// Reading all of stdin before opening is deliberate: knowing the commands first
// lets the batch pay for exactly the sub-indexes they union to — the P2 trick,
// applied per batch instead of per command.

// batchReq is one line of stdin. Cmd names the command; Arg carries its
// positional (symbol, query, path, key). The optional fields cover the flags
// those commands take. Per-item scope is rejected because the snapshot is
// open by the time requests run: scope is a batch decision, stated as a flag.
type batchReq struct {
	Cmd     string `json:"cmd"`
	Arg     string `json:"arg,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Method  string `json:"method,omitempty"`
	Path    string `json:"path,omitempty"`
	Type    string `json:"type,omitempty"`
	Key     string `json:"key,omitempty"`
	Profile string `json:"profile,omitempty"`
	Budget  int    `json:"budget,omitempty"`
}

// batchRes is one line of stdout, in request order.
type batchRes struct {
	I      int    `json:"i"`
	Cmd    string `json:"cmd,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

// batchSummary is the last line, and the proof of the whole feature: open_ms
// next to wall_ms says out loud that the Opens were amortized.
type batchSummary struct {
	Batch  bool   `json:"batch"`
	Items  int    `json:"items"`
	OK     int    `json:"ok"`
	OpenMs int64  `json:"open_ms"`
	WallMs int64  `json:"wall_ms"`
	Scope  string `json:"scope"`
	Error  string `json:"error,omitempty"`
}

// batchable maps every command a batch may run. `ask` is excluded on purpose: it
// shells out to Needle and can prompt. install/version/update never open a workspace.
var batchable = map[string]bool{
	"brief": true, "find": true, "show": true, "outline": true, "map": true,
	"trail": true, "references": true, "callers": true, "callees": true,
	"keys": true, "endpoints": true, "beans": true, "config": true,
	"context": true, "status": true,
}

// batchWantsAll lists the commands whose standalone defaults include test sources
// or text search. A batch containing any of them defaults to that breadth —
// answers must not silently differ from the standalone command's; --scope wins.
var batchWantsAll = map[string]bool{"brief": true, "find": true, "map": true}

const (
	batchMaxItems = 500
	batchMaxBytes = 4 << 20 // one line; a 4 MiB query is a mistake worth naming
)

func runBatch(args []string) int {
	if containsFlag(args, "-h") || containsFlag(args, "--help") {
		fmt.Fprint(os.Stderr, `usage: cxj batch [--root PATH] [--scope all|main|test]
  JSONL queries on stdin, one JSON object per line:
    {"cmd":"find","arg":"PaymentController"}
    {"cmd":"show","arg":"com.acme.Foo.bar","limit":40}
    {"cmd":"endpoints","method":"GET","path":"/owners"}
  One Open answers the whole batch; answers come back in request order,
  {"i":N,"cmd":...,"ok":true,"result":...} per line, then a summary line.
`)
		return 0
	}
	if containsFlag(args, "--example") || containsFlag(args, "-example") {
		return printStaticExamples("batch")
	}
	return runBatchWith(os.Stdin, os.Stdout, args)
}

// batchEntry preserves input order through the two-phase read: a line that
// never parsed still owns its position in the answer sequence.
type batchEntry struct {
	index int
	cmd   string
	req   batchReq
	err   string // parse or validation failure; req is unused then
}

func runBatchWith(in io.Reader, out io.Writer, args []string) int {
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "workspace root (default: current directory)")
	scopeFlag := fs.String("scope", "", "source scope for the whole batch (default: all if any query needs it, else main)")
	fs.String("json", "", "accepted for consistency; batch output is always JSONL")
	fs.String("compact", "", "accepted for consistency")
	fs.String("verbose", "", "accepted for consistency")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	r := strings.TrimSpace(*root)
	if r == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		r = wd
	}
	if err := scan.ValidateRoot(r); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}

	started := time.Now()
	entries := readBatchEntries(in)
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "batch: no queries on stdin")
		return 2
	}
	if len(entries) > batchMaxItems {
		fmt.Fprintf(os.Stderr, "batch: %d queries exceeds the %d limit\n", len(entries), batchMaxItems)
		return 2
	}

	caps := engine.Capabilities(0)
	scopeAll := false
	for _, e := range entries {
		if e.err != "" {
			continue
		}
		caps |= engine.CapsFor(e.cmd)
		if batchWantsAll[e.cmd] {
			scopeAll = true
		}
	}
	defaultScope := scan.ScopeMain
	if scopeAll {
		defaultScope = scan.ScopeAll
	}
	scope, err := resolveScope(*scopeFlag, defaultScope)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}

	enc := json.NewEncoder(out)
	openedAt := time.Now()
	eng, err := engine.Open(r, engine.Options{Caps: caps, Scope: scope})
	opened := time.Since(openedAt)
	if err != nil {
		enc.Encode(batchSummary{Batch: true, Error: err.Error()})
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	ok := 0
	ran := 0
	for _, e := range entries {
		if e.err != "" {
			enc.Encode(batchRes{I: e.index, Cmd: e.cmd, Error: e.err})
			continue
		}
		res := batchRes{I: e.index, Cmd: e.cmd}
		res.OK, res.Error, res.Result = runBatchReq(eng, e.req)
		if res.OK {
			ok++
		}
		ran++
		enc.Encode(res)
	}
	enc.Encode(batchSummary{
		Batch:  true,
		Items:  len(entries),
		OK:     ok,
		OpenMs: opened.Milliseconds(),
		WallMs: time.Since(started).Milliseconds(),
		Scope:  scopeName(scope),
	})
	if ok < ran || ran < len(entries) {
		return 1
	}
	return 0
}

func readBatchEntries(in io.Reader) []batchEntry {
	var out []batchEntry
	index := 0
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64<<10), batchMaxBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e := batchEntry{index: index}
		index++
		var q batchReq
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		switch {
		case len(line) > batchMaxBytes:
			e.err = "line too large"
		default:
			if err := dec.Decode(&q); err != nil {
				e.err = "bad JSON: " + err.Error()
			} else if !batchable[q.Cmd] {
				e.err = fmt.Sprintf("unknown or non-batchable command %q", q.Cmd)
				e.cmd = q.Cmd
			} else {
				e.cmd = q.Cmd
				e.req = q
			}
		}
		out = append(out, e)
	}
	return out
}

// runBatchReq dispatches one query against the open snapshot. The payload is
// byte-for-byte the standalone command's JSON reply: a batch answer and a
// single-command answer must not be two schemas.
func runBatchReq(eng *engine.Engine, q batchReq) (bool, string, any) {
	arg := strings.TrimSpace(q.Arg)
	switch q.Cmd {
	case "brief":
		return true, "", eng.Brief()
	case "find":
		if arg == "" {
			return false, "arg required", nil
		}
		limit := q.Limit
		if limit <= 0 {
			limit = ir.DefaultFindLimit
		}
		return true, "", eng.Search(arg, limit)
	case "show":
		if arg == "" {
			return false, "arg required", nil
		}
		maxLines := q.Limit
		if maxLines <= 0 {
			maxLines = ir.DefaultShowLines
		}
		return true, "", eng.Show(arg, maxLines)
	case "outline":
		if arg == "" {
			return false, "arg required", nil
		}
		return true, "", eng.Outline(arg)
	case "trail":
		if arg == "" {
			return false, "arg required", nil
		}
		return true, "", eng.Trail(arg)
	case "references":
		if arg == "" {
			return false, "arg required", nil
		}
		return true, "", eng.References(arg)
	case "callers":
		if arg == "" {
			return false, "arg required", nil
		}
		return true, "", eng.Callers(arg)
	case "callees":
		if arg == "" {
			return false, "arg required", nil
		}
		return true, "", eng.Callees(arg)
	case "map":
		return true, "", eng.Map()
	case "keys":
		return true, "", eng.Keys()
	case "status":
		return true, "", eng.Status()
	case "endpoints":
		return true, "", eng.Endpoints(q.Method, q.Path)
	case "beans":
		typ := q.Type
		if typ == "" {
			typ = arg
		}
		if strings.TrimSpace(typ) == "" {
			return false, "type required (\"type\" or \"arg\")", nil
		}
		return true, "", eng.Beans(typ)
	case "config":
		key := q.Key
		if key == "" {
			key = arg
		}
		if strings.TrimSpace(key) == "" {
			return false, "key required (\"key\" or \"arg\")", nil
		}
		return true, "", eng.Config(key, q.Profile)
	case "context":
		if arg == "" {
			return false, "arg required", nil
		}
		budget := q.Budget
		if budget <= 0 {
			budget = 500
		}
		return true, "", eng.Context(arg, budget)
	default:
		return false, "command not batchable: " + q.Cmd, nil
	}
}

func scopeName(s scan.Scope) string {
	switch s {
	case scan.ScopeMain:
		return "main"
	case scan.ScopeTest:
		return "test"
	default:
		return "all"
	}
}

func containsFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}
