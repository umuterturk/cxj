package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
)

// Dependency ground truth: "which version am I actually on, and where did it come
// from?" — read from the build files and the user's own artifact cache, on demand,
// and nothing else. `deps` deliberately does not open an engine snapshot: build
// files are a dozen reads, not an 850-file scan, and JARs must never approach the
// symbol graph. It also does not reimplement Maven/Gradle resolution. Where a
// version only exists after a resolve (a remote BOM's choice), the honest answer is
// `unknown`, and the tier says so instead of guessing (FIX_PLAN §5.1).

// DepsOptions configures a Deps run.
type DepsOptions struct {
	Root   string
	Filter string // substring of "group:artifact", case-insensitive
	Tier   string // exact tier (see ir.Dep* constants), "" for all
	Limit  int    // rows in the reply; <= 0 means DefaultDepsLimit
	// M2Dir and GradleCache override the local artifact caches; empty means the
	// default ~/.m2/repository and ~/.gradle/caches/modules-2. Tests point these
	// at fixture directories; nothing here ever writes to them.
	M2Dir       string
	GradleCache string
}

const (
	// DefaultDepsLimit bounds the table the same way find's limit does; a
	// real workspace can carry hundreds of managed rows.
	DefaultDepsLimit = 40
	// MaxDepsLimit keeps --limit from turning into a page-fest.
	MaxDepsLimit = 500
)

// Deps scans root's build files, cross-checks the local caches, and answers.
func Deps(o DepsOptions) (ir.DepsReply, error) {
	files, err := findBuildFiles(o.Root)
	if err != nil {
		return ir.DepsReply{}, err
	}

	var hits []versionHit
	var rows []ir.Dep
	catalogs := map[string]*versionCatalog{} // by gradle dir
	locks := map[string]map[string]string{}  // lockfile rel -> "g:a" -> version
	var lockFiles []depFile

	for _, f := range files {
		switch f.base {
		case "pom.xml", "build.gradle", "build.gradle.kts":
			if j, b := parseBuildVersions(f.base, f.data); j != "" || b != "" {
				hits = append(hits, versionHit{depth: strings.Count(f.rel, "/"), java: j, boot: b})
			}
			if f.base == "pom.xml" {
				rows = append(rows, parsePomDeps(f)...)
			} else {
				rows = append(rows, parseGradleDeps(f, catalogs, files)...)
			}
		case "libs.versions.toml":
			// Parsed lazily by parseGradleDeps through the gradle/ convention.
		case "dependencies.lock":
			lf, lock := parseDepLock(f)
			if lock != nil {
				locks[lf.rel] = lock
				lockFiles = append(lockFiles, lf)
			}
		}
	}

	filter := strings.ToLower(strings.TrimSpace(o.Filter))
	rows = dedupeDeps(filterRows(rows, filter))
	applyLocks(locks, lockFiles, rows)

	// A filter searches the lockfile too: it is the transitive answer the build
	// files only approximate. Unfiltered, lockfile-only rows would bury the rows
	// the user actually maintains.
	if filter != "" {
		rows = append(rows, lockOnlyRows(locks, lockFiles, filter, rows)...)
	}

	cache := newDepCache(o.M2Dir, o.GradleCache)
	cache.crossCheck(rows)

	if o.Tier != "" {
		rows = tierRows(rows, strings.ToLower(strings.TrimSpace(o.Tier)))
	}
	sortDeps(rows)

	limit := o.Limit
	switch {
	case limit <= 0:
		limit = DefaultDepsLimit
	case limit > MaxDepsLimit:
		limit = MaxDepsLimit
	}
	shown := rows
	if len(shown) > limit {
		shown = shown[:limit]
	}
	java, boot := pickVersions(hits)
	return ir.DepsReply{
		Java:  java,
		Boot:  boot,
		Files: len(files),
		Count: len(rows),
		Shown: len(shown),
		Deps:  shown,
	}, nil
}

// --- file discovery -------------------------------------------------------------

