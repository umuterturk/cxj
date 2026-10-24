// Package ir holds parser-neutral domain types shared across scan, index, and HTTP.
package ir

// SemanticState reports embedding readiness independently of lexical readiness.
type SemanticState string

const (
	SemanticOff         SemanticState = "off"
	SemanticBuilding    SemanticState = "building"
	SemanticReady       SemanticState = "ready"
	SemanticUnavailable SemanticState = "unavailable"
)

// Status is the GET /v1/status payload.
type Status struct {
	Status             string        `json:"status"`
	IndexedFiles       int           `json:"indexed_files"`
	IndexedJavaFiles   int           `json:"indexed_java_files"`
	Generation         int           `json:"generation"`
	SemanticState      SemanticState `json:"semantic_state"`
	SemanticGeneration int           `json:"semantic_generation"`
	Model              string        `json:"model,omitempty"`
	GoHeapAllocBytes   uint64        `json:"go_heap_alloc_bytes"`
}

// MatchKind classifies how a search hit was produced.
type MatchKind string

const (
	MatchExact    MatchKind = "exact"
	MatchPrefix   MatchKind = "prefix"
	MatchFuzzy    MatchKind = "fuzzy"
	MatchText     MatchKind = "text"
	MatchSemantic MatchKind = "semantic"
	MatchHybrid   MatchKind = "hybrid"
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

// SearchResult is one hit in GET /v1/search.
type SearchResult struct {
	Path    string     `json:"path"`
	Line    int        `json:"line"`
	Symbol  string     `json:"symbol"`
	Kind    ResultKind `json:"kind"`
	Match   MatchKind  `json:"match"`
	Score   float64    `json:"score"`
	Snippet string     `json:"snippet"`
}

// SearchReply is the GET /v1/search payload.
type SearchReply struct {
	Results       []SearchResult `json:"results"`
	SemanticState SemanticState  `json:"semantic_state"`
}
