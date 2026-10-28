// Package tshelp holds the handful of tree-sitter helpers every extractor needs.
//
// It exists because each extractor grew its own copy of the same two lines, and one of
// those copies was a performance bug: three independent byte scans from the start of the
// file to find a line number, in three files, with three different implementations that
// did not even agree on whether to decode UTF-8 while doing it (FIX_PLAN P-RC4). A
// helper with one definition cannot drift the way those did (see A-RC10, the same shape
// in the ranking code).
package tshelp

import (
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Line is the 1-based source line a node starts on.
//
// The parser already computes this; reading it back costs one call, where counting
// newlines from byte zero costs a scan proportional to how far into the file the node
// is. A method with many definitions and call sites therefore paid quadratic time in its
// own length, which is what made line numbering 36 % of a cold Open on a real repository.
//
// Two invariants this relies on, both pinned by tests in internal/java: a tree-sitter row
// equals the number of '\n' bytes before the offset plus one, and it does so for CRLF
// files, multi-byte source, text blocks and byte-order marks alike.
//
// There is deliberately no nil guard. Every call site already dereferences the node to get
// its byte offset, so a nil here is a programming error, and returning line 0 would trade a
// loud failure for a plausible-looking wrong line number.
func Line(n *tree_sitter.Node) int {
	return int(n.StartPosition().Row) + 1
}

// EndLine is the 1-based line a node ends on. End offsets are exclusive, so a node ending
// on a newline still reports the line it is on; that case is in the parity test.
func EndLine(n *tree_sitter.Node) int {
	return int(n.EndPosition().Row) + 1
}