type depFile struct {
	base string // file name
	rel  string // workspace-relative, slash-separated
	dir  string // absolute directory containing the file
	data []byte
}

// findBuildFiles walks the workspace for dependency sources. It does not use
// scan.Scan on purpose: the scan is tuned for sources and their side files, and
// deps must also see things the scan skips by design (lockfiles next to a root
// settings.gradle, a catalog under gradle/). The skip set is the same intuition:
// build output and VCS internals.
func findBuildFiles(root string) ([]depFile, error) {
	var out []depFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corner of the tree: keep walking
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || skipDepDir(name)) {
				return filepath.SkipDir
			}
			return nil
		}
		base := d.Name()
		switch base {
		case "pom.xml", "build.gradle", "build.gradle.kts", "dependencies.lock", "gradle.properties":
		case "libs.versions.toml":
			// Only the conventional location; stray tomls elsewhere are not catalogs.
			if filepath.Base(filepath.Dir(path)) != "gradle" {
				return nil
			}
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		out = append(out, depFile{
			base: base,
			rel:  filepath.ToSlash(rel),
			dir:  filepath.Dir(path),
			data: data,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, nil
}
func skipDepDir(name string) bool {
	switch name {
	case "target", "build", "out", "dist", "node_modules", "vendor":
		return true
	}
	return false
}

func (f depFile) line(at int) int {
	if at > len(f.data) {
		at = len(f.data)
	}
	return bytes.Count(f.data[:at], []byte{'\n'}) + 1
}

func (f depFile) src(line int) string {
	if line <= 0 {
		return f.rel
	}
	return fmt.Sprintf("%s:%d", f.rel, line)
}

// --- pom.xml --------------------------------------------------------------------

var (
	pomPropertiesBlock = regexp.MustCompile(`(?s)<properties>(.*?)</properties>`)
	pomProperty        = regexp.MustCompile(`(?s)<([A-Za-z][^<>/]*)>\s*([^<]*?)\s*</[A-Za-z][^<>/]*>`)
	pomParent          = regexp.MustCompile(`(?s)<parent>(.*?)</parent>`)
	pomDepMgmt         = regexp.MustCompile(`(?s)<dependencyManagement>(.*?)</dependencyManagement>`)
	pomDependency      = regexp.MustCompile(`(?s)<dependency>(.*?)</dependency>`)
	pomGroupID         = regexp.MustCompile(`(?s)<groupId>\s*([^<]+?)\s*</groupId>`)
	pomArtifactID      = regexp.MustCompile(`(?s)<artifactId>\s*([^<]+?)\s*</artifactId>`)
	pomVersion         = regexp.MustCompile(`(?s)<version>\s*([^<]+?)\s*</version>`)
	pomScope           = regexp.MustCompile(`(?s)<scope>\s*([^<]+?)\s*</scope>`)
	pomType            = regexp.MustCompile(`(?s)<type>\s*([^<]+?)\s*</type>`)
)

func parsePomDeps(f depFile) []ir.Dep {
	s := string(f.data)

	props := map[string]string{}
	for _, blk := range pomPropertiesBlock.FindAllStringSubmatch(s, -1) {
		for _, m := range pomProperty.FindAllStringSubmatch(blk[1], -1) {
			if m[2] != "" {
				props[m[1]] = m[2]
			}
		}
	}

	var out []ir.Dep
	var parentVersion string
	if pm := pomParent.FindStringSubmatch(s); pm != nil {
		g := firstSubMatch(pomGroupID, pm[1])
		a := firstSubMatch(pomArtifactID, pm[1])
		v := substProps(firstSubMatch(pomVersion, pm[1]), props)
		parentVersion = v
		if g != "" && a != "" {
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: v, Tier: ir.DepManaged,
				Source: f.src(f.line(strings.Index(s, "<parent>"))),
				Note:   "parent",
			})
		}
		props["project.parent.version"] = parentVersion
	}

	// dependencyManagement rows are the version authority for this file; the
	// dependency rows themselves then inherit from it.
	managed := map[string]string{}
	dmIdx := pomDepMgmt.FindAllStringSubmatchIndex(s, -1)
	for _, ix := range dmIdx {
		body := s[ix[2]:ix[3]]
		for _, dm := range pomDependency.FindAllStringSubmatchIndex(body, -1) {
			depBody := body[dm[2]:dm[3]]
			g := substProps(firstSubMatch(pomGroupID, depBody), props)
			a := substProps(firstSubMatch(pomArtifactID, depBody), props)
			if g == "" || a == "" {
				continue
			}
			v := substProps(firstSubMatch(pomVersion, depBody), props)
			note := "dependencyManagement"
			if firstSubMatch(pomScope, depBody) == "import" && firstSubMatch(pomType, depBody) == "pom" {
				note = "BOM import"
			}
			tier := ir.DepManaged
			if unresolved(v) {
				tier = ir.DepUnknown
			}
			if v != "" && !unresolved(v) {
				managed[g+":"+a] = v
			}
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: cleanVer(v, tier), Tier: tier,
				Source: f.src(f.line(ix[2] + dm[0])),
				Note:   note,
			})
		}
	}

	inDM := func(at int) bool {
		for _, ix := range dmIdx {
			if at >= ix[0] && at < ix[1] {
				return true
			}
		}
		return false
	}

	for _, m := range pomDependency.FindAllStringSubmatchIndex(s, -1) {
		at := m[0]
		if inDM(at) {
			continue
		}
		body := s[m[2]:m[3]]
		g := substProps(firstSubMatch(pomGroupID, body), props)
		a := substProps(firstSubMatch(pomArtifactID, body), props)
		if g == "" || a == "" {
			continue
		}
		v := substProps(firstSubMatch(pomVersion, body), props)
		line := f.line(at)
		switch {
		case unresolved(v):
			// A ${placeholder} that survived substitution stays visible: a row
			// showing the raw expression is diagnosable, one showing nothing is not.
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Tier: ir.DepUnknown,
				Source: f.src(line),
				Note:   "unresolved property " + v,
			})
		case v != "":
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: v, Tier: ir.DepDeclared,
				Source: f.src(line),
			})
		case managed[g+":"+a] != "":
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: managed[g+":"+a],
				Tier: ir.DepManaged, Source: f.src(line),
				Note: "version from dependencyManagement",
			})
		case parentVersion != "":
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Tier: ir.DepUnknown,
				Source: f.src(line),
				Note:   "version set by parent " + parentVersion + " (not on disk)",
			})
		default:
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Tier: ir.DepUnknown,
				Source: f.src(line),
				Note:   "no version found",
			})
		}
	}
	return out
}

