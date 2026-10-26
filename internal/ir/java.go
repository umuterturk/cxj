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
	EndLine   int
	StartByte uint
	EndByte   uint
}

// Definition is a declared symbol in a file.
type Definition struct {
	Name      string
	Qualified string
	Kind      SymbolKind
	Owner     string // enclosing type qualified name, if any
	Package   string
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

// RefKind distinguishes declaration sites from uses.
type RefKind string

const (
	RefKindDef  RefKind = "def"
	RefKindImpl RefKind = "impl"
	RefKindUse  RefKind = "use"
)

// RefResult is one GET /v1/references hit.
type RefResult struct {
	Path      string    `json:"path"`
	Line      int       `json:"line"`
	Target    string    `json:"target"`
	Kind      RefKind   `json:"kind,omitempty"`
	Certainty Certainty `json:"certainty"`
}

// RefReply is the cx references --json payload.
type RefReply struct {
	Results  []RefResult `json:"results"`
	Resolved string      `json:"resolved,omitempty"`
}

// ShowReply is the cx show --json payload.
type ShowReply struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	EndLine   int    `json:"end_line"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	Truncated bool   `json:"truncated"`
	Body      string `json:"body"`
	Resolved  string `json:"resolved,omitempty"`
}

// OutlineMember is one type member in an outline.
type OutlineMember struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	Line      int    `json:"line"`
}

// OutlineType is one type in an outline.
type OutlineType struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Line    int             `json:"line"`
	EndLine int             `json:"end_line"`
	Members []OutlineMember `json:"members"`
}

// OutlineReply is the cx outline --json payload.
type OutlineReply struct {
	Path    string        `json:"path"`
	Package string        `json:"package"`
	Types   []OutlineType `json:"types"`
}

// MapType is one type in a package map.
type MapType struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

// MapPackage is one package in a workspace map.
type MapPackage struct {
	Name  string    `json:"name"`
	Types []MapType `json:"types"`
}

// MapReply is the cx map --json payload.
type MapReply struct {
	Packages []MapPackage `json:"packages"`
}

// Entrypoint is one ingress site (HTTP, scheduled, listener, …).
type Entrypoint struct {
	Kind   string `json:"kind"`
	Symbol string `json:"symbol"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Detail string `json:"detail,omitempty"`
}

// BriefPackage is a package row in cx brief.
type BriefPackage struct {
	Name  string `json:"name"`
	Scope string `json:"scope"` // main | test
	Types int    `json:"types"`
}

// BriefReply is the cx brief --json payload.
type BriefReply struct {
	Kind         string         `json:"kind"`
	JavaFiles    int            `json:"java_files"`
	Types        int            `json:"types"`
	Packages     []BriefPackage `json:"packages"`
	PackageTotal int            `json:"packages_total"`
	Entrypoints  []Entrypoint   `json:"entrypoints"`
	Profiles     []string       `json:"profiles"`
	TestClasses  int            `json:"test_classes"`
}

// TrailHit is one compact related symbol in cx trail.
type TrailHit struct {
	Path      string `json:"path,omitempty"`
	Line      int    `json:"line,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Certainty string `json:"certainty,omitempty"`
}

// TrailReply is the cx trail --json payload.
type TrailReply struct {
	Symbol   string     `json:"symbol"`
	Kind     string     `json:"kind"`
	Path     string     `json:"path"`
	Line     int        `json:"line"`
	EndLine  int        `json:"end_line"`
	Resolved string     `json:"resolved,omitempty"`
	HTTP     []TrailHit `json:"http"`
	Injects  []TrailHit `json:"injects"`
	Callers  []TrailHit `json:"callers"`
	Callees  []TrailHit `json:"callees"`
	Supers   []TrailHit `json:"supers"`
	Subtypes []TrailHit `json:"subtypes"`
	Tests    []TrailHit `json:"tests"`
}

// KeyRow is one config key in cx keys (no values).
type KeyRow struct {
	Key        string   `json:"key"`
	Profiles   []string `json:"profiles"`
	Resolution string   `json:"resolution"`
	Secret     bool     `json:"secret,omitempty"`
}

// KeysReply is the cx keys --json payload.
type KeysReply struct {
	Profiles []string `json:"profiles"`
	Keys     []KeyRow `json:"keys"`
}
