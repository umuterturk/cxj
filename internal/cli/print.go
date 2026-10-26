// Package cli formats one-shot engine results for stdout.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
		return p.JSON(out)
	}
	if m.JSON {
		return p.JSON(reply)
	}
	if reply.Resolved != "" {
		fmt.Fprintf(p.Out, "resolved %s\n", reply.Resolved)
	}
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
	fmt.Fprintf(p.Out, "kind=%s java_files=%d types=%d test_classes=%d\n",
		reply.Kind, reply.JavaFiles, reply.Types, reply.TestClasses)
	if len(reply.Profiles) > 0 {
		fmt.Fprintf(p.Out, "profiles %s\n", strings.Join(reply.Profiles, ","))
	}
	for _, pkg := range reply.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default)"
		}
		fmt.Fprintf(p.Out, "pkg %s %d\n", name, pkg.Types)
	}
	for _, ep := range reply.Entrypoints {
		if ep.Detail != "" {
			fmt.Fprintf(p.Out, "%s %s %s %s:%d\n", ep.Kind, ep.Detail, ep.Symbol, ep.Path, ep.Line)
			continue
		}
		fmt.Fprintf(p.Out, "%s %s %s:%d\n", ep.Kind, ep.Symbol, ep.Path, ep.Line)
	}
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

// Generic writes a map-shaped payload as JSON or a compact dump.
func (p *Printer) Generic(v any, asJSON bool) int {
	return p.JSON(v)
}