// --- gradle ---------------------------------------------------------------------

var (
	gradleCoord     = regexp.MustCompile(`["']([\w.\-]+):([\w.\-]+)(?::([^"'\s]+))?["']`)
	gradleLibRef    = regexp.MustCompile(`libs\.([\w.]+)`)
	gradleLocalProp = regexp.MustCompile(`(?m)^\s*([\w.]+)\s*=\s*["']([^"'\s$]+)["']`)
)

func parseGradleDeps(f depFile, catalogs map[string]*versionCatalog, files []depFile) []ir.Dep {
	cat := gradleCatalog(f, catalogs, files)
	props := gradleProps(f, files)
	// `springCloudVersion = "2025.1.2"` earlier in the same file is the classic
	// BOM spelling. It is on disk, so cxj reads it; gradle.properties still wins
	// because that is the layered order Gradle itself would apply.
	for _, m := range gradleLocalProp.FindAllStringSubmatch(string(f.data), -1) {
		if _, in := props[m[1]]; !in {
			props[m[1]] = m[2]
		}
	}
	var out []ir.Dep
	hasBOM := false

	// Pass 1 finds the BOMs, because a version-less dependency is only
	// "managed by a platform" if a platform line actually exists.
	if bytes.Contains(f.data, []byte("platform(")) {
		hasBOM = true
	}

	for i, raw := range strings.Split(string(f.data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}
		if cut := strings.Index(line, "//"); cut >= 0 {
			line = line[:cut]
		}

		platform := strings.Contains(line, "platform(")

		for _, m := range gradleCoord.FindAllStringSubmatchIndex(line, -1) {
			g := line[m[2]:m[3]]
			a := line[m[4]:m[5]]
			v := ""
			if m[6] >= 0 {
				v = line[m[6]:m[7]]
			}
			d := gradleDep(g, a, v, platform, f, i+1)
			// `"$springCloudVersion"` is the normal way a Gradle BOM version is
			// written. gradle.properties is on disk, so resolving it is reading,
			// not guessing — but a property cxj cannot see stays an unknown, not
			// a row cheerfully declaring "$springCloudVersion" as a version.
			if v != "" && strings.ContainsRune(v, '$') {
				rv, ok := substGradleVer(v, props)
				if !ok {
					d.Version = ""
					d.Tier = ir.DepUnknown
					d.Note = "unresolved property " + rv
				} else {
					d.Version = rv
					if d.Tier == ir.DepDeclared {
						d.Note = joinNote(d.Note, "version from build property")
					}
				}
			}
			out = append(out, d)
		}
		for _, m := range gradleLibRef.FindAllStringSubmatch(line, -1) {
			ref := m[1]
			if strings.HasPrefix(ref, "plugins.") || strings.HasSuffix(ref, ".plugins") {
				continue // a plugin version reference, not a dependency
			}
			g, a, v, note, ok := lookupCatalog(cat, ref)
			if !ok {
				if !platform {
					out = append(out, ir.Dep{
						Tier: ir.DepUnknown, Source: f.src(i + 1),
						Note: "catalog reference " + ref + " not found",
					})
				}
				continue
			}
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: v,
				Tier: ir.DepManaged, Source: f.src(i + 1),
				Note: "version catalog: " + note,
			})
		}
	}
	note := "no version found"
	if hasBOM {
		note = "version from platform/BOM"
	}
	for i := range out {
		if out[i].Tier == ir.DepUnknown && out[i].Note == "" {
			out[i].Note = note
		}
	}
	return out
}

