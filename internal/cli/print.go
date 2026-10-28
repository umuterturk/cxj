// Package cli formats one-shot engine results for stdout.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"example.com/cx/internal/ir"
)

// Mode selects compact vs full output.
type Mode struct {
	JSON    bool
	Compact bool
	Verbose bool
}

// compactJSON is true when the caller asked for a stripped JSON payload.
func (m Mode) compactJSON() bool { return m.JSON && m.Compact && !m.Verbose }

// verboseText is true when the caller asked for extra text columns.
func (m Mode) verboseText() bool { return !m.JSON && m.Verbose && !m.Compact }

// Printer writes CLI output.
type Printer struct {
	Out io.Writer
	Err io.Writer
}

// NewPrinter writes to stdout/stderr.
func NewPrinter() *Printer {
	return &Printer{Out: os.Stdout, Err: os.Stderr}
}

// JSON writes v as UTF-8 JSON plus a trailing newline.
func (p *Printer) JSON(v any) int {
	enc := json.NewEncoder(p.Out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(p.Err, err)
		return 1
	}
	return 0
}

// Status writes status JSON or a one-line summary.
func (p *Printer) Status(st ir.Status, asJSON bool) int {
	if asJSON {
		return p.JSON(st)
	}
	fmt.Fprintf(p.Out, "status=%s indexed_files=%d indexed_java_files=%d generation=%d\n",
		st.Status, st.IndexedFiles, st.IndexedJavaFiles, st.Generation)
	return 0
}

type compactHit struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol,omitempty"`
}

type compactRef struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Kind   string `json:"kind,omitempty"`
	Target string `json:"target,omitempty"`
}

// Find writes search results.
func (p *Printer) Find(reply ir.SearchReply, m Mode) int {
	if m.compactJSON() {
		hits := make([]compactHit, 0, len(reply.Results))
		for _, r := range reply.Results {
			hits = append(hits, compactHit{Path: r.Path, Line: r.Line, Symbol: r.Symbol})
		}
		return p.JSON(map[string]any{"results": hits})
	}
	if m.JSON {
		return p.JSON(reply)
	}
	for _, r := range reply.Results {
		if m.verboseText() {
			fmt.Fprintf(p.Out, "%s:%d\t%s\t%s\t%s\n", r.Path, r.Line, r.Symbol, r.Kind, r.Match)
			continue
		}
		fmt.Fprintf(p.Out, "%s:%d %s\n", r.Path, r.Line, r.Symbol)
	}
	return 0
}

// References writes reference results.
func (p *Printer) References(reply ir.RefReply, m Mode) int {
	if m.compactJSON() {
		hits := make([]compactRef, 0, len(reply.Results))
		for _, r := range reply.Results {
			hits = append(hits, compactRef{Path: r.Path, Line: r.Line, Kind: string(r.Kind), Target: r.Target})
		}
		out := map[string]any{"results": hits}
		if reply.Resolved != "" {
			out["resolved"] = reply.Resolved
		}
		if len(reply.Ambiguous) > 0 {
			shown, total := capAmbiguous(reply.Ambiguous)
			out["ambiguous"] = shown
			out["ambiguous_total"] = total
		}
		return p.JSON(out)
	}
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Resolved != "" {
		fmt.Fprintf(p.Out, "resolved %s\n", reply.Resolved)
	}
	p.ambiguous(reply.Ambiguous)
	for _, r := range reply.Results {
		kind := string(r.Kind)
		if kind == "" {
			kind = "ref"
		}
		if m.verboseText() {
			fmt.Fprintf(p.Out, "%s:%d\t%s\t%s\t%s\n", r.Path, r.Line, kind, r.Target, r.Certainty)
			continue
		}
		fmt.Fprintf(p.Out, "%s:%d %s %s\n", r.Path, r.Line, kind, r.Target)
	}
	return 0
}

// ambiguous flags a query that matched more than one owner. The results below it
// describe only the first candidate, so a reader who is not told may act on one
// class's data believing it covers all of them.
func (p *Printer) ambiguous(alts []string) {
	if len(alts) == 0 {
		return
	}
	shown, total := capAmbiguous(alts)
	extra := ""
	if total > len(shown) {
		extra = fmt.Sprintf(" +%d", total-len(shown))
	}
	fmt.Fprintf(p.Out, "ambiguous %d: qualify the query (%s%s)\n", total, strings.Join(shown, " "), extra)
}

