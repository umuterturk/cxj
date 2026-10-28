package ir

// Dependency tiers, ordered by how the version was learned. The ladder is honest on
// purpose: cxj does not reimplement Maven/Gradle resolution, so a version a build
// would only discover at resolve time is reported as `unknown`, not guessed.
const (
	// DepDeclared: the version is written at the dependency itself in a build file.
	DepDeclared = "declared"
	// DepResolved: a lockfile pinned this version; it is what the last build used.
	DepResolved = "resolved"
	// DepManaged: the version comes from a shared manager the file can show —
	// a dependencyManagement block, a version catalog — not from the declaration.
	DepManaged = "managed"
	// DepCached: the only evidence is the local artifact cache; several cached
	// versions with nothing to pick between them is ambiguous, and says so.
	DepCached = "cached"
	// DepUnknown: the dependency is real but its version cannot be determined
	// from disk without running the build.
	DepUnknown = "unknown"
)

// Dep is one dependency row: what it is, the best version cxj could establish,
// where that version came from, and where the dependency is declared.
type Dep struct {
	Group    string `json:"group"`
	Artifact string `json:"artifact"`
	Version  string `json:"version,omitempty"`
	Tier     string `json:"tier"`
	// Source is the build file (workspace-relative) and line of the declaration,
	// or of the manager when the manager is the origin.
	Source string `json:"source,omitempty"`
	Note   string `json:"note,omitempty"`
}

// DepsReply is the answer to `cxj deps`.
type DepsReply struct {
	Java string `json:"java,omitempty"`
	Boot string `json:"boot,omitempty"`
	// Files is how many build files were read; it distinguishes "no dependencies"
	// from "no build files", which are very different repos.
	Files int   `json:"files"`
	Count int   `json:"count"` // rows after filtering, before the cap
	Shown int   `json:"shown"`
	Deps  []Dep `json:"deps"`
}