func gradleDep(g, a, v string, platform bool, f depFile, line int) ir.Dep {
	d := ir.Dep{Group: g, Artifact: a, Source: f.src(line)}
	switch {
	case platform && v != "":
		d.Version = v
		d.Tier = ir.DepManaged
		d.Note = "platform (BOM)"
	case platform:
		d.Tier = ir.DepUnknown
		d.Note = "platform without version"
	case v != "":
		d.Version = v
		d.Tier = ir.DepDeclared
	default:
		d.Tier = ir.DepUnknown
		// The note depends on whether this file actually has a platform line,
		// which the caller knows and fills in after the scan.
	}
	return d
}

// gradleCatalog finds the conventional version catalog for a build file:
// <root>/gradle/libs.versions.toml, where <root> is the build file's module
// directory or one of its ancestors (the multi-module case: children inherit the
// root's catalog). The nearest match wins, and each catalog is parsed once.
func gradleCatalog(f depFile, catalogs map[string]*versionCatalog, files []depFile) *versionCatalog {
	var best *depFile
	bestRoot := -1
	for i := range files {
		c := &files[i]
		if c.base != "libs.versions.toml" {
			continue
		}
		// c.dir is the gradle/ directory; its parent is the repo root it serves.
		moduleRoot := filepath.Dir(c.dir)
		if f.dir != moduleRoot && !strings.HasPrefix(f.dir+string(filepath.Separator), moduleRoot+string(filepath.Separator)) {
			continue
		}
		if len(moduleRoot) > bestRoot {
			best, bestRoot = c, len(moduleRoot)
		}
	}
	if best == nil {
		return nil
	}
	cat, ok := catalogs[best.rel]
	if !ok {
		cat = parseVersionCatalog(*best)
		catalogs[best.rel] = cat
	}
	return cat
}

