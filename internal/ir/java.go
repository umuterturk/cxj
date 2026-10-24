package ir

// Certainty grades how sure we are about a relation or resolution.
type Certainty string

const (
	CertaintyExact      Certainty = "exact"
	CertaintyCandidate  Certainty = "candidate"
	CertaintyUnresolved Certainty = "unresolved"
	CertaintyAmbiguous  Certainty = "ambiguous"
)

// SymbolKind is a parser-neutral classification of a definition.
type SymbolKind string

const (
	SymbolClass       SymbolKind = "class"
	SymbolInterface   SymbolKind = "interface"
	SymbolEnum        SymbolKind = "enum"
	SymbolMethod      SymbolKind = "method"
	SymbolField       SymbolKind = "field"
	SymbolConstructor SymbolKind = "constructor"
)

// Span is a 1-based line location with optional byte offsets.
type Span struct {
	Line      int
	StartByte uint
	EndByte   uint
}

// Definition is a declared symbol in a file.
type Definition struct {
	Name      string
	Qualified string
	Kind      SymbolKind
	Owner     string // enclosing type qualified name, if any
	Signature string // parameter type summary for methods
	Path      string
	Span      Span
	Certainty Certainty
}

// Occurrence is a name use (call, type reference, etc.).
type Occurrence struct {
	Name      string
	Qualified string // best-effort; may be simple
	Kind      SymbolKind
	Path      string
	Span      Span
	Receiver  string // simple/typed receiver when known
}

// Edge is a candidate relation between symbols (implements, calls, ...).
type Edge struct {
	Kind      string // "calls" | "implements" | "extends" | "overrides"
	From      string
	To        string
	Path      string
	Span      Span
	Certainty Certainty
}

// FileFacts is the parser-neutral extraction result for one source file.
type FileFacts struct {
	Path        string
	Package     string
	Definitions []Definition
	Occurrences []Occurrence
	Edges       []Edge
}

// RefResult is one GET /v1/references hit.
type RefResult struct {
	Path      string    `json:"path"`
	Line      int       `json:"line"`
	Target    string    `json:"target"`
	Certainty Certainty `json:"certainty"`
}

// RefReply is the GET /v1/references payload.
type RefReply struct {
	Results []RefResult `json:"results"`
}
