// Package ir holds parser-neutral domain types shared across scan, index, and CLI,
// plus the ranking rules both the symbol index and the merged `find` surface use.
package ir

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Status is the cx status --json payload.
type Status struct {
	Status           string `json:"status"`
	IndexedFiles     int    `json:"indexed_files"`
	IndexedJavaFiles int    `json:"indexed_java_files"`
	Generation       int    `json:"generation"`
	GoHeapAllocBytes uint64 `json:"go_heap_alloc_bytes"`
}

// MatchKind classifies how a search hit was produced.
type MatchKind string

const (
	MatchExact     MatchKind = "exact"
	MatchPrefix    MatchKind = "prefix"
	MatchFuzzy     MatchKind = "fuzzy"
	MatchSubstring MatchKind = "substring"
	MatchText      MatchKind = "text"
)

// ResultKind classifies the indexed entity.
type ResultKind string

const (
	KindClass  ResultKind = "class"
	KindMethod ResultKind = "method"
	KindField  ResultKind = "field"
	KindText   ResultKind = "text"
	KindConfig ResultKind = "config"
)

// SearchResult is one hit in cx find --json.
type SearchResult struct {
	Path    string     `json:"path"`
	Line    int        `json:"line"`
	Symbol  string     `json:"symbol"`
	Kind    ResultKind `json:"kind"`
	Match   MatchKind  `json:"match"`
	Score   float64    `json:"score"`
	Snippet string     `json:"snippet"`
}

// SearchReply is the cx find --json payload.
type SearchReply struct {
	Results []SearchResult `json:"results"`
}

// DefaultFindLimit is the compact default for cx find --limit.
const DefaultFindLimit = 5

// DefaultShowLines is the hard cap for cx show.
const DefaultShowLines = 80

// RankMatch orders match kinds best-first. It lives here because the tier order is
// part of the payload contract: index uses it to rank symbol hits, engine uses it
// to interleave those with raw text hits. Two private copies of this table existed,
// and when a new kind was added only one was updated - the new kind fell to the
// default and every CLI result sorted it below plain grep matches.
func RankMatch(m MatchKind) int {
	switch m {
	case MatchExact:
		return 0
	case MatchPrefix:
		return 1
	case MatchFuzzy:
		return 2
	case MatchSubstring:
		return 3
	case MatchText:
		return 4
	default:
		return 9
	}
}

// WantsTypeName reports whether a query is spelled like a type: capitalized and
// unqualified. `Handler` is almost certainly a class; `handler` is a collaborator
// field; `PaymentController.pay` already names exactly what it wants, so a dotted
// query is never treated this way.
func WantsTypeName(q string) bool {
	if strings.ContainsRune(q, '.') {
		return false
	}
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(q))
	return unicode.IsUpper(r)
}

// LexicalLess orders two search hits best-first, and is the single ranking used by
// both the symbol index and the merged `find` output. It has to be one function:
// the index sorted symbol hits one way and the merge re-sorted the combined list
// another, so a preference stated only in the first was quietly undone whenever the
// query also matched a text line.
//
// wantsType comes from WantsTypeName on the original query.
func LexicalLess(a, b SearchResult, wantsType bool) bool {
	if pa, pb := lexicalPenalty(a, wantsType), lexicalPenalty(b, wantsType); pa != pb {
		return pa < pb
	}
	if ra, rb := RankMatch(a.Match), RankMatch(b.Match); ra != rb {
		return ra < rb
	}
	if wantsType {
		ta, tb := a.Kind == KindClass, b.Kind == KindClass
		if ta != tb {
			return ta
		}
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Symbol < b.Symbol
}

// lexicalPenalty sinks members that only matched because matching ignores case.
//
// The plan's A7 rule was "for a capitalized query, types rank above members", and a
// within-tier boost alone cannot deliver it: the private field `handler` claims
// MatchExact for the query `Handler` through case-insensitive equality, so it sits
// in a strictly better tier that no within-tier preference can reach. Case is the
// signal that actually separates them - a capitalized query and a lowercase member
// name disagree about what was asked.
//
// It is a sink, not a filter: the member is still returned, behind the types.
func lexicalPenalty(r SearchResult, wantsType bool) int {
	if !wantsType || r.Kind == KindClass || r.Symbol == "" {
		return 0
	}
	name := r.Symbol
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return 0
	}
	r0, _ := utf8.DecodeRuneInString(name)
	if unicode.IsUpper(r0) {
		return 0
	}
	return 1
}