// gradleProps reads the nearest gradle.properties (Gradle layers the project's
// own file over the root's; the nearest ancestor file wins the same way).
func gradleProps(f depFile, files []depFile) map[string]string {
	props := map[string]string{}
	var best *depFile
	bestRoot := -1
	for i := range files {
		c := &files[i]
		if c.base != "gradle.properties" {
			continue
		}
		if c.dir != f.dir && !strings.HasPrefix(f.dir+string(filepath.Separator), c.dir+string(filepath.Separator)) {
			continue
		}
		if len(c.dir) > bestRoot {
			best, bestRoot = c, len(c.dir)
		}
	}
	if best == nil {
		return props
	}
	for _, raw := range strings.Split(string(best.data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		props[strings.TrimSpace(line[:eq])] = strings.TrimSpace(line[eq+1:])
	}
	return props
}

var gradlePropRef = regexp.MustCompile(`\$\{?([\w.\-]+)\}?`)

func substGradleVer(v string, props map[string]string) (string, bool) {
	all := true
	out := gradlePropRef.ReplaceAllStringFunc(v, func(m string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")
		key = strings.TrimPrefix(key, "$")
		if val, ok := props[key]; ok {
			return val
		}
		all = false
		return m
	})
	return out, all
}

// --- version catalog (libs.versions.toml) ----------------------------------------

type versionCatalog struct {
	versions  map[string]string
	libraries map[string]catalogLib // canonical-keyed
}

type catalogLib struct {
	group, name, version string
	versionRef           string
}

func parseVersionCatalog(f depFile) *versionCatalog {
	cat := &versionCatalog{
		versions:  map[string]string{},
		libraries: map[string]catalogLib{},
	}
	section := ""
	for _, raw := range strings.Split(string(f.data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		alias := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		switch section {
		case "versions":
			cat.versions[canonCatalogKey(alias)] = unquoteToml(val)
		case "libraries":
			lib := parseCatalogLib(val)
			if lib.group == "" || lib.name == "" {
				continue
			}
			cat.libraries[canonCatalogKey(alias)] = lib
		}
	}
	// Resolve version.ref now that [versions] is complete.
	for k, lib := range cat.libraries {
		if lib.version == "" && lib.versionRef != "" {
			lib.version = cat.versions[canonCatalogKey(lib.versionRef)]
			cat.libraries[k] = lib
		}
	}
	return cat
}

func parseCatalogLib(val string) catalogLib {
	var lib catalogLib
	if strings.HasPrefix(val, "\"") {
		g, a, v := splitCoord(unquoteToml(val))
		lib.group, lib.name, lib.version = g, a, v
		return lib
	}
	body := strings.TrimSuffix(strings.TrimPrefix(val, "{"), "}")
	for _, part := range strings.Split(body, ",") {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		v := unquoteToml(strings.TrimSpace(part[eq+1:]))
		switch key {
		case "group":
			lib.group = v
		case "name":
			lib.name = v
		case "version":
			lib.version = v
		case "version.ref":
			lib.versionRef = v
		case "module":
			g, a, ver := splitCoord(v)
			lib.group, lib.name, lib.version = g, a, ver
		}
	}
	return lib
}

// lookupCatalog maps a build-script accessor (libs.foo.barBaz) onto a catalog
// library. The alias and the accessor differ by convention only: dashes in the
// alias become dots (and sometimes camel humps) in the accessor.
func lookupCatalog(cat *versionCatalog, access string) (group, artifact, version, alias string, ok bool) {
	if cat == nil {
		return "", "", "", "", false
	}
	key := canonCatalogKey(access)
	for _, cand := range []string{key, strings.ReplaceAll(key, ".", "-")} {
		if lib, has := cat.libraries[cand]; has {
			return lib.group, lib.name, lib.version, strings.ReplaceAll(cand, ".", "-"), true
		}
	}
	return "", "", "", "", false
}

// canonCatalogKey turns both "spring-cloud-gateway" and "springCloud.gateway"
// into "spring.cloud.gateway" so aliases and accessors collide.
func canonCatalogKey(s string) string {
	var segs []string
	for _, seg := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' || r == '_' }) {
		segs = append(segs, camelSplit(seg)...)
	}
	return strings.ToLower(strings.Join(segs, "."))
}

// camelSplit breaks a camel-hump accessor segment at lower-to-upper transitions
// only, so the acronym a Gradle alias would never contain stays intact:
// "springCloud" splits, "SDK" does not.
func camelSplit(s string) []string {
	var out []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' && (s[i-1] >= 'a' && s[i-1] <= 'z' || s[i-1] >= '0' && s[i-1] <= '9') {
			out = append(out, strings.ToLower(s[start:i]))
			start = i
		}
	}
	return append(out, strings.ToLower(s[start:]))
}