// capAmbiguous keeps the hint inside a sane token budget: a bare method name like
// handle can be owned by dozens of classes in a real repo, and the whole point of
// the flag is that the reader should narrow the query, not read every candidate.
// The count is always the true total so the truncation is visible.
func capAmbiguous(alts []string) ([]string, int) {
	const maxShown = 5
	if len(alts) <= maxShown {
		return alts, len(alts)
	}
	return alts[:maxShown], len(alts)
}

// Show writes a definition body.
func (p *Printer) Show(reply ir.ShowReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Path == "" {
		fmt.Fprintln(p.Out, "not found")
		return 0
	}
	if reply.Resolved != "" {
		fmt.Fprintf(p.Out, "resolved %s\n", reply.Resolved)
	}
	fmt.Fprintf(p.Out, "%s:%d %s %s\n", reply.Path, reply.Line, reply.Kind, reply.Symbol)
	if reply.Body != "" {
		fmt.Fprintln(p.Out, reply.Body)
	}
	if reply.Truncated {
		fmt.Fprintln(p.Out, "... [truncated]")
	}
	return 0
}

// Outline writes a structural summary.
func (p *Printer) Outline(reply ir.OutlineReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Path == "" {
		fmt.Fprintln(p.Out, "not found")
		return 0
	}
	pkg := reply.Package
	if pkg == "" {
		pkg = "(default)"
	}
	fmt.Fprintf(p.Out, "pkg %s  %s\n", pkg, reply.Path)
	for _, t := range reply.Types {
		end := t.EndLine
		if end < t.Line {
			end = t.Line
		}
		fmt.Fprintf(p.Out, "  %s %s %d-%d\n", t.Kind, t.Name, t.Line, end)
		for _, mem := range t.Members {
			sig := mem.Signature
			if sig != "" && !strings.Contains(sig, "(") {
				sig = "(" + sig + ")"
			}
			fmt.Fprintf(p.Out, "    %s %s%s %d\n", mem.Kind, mem.Name, sig, mem.Line)
		}
	}
	return 0
}

// Map writes the package index.
func (p *Printer) Map(reply ir.MapReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	for _, pkg := range reply.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default)"
		}
		fmt.Fprintln(p.Out, name)
		for _, t := range pkg.Types {
			fmt.Fprintf(p.Out, "  %s  %s:%d\n", t.Name, t.Path, t.Line)
		}
	}
	return 0
}

// Brief writes the orientation card.
func (p *Printer) Brief(reply ir.BriefReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	fmt.Fprintln(p.Out, ir.FormatBrief(reply))
	return 0
}

// Trail writes a one-hop neighborhood.
func (p *Printer) Trail(reply ir.TrailReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Path == "" {
		fmt.Fprintln(p.Out, "not found")
		return 0
	}
	fmt.Fprintf(p.Out, "%s %s %s:%d-%d\n", reply.Kind, reply.Symbol, reply.Path, reply.Line, reply.EndLine)
	writeHits := func(label string, hits []ir.TrailHit) {
		for _, h := range hits {
			sym := h.Symbol
			if h.Detail != "" {
				sym = h.Detail + " " + sym
			}
			fmt.Fprintf(p.Out, "%s %s:%d %s\n", label, h.Path, h.Line, strings.TrimSpace(sym))
		}
	}
	writeHits("http", reply.HTTP)
	writeHits("injects", reply.Injects)
	writeHits("callers", reply.Callers)
	writeHits("callees", reply.Callees)
	writeHits("supers", reply.Supers)
	writeHits("subtypes", reply.Subtypes)
	writeHits("tests", reply.Tests)
	return 0
}

// Keys writes config key names.
func (p *Printer) Keys(reply ir.KeysReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if len(reply.Profiles) > 0 {
		fmt.Fprintf(p.Out, "profiles %s\n", strings.Join(reply.Profiles, ","))
	}
	for _, k := range reply.Keys {
		line := k.Key + " " + k.Resolution
		if k.Secret {
			line += " [REDACTED]"
		}
		if len(k.Profiles) > 0 {
			line += " " + strings.Join(k.Profiles, ",")
		}
		fmt.Fprintln(p.Out, line)
	}
	return 0
}

