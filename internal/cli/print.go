// Package cli formats one-shot engine results for stdout.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"example.com/cx/internal/ir"
)

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

// Find writes search results.
func (p *Printer) Find(reply ir.SearchReply, asJSON bool) int {
	if asJSON {
		return p.JSON(reply)
	}
	for _, r := range reply.Results {
		fmt.Fprintf(p.Out, "%s:%d\t%s\t%s\t%s\n", r.Path, r.Line, r.Symbol, r.Kind, r.Match)
	}
	return 0
}

// References writes reference results.
func (p *Printer) References(reply ir.RefReply, asJSON bool) int {
	if asJSON {
		return p.JSON(reply)
	}
	for _, r := range reply.Results {
		fmt.Fprintf(p.Out, "%s:%d\t%s\t%s\n", r.Path, r.Line, r.Target, r.Certainty)
	}
	return 0
}

// Generic writes a map-shaped payload as JSON or a compact dump.
func (p *Printer) Generic(v any, asJSON bool) int {
	return p.JSON(v)
}