func unquoteToml(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// --- lockfile --------------------------------------------------------------------

// dependencies.lock is the gradle-lockfile-plugin JSON:
// {"dependencies": {"compileClasspath": [{"group": "g", "name": "a", "version": "v"}]}}.
// Whatever it pins is what the last build actually used, which is the strongest
// version evidence cxj can read without running anything.
func parseDepLock(f depFile) (depFile, map[string]string) {
	var doc struct {
		Dependencies map[string][]struct {
			Group   string `json:"group"`
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(f.data, &doc); err != nil || len(doc.Dependencies) == 0 {
		return f, nil
	}
	lock := map[string]string{}
	for _, entries := range doc.Dependencies {
		for _, e := range entries {
			if e.Group == "" || e.Name == "" {
				continue
			}
			key := e.Group + ":" + e.Name
			if _, seen := lock[key]; !seen {
				lock[key] = strings.TrimSpace(e.Version)
			}
		}
	}
	return f, lock
}

// applyLocks upgrades rows whose coordinate the lockfile pins: the locked version
// is what the last build actually resolved, which outranks anything the build
// file intended. A pin like "5.+" is dynamic — real, but not a version, so it is
// left alone rather than reported as a version.
func applyLocks(locks map[string]map[string]string, lockFiles []depFile, rows []ir.Dep) {
	for i := range rows {
		key := rows[i].Group + ":" + rows[i].Artifact
		for _, lf := range lockFiles {
			v, ok := locks[lf.rel][key]
			if !ok {
				continue
			}
			if v == "" || strings.ContainsAny(v, "+*") {
				continue
			}
			note := "locked"
			if rows[i].Version != "" && rows[i].Version != v {
				note = "locked (build file says " + rows[i].Version + ")"
			}
			rows[i].Version = v
			rows[i].Tier = ir.DepResolved
			rows[i].Note = note
			break
		}
	}
}

// lockOnlyRows answers from the lockfile for coordinates no build file declares:
// the transitives. Rows the build files already speak for are skipped; they have
// richer provenance.
func lockOnlyRows(locks map[string]map[string]string, lockFiles []depFile, filter string, have []ir.Dep) []ir.Dep {
	known := map[string]bool{}
	for _, d := range have {
		known[d.Group+":"+d.Artifact] = true
	}
	var out []ir.Dep
	for _, lf := range lockFiles {
		keys := make([]string, 0, len(locks[lf.rel]))
		for key := range locks[lf.rel] {
			keys = append(keys, key)
		}
		sort.Strings(keys) // map order must not reach the output
		for _, key := range keys {
			v := locks[lf.rel][key]
			if known[key] || !strings.Contains(strings.ToLower(key), filter) {
				continue
			}
			g, a, _ := splitCoord(key) // the key is "g:a"; the version lives in the map
			dyn := v == "" || strings.ContainsAny(v, "+*")
			tier, note := ir.DepResolved, "from lockfile (not declared in build files)"
			if dyn {
				// A dynamic pin is real evidence the dependency exists, but the
				// lock never resolved it to one version — `resolved` would lie.
				tier, note = ir.DepUnknown, "dynamic pin in lockfile"
			}
			out = append(out, ir.Dep{
				Group: g, Artifact: a, Version: v, Tier: tier,
				Source: lf.src(0),
				Note:   note,
			})
		}
	}
	return out
}

// --- local cache -----------------------------------------------------------------

type depCache struct {
	m2     string
	gradle string
	memos  map[string][]string
	any    bool
}

func newDepCache(m2, gradle string) depCache {
	c := depCache{m2: m2, gradle: gradle, memos: map[string][]string{}}
	home, err := os.UserHomeDir()
	if err == nil {
		if c.m2 == "" {
			c.m2 = filepath.Join(home, ".m2", "repository")
		}
		if c.gradle == "" {
			c.gradle = filepath.Join(home, ".gradle", "caches", "modules-2")
		}
	}
	if st, err := os.Stat(c.m2); err == nil && st.IsDir() {
		c.any = true
	}
	if st, err := os.Stat(c.gradle); err == nil && st.IsDir() {
		c.any = true
	}
	return c
}

// versions lists the cached versions of g:a, judged by directory listing only
// (with one look for a .jar inside, since an empty version directory is a
// download that never finished). m2 stores the group as nested directories;
// gradle's modules-2 keeps it as one literal directory under files-2.1.
func (c *depCache) versions(group, artifact string) []string {
	key := group + ":" + artifact
	if v, ok := c.memos[key]; ok {
		return v
	}
	var out []string
	seen := map[string]bool{}
	collect := func(dir string, nested bool) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if seen[e.Name()] {
				continue
			}
			if hasCachedJar(filepath.Join(dir, e.Name()), nested) {
				seen[e.Name()] = true
				out = append(out, e.Name())
			}
		}
	}
	if c.m2 != "" {
		collect(filepath.Join(c.m2, strings.ReplaceAll(group, ".", "/"), artifact), false)
	}
	if c.gradle != "" {
		collect(filepath.Join(c.gradle, "files-2.1", group, artifact), true)
	}
	sort.Strings(out)
	c.memos[key] = out
	return out
}

func hasCachedJar(versionDir string, nested bool) bool {
	ents, err := os.ReadDir(versionDir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jar") {
			return true
		}
		if nested && e.IsDir() {
			inner, err := os.ReadDir(filepath.Join(versionDir, e.Name()))
			if err != nil {
				continue
			}
			for _, f := range inner {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".jar") {
					return true
				}
			}
		}
	}
	return false
}

