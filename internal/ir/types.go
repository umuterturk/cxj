// Package ir holds parser-neutral domain types shared across scan, index, and CLI.
package ir

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
	MatchExact  MatchKind = "exact"
	MatchPrefix MatchKind = "prefix"
	MatchFuzzy  MatchKind = "fuzzy"
	MatchText   MatchKind = "text"
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
