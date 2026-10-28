package ir

// ApiSurface names which artifact the public surface was read from. It is the
// tier the reader should trust: a sources jar carries doc comments and real
// declarations; the class file is the compiled truth without names for
// parameters. "none" means no artifact was readable and the reply is
// coordinates and workspace usage only.
const (
	ApiSources = "sources"
	ApiClasses = "class"
	ApiNone    = "none"
)

// ApiMember is one public member of a dependency type, rendered from whichever
// tier produced it. Signatures carry types, not parameter names: descriptors
// never had names, and the sources tier reuses the extractor's parameter-type
// summary for the same reason — one honest shape from both tiers.
type ApiMember struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Kind      string `json:"kind,omitempty"` // method | constructor | field
	Doc       string `json:"doc,omitempty"`  // first doc-comment line (sources only)
}

// ApiType is one dependency type. In headline mode (no Type asked for) only
// Name/Path are filled; reading members is exactly the work a bare `api` run
// refuses to do across hundreds of types.
type ApiType struct {
	Name         string      `json:"name"` // dotted, e.g. com.acme.Foo
	Kind         string      `json:"kind,omitempty"`
	Path         string      `json:"path,omitempty"` // jar entry it was read from
	Doc          string      `json:"doc,omitempty"`
	Members      []ApiMember `json:"members,omitempty"`
	MembersTotal int         `json:"members_total,omitempty"` // before the member cap
}

// ApiReply is the cx api --json payload: what a dependency's public surface
// looks like at the version this workspace resolves, and where this workspace
// already uses it.
type ApiReply struct {
	Query    string `json:"query"` // the DEP argument as typed
	Group    string `json:"group,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Version  string `json:"version,omitempty"`
	// Tier is the deps tier that supplied the version (declared/resolved/...),
	// and Source the build file that proved it — the coordinate arrives with its
	// provenance, the way deps rows do.
	Tier   string `json:"tier,omitempty"`
	Source string `json:"source,omitempty"`
	// Ambiguous lists the coordinates the filter matched when the answer was not
	// one (several artifacts, or one artifact at two cached versions). cxj
	// lists; it does not guess.
	Ambiguous []string `json:"ambiguous,omitempty"`

	Jar     string    `json:"jar,omitempty"` // artifact actually opened
	Surface string    `json:"surface"`       // ApiSources | ApiClasses | ApiNone
	Types   []ApiType `json:"types"`
	// TypesTotal counts matching types before the --limit cap; zero means the
	// headline was not truncated (or not enumerable).
	TypesTotal int `json:"types_total"`

	Usage      []RefResult `json:"usage,omitempty"` // call sites in this workspace
	UsageTotal int         `json:"usage_total,omitempty"`
	UsageQuery string      `json:"usage_query,omitempty"` // what was searched
	Note       string      `json:"note,omitempty"`
}