// crossCheck raises rows to what the cache proves, and annotates the rest.
func (c *depCache) crossCheck(rows []ir.Dep) {
	if !c.any {
		return
	}
	for i := range rows {
		cand := c.versions(rows[i].Group, rows[i].Artifact)
		if len(cand) == 0 {
			continue
		}
		if rows[i].Version != "" {
			for _, v := range cand {
				if v == rows[i].Version {
					rows[i].Note = joinNote(rows[i].Note, "cached")
					break
				}
			}
			continue
		}
		switch {
		case len(cand) == 1:
			rows[i].Version = cand[0]
			rows[i].Tier = ir.DepCached
			rows[i].Note = cacheNote(rows[i].Note, "only version in local cache")
		case len(cand) > 1:
			rows[i].Tier = ir.DepCached
			list := cand
			more := ""
			if len(list) > 3 {
				more = fmt.Sprintf(", +%d more", len(list)-3)
				list = list[:3]
			}
			rows[i].Note = cacheNote(rows[i].Note, "ambiguous: "+strings.Join(list, ", ")+more)
		}
	}
}

// --- shaping ---------------------------------------------------------------------

func filterRows(rows []ir.Dep, filter string) []ir.Dep {
	if filter == "" {
		return rows
	}
	var out []ir.Dep
	for _, d := range rows {
		if strings.Contains(strings.ToLower(d.Group+":"+d.Artifact), filter) {
			out = append(out, d)
		}
	}
	return out
}