// Deps writes the dependency table: coordinate, version, how the version was
// learned, and where it is declared. The "+N more" line matters as much here as
// it does in trail: without it, a cap and an end of data look identical.
func (p *Printer) Deps(reply ir.DepsReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Files == 0 {
		fmt.Fprintln(p.Out, "no build files (pom.xml, build.gradle) found")
		return 0
	}
	head := make([]string, 0, 2)
	if reply.Java != "" {
		head = append(head, "java "+reply.Java)
	}
	if reply.Boot != "" {
		head = append(head, "boot "+reply.Boot)
	}
	if len(head) > 0 {
		fmt.Fprintf(p.Out, "%s  (%d build files)\n", strings.Join(head, "  "), reply.Files)
	}
	if reply.Count == 0 {
		fmt.Fprintln(p.Out, "no dependencies match")
		return 0
	}
	w := 0
	for _, d := range reply.Deps {
		if n := len(d.Group + ":" + d.Artifact); n > w {
			w = n
		}
	}
	for _, d := range reply.Deps {
		ver := d.Version
		if ver == "" {
			ver = "?"
		}
		line := fmt.Sprintf("%-*s  %-14s %-9s %s", w+1, d.Group+":"+d.Artifact, ver, d.Tier, d.Source)
		if d.Note != "" {
			line += "  # " + d.Note
		}
		fmt.Fprintln(p.Out, line)
	}
	if more := reply.Count - reply.Shown; more > 0 {
		fmt.Fprintf(p.Out, "+%d more (--limit %d, or a filter)\n", more, reply.Count)
	}
	return 0
}

// Api writes the dependency-surface answer: the resolved coordinate with its
// provenance, the public types and signatures read from the artifact, and the
// workspace's own call sites. The surface label is load-bearing — a signature
// from a sources jar and one from a class file are different strengths of
// evidence, and the reader should know which row is which.
func (p *Printer) Api(reply ir.ApiReply, m Mode) int {
	if m.JSON {
		return p.JSON(reply)
	}
	if len(reply.Ambiguous) > 0 {
		if reply.Note != "" {
			fmt.Fprintln(p.Out, reply.Note)
		}
		for _, c := range reply.Ambiguous {
			fmt.Fprintln(p.Out, "  "+c)
		}
		p.apiUsage(reply)
		return 0
	}
	if reply.Version != "" {
		line := reply.Group + ":" + reply.Artifact + " " + reply.Version
		if reply.Tier != "" {
			line += "  " + reply.Tier
		}
		if reply.Source != "" {
			line += "  " + reply.Source
		}
		fmt.Fprintln(p.Out, line)
	}
	switch reply.Surface {
	case ir.ApiSources:
		fmt.Fprintf(p.Out, "public surface from sources jar: %s\n", filepath.Base(reply.Jar))
	case ir.ApiClasses:
		fmt.Fprintf(p.Out, "public surface from class jar (no parameter names): %s\n", filepath.Base(reply.Jar))
	}
	for _, t := range reply.Types {
		if len(t.Members) == 0 && t.Kind == "" {
			fmt.Fprintln(p.Out, "  "+t.Name) // headline row: a name is the whole answer
			continue
		}
		head := "  " + t.Kind + " " + t.Name
		if t.Doc != "" {
			head += "  # " + t.Doc
		}
		fmt.Fprintln(p.Out, head)
		for _, mem := range t.Members {
			row := "    " + mem.Signature
			if mem.Doc != "" {
				row += "  # " + mem.Doc
			}
			fmt.Fprintln(p.Out, row)
		}
		if more := t.MembersTotal - len(t.Members); more > 0 {
			fmt.Fprintf(p.Out, "    +%d more members (--members)\n", more)
		}
	}
	if more := reply.TypesTotal - len(reply.Types); more > 0 {
		fmt.Fprintf(p.Out, "+%d more types (--limit, or name one: cxj api %s <Type>)\n", more, reply.Query)
	}
	if reply.Note != "" {
		fmt.Fprintln(p.Out, reply.Note)
	}
	p.apiUsage(reply)
	return 0
}

func (p *Printer) apiUsage(reply ir.ApiReply) {
	if len(reply.Usage) == 0 {
		return
	}
	fmt.Fprintf(p.Out, "your call sites (%d)\n", reply.UsageTotal)
	for _, u := range reply.Usage {
		fmt.Fprintf(p.Out, "  %s:%d  %s %s  [%s]\n", u.Path, u.Line, u.Kind, u.Target, u.Certainty)
	}
	if more := reply.UsageTotal - len(reply.Usage); more > 0 {
		fmt.Fprintf(p.Out, "+%d more (--scope all, or a narrower symbol)\n", more)
	}
}

// Generic writes a map-shaped payload as JSON or a compact dump.
func (p *Printer) Generic(v any, asJSON bool) int {
	return p.JSON(v)
}