func tierRows(rows []ir.Dep, tier string) []ir.Dep {
	var out []ir.Dep
	for _, d := range rows {
		if d.Tier == tier {
			out = append(out, d)
		}
	}
	return out
}

// dedupeDeps collapses repeated declarations of the same coordinate (the same
// dependency line usually appears in more than one configuration, and the same
// version often comes from two modules). The first occurrence keeps its place;
// the rest leave a count in the note.
func dedupeDeps(rows []ir.Dep) []ir.Dep {
	type k struct{ g, a, v, tier string }
	seen := map[k]int{}
	var out []ir.Dep
	for _, d := range rows {
		key := k{d.Group, d.Artifact, d.Version, d.Tier}
		if idx, ok := seen[key]; ok {
			out[idx].Note = joinNote(out[idx].Note, fmt.Sprintf("declared %d times", seen[key]+2))
			seen[key]++
			continue
		}
		seen[key] = 1
		out = append(out, d)
	}
	return out
}

// sortDeps puts the headline frameworks first (the plan's "boot, core first"),
// then coordinates alphabetically. Version is part of the key because two
// versions of one artifact are two rows and must order deterministically.
func sortDeps(rows []ir.Dep) {
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := depRank(rows[i]), depRank(rows[j])
		if ri != rj {
			return ri < rj
		}
		if rows[i].Group != rows[j].Group {
			return rows[i].Group < rows[j].Group
		}
		if rows[i].Artifact != rows[j].Artifact {
			return rows[i].Artifact < rows[j].Artifact
		}
		if rows[i].Version != rows[j].Version {
			return rows[i].Version < rows[j].Version
		}
		return rows[i].Source < rows[j].Source
	})
}

func depRank(d ir.Dep) int {
	switch {
	case strings.HasPrefix(d.Group, "org.springframework.boot"):
		return 0
	case strings.HasPrefix(d.Group, "org.springframework"):
		return 1
	default:
		return 2
	}
}

// --- small helpers ----------------------------------------------------------------

func firstSubMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// substProps expands ${property} references. Unknown placeholders survive
// verbatim (unresolved() then flags the row) instead of becoming empty strings:
// a row that shows `${foo.version}` is diagnosable, one that shows nothing is not.
func substProps(s string, props map[string]string) string {
	out, _ := substPass(s, props, 0)
	return out
}

func substPass(s string, props map[string]string, depth int) (string, bool) {
	var b strings.Builder
	all := true
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			b.WriteString(s)
			break
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			b.WriteString(s)
			all = false
			break
		}
		b.WriteString(s[:i])
		if depth < 6 {
			if v, ok := props[s[i+2:i+j]]; ok {
				sub, okSub := substPass(v, props, depth+1)
				b.WriteString(sub)
				if !okSub {
					all = false
				}
				s = s[i+j+1:]
				continue
			}
		}
		b.WriteString(s[i : i+j+1])
		all = false
		s = s[i+j+1:]
	}
	return b.String(), all
}

func unresolved(v string) bool { return strings.Contains(v, "${") }

func cleanVer(v string, tier string) string {
	if tier == ir.DepUnknown && unresolved(v) {
		return ""
	}
	return v
}

func splitCoord(s string) (group, artifact, version string) {
	parts := strings.SplitN(s, ":", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	default:
		return "", "", ""
	}
}

func joinNote(existing, add string) string {
	if existing == "" {
		return add
	}
	if strings.Contains(existing, add) {
		return existing
	}
	return existing + "; " + add
}

// cacheNote promotes a row whose version the cache supplied. A row that said
// "no version found" no longer means that once the cache fills it in, so the
// placeholder is dropped rather than stacked next to its own contradiction.
// Notes that carry information of their own ("version set by parent 3.2.0")
// stay and gain the cache fact beside them.
func cacheNote(existing, add string) string {
	switch {
	case strings.HasPrefix(existing, "no version found"),
		strings.HasPrefix(existing, "version from platform/BOM"),
		strings.HasPrefix(existing, "unresolved property"):
		return add
	default:
		return joinNote(existing, add)
	}
}
