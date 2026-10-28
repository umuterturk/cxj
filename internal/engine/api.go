package engine

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/java"
)

// The dependency-API answer (FIX_PLAN §5.2): "what is the real public surface of
// this dependency *at the version this build resolves*, and where do I already
// use it?" — read from the user's own artifact cache and workspace, on demand.
// Context7 answers this from a hosted index of some version someone published;
// cxj answers it from the jar this machine compiled against, offline.
//
// The tier ladder mirrors deps' honesty: a sources jar gives declarations plus
// doc-comment first lines; a plain class jar gives the compiled truth with types
// but no parameter names (descriptors never carried names); no artifact at all
// still yields the resolved coordinates, the one command that would fetch
// sources, and the workspace usage half of the answer, which needs no jar.
//
// No state: two zip reads, bounded parses, exit. The workspace half arrives
// through an Engine the caller already opened — and `--sources-only` never
// opens one, because a jar question must not pay for a repository scan.

// ApiOptions configures an Api run.
type ApiOptions struct {
	Root   string
	Dep    string // dependency filter, same language as deps' (substring of g:a)
	Symbol string // "Type" or "Type.member"; empty asks for the headline list
	Limit  int    // types in the reply; <= 0 means DefaultApiLimit
	// MemberLimit caps rendered members per type; <= 0 means DefaultApiMembers.
	MemberLimit int
	// SkipSurface suppresses the artifact read (--from-repo-only); SkipUsage
	// suppresses the workspace half (--sources-only). The CLI enforces that at
	// least one of them is false.
	SkipSurface bool
	SkipUsage   bool
	// Eng answers the usage tier. Nil means no workspace half.
	Eng *Engine
	// M2Dir and GradleCache override the artifact caches, same as DepsOptions.
	M2Dir       string
	GradleCache string
}

const (
	// DefaultApiLimit caps headline types; a real jar carries hundreds.
	DefaultApiLimit = 40
	// MaxApiLimit bounds --limit.
	MaxApiLimit = 500
	// DefaultApiMembers caps rendered members per type.
	DefaultApiMembers = 80
	// apiMaxFiles bounds how many sources entries get a Tree-sitter parse;
	// only types that matched the query reach this, and a match that spans
	// more than a handful of files is a query the user should tighten.
	apiMaxFiles = 8
	// apiMaxUsage is how many call sites the usage tier shows before "+N more".
	apiMaxUsage = 10
	// apiMaxEntry bounds one zip entry read (a .java over 2 MiB is not a source file).
	apiMaxEntry = 2 << 20
)

// Api resolves the dependency, reads its public surface, and collects usage.
func Api(o ApiOptions) (ir.ApiReply, error) {
	if strings.TrimSpace(o.Dep) == "" {
		return ir.ApiReply{}, fmt.Errorf("dependency required")
	}
	limit := o.Limit
	switch {
	case limit <= 0:
		limit = DefaultApiLimit
	case limit > MaxApiLimit:
		limit = MaxApiLimit
	}
	memberLimit := o.MemberLimit
	if memberLimit <= 0 {
		memberLimit = DefaultApiMembers
	}

	reply := ir.ApiReply{Query: o.Dep, Surface: ir.ApiNone, Types: []ir.ApiType{}}
	want, member := splitTypeMember(o.Symbol)

	// 1. The coordinate, with its provenance, from the build files and cache.
	if !o.SkipSurface {
		dep, err := Deps(DepsOptions{
			Root: o.Root, Filter: o.Dep, Limit: MaxDepsLimit,
			M2Dir: o.M2Dir, GradleCache: o.GradleCache,
		})
		if err != nil {
			return ir.ApiReply{}, err
		}
		coords, unresolvedAt := uniqueCoords(dep.Deps)
		switch {
		case len(coords) == 1:
			c := coords[0]
			reply.Group, reply.Artifact, reply.Version = c.group, c.artifact, c.version
			reply.Tier, reply.Source = c.tier, c.source // 2. The surface, sources jar first, then the compiled jar.
			cache := newDepCache(o.M2Dir, o.GradleCache)
			if path, ok := cache.jarPath(c.group, c.artifact, c.version, "sources"); ok {
				types, total, note, err := surfaceFromSources(path, want, member, limit, memberLimit)
				if err != nil {
					reply.Note = "sources jar unreadable: " + err.Error()
				} else {
					reply.Jar, reply.Surface, reply.Types, reply.TypesTotal = path, ir.ApiSources, types, total
					reply.Note = note
				}
			} else if path, ok := cache.jarPath(c.group, c.artifact, c.version, ""); ok {
				types, total, note, err := surfaceFromClasses(path, want, member, limit, memberLimit)
				if err != nil {
					reply.Note = "class jar unreadable: " + err.Error()
				} else {
					reply.Jar, reply.Surface, reply.Types, reply.TypesTotal = path, ir.ApiClasses, types, total
					reply.Note = note
				}
			} else {
				reply.Note = "no jar for this version in the local caches — fetch sources " +
					"(`mvn dependency:sources` / Gradle `--refresh-dependencies`), then re-run"
			}
		case len(coords) > 1:
			// Several artifacts, or one at two cached versions. Which one this
			// build uses is a resolution answer cxj does not have.
			for _, c := range coords {
				reply.Ambiguous = append(reply.Ambiguous, coordLabel(c))
			}
			reply.Note = "several coordinates match — narrow the dependency name, or resolve the version"
		default:
			if len(unresolvedAt) > 0 {
				reply.Ambiguous = unresolvedAt
				reply.Note = "no version established — see the tiers with `cxj deps " + o.Dep + "`"
			} else {
				reply.Note = "no dependency matches " + o.Dep + " in the build files or lockfiles"
			}
		}
	}

	// 3. The workspace half: where this code already uses the type.
	if o.Eng != nil && !o.SkipUsage && want != "" {
		q := want
		if member != "" {
			q = want + "." + member
		}
		rep := o.Eng.References(q)
		if len(rep.Results) == 0 && member != "" {
			q = want // member-level edges miss external calls; fall back to the type
			rep = o.Eng.References(q)
		}
		reply.UsageQuery = q
		reply.UsageTotal = len(rep.Results)
		if len(rep.Results) > apiMaxUsage {
			reply.Usage = rep.Results[:apiMaxUsage]
		} else {
			reply.Usage = rep.Results
		}
	}
	if o.Eng != nil && !o.SkipUsage && want == "" && len(reply.Ambiguous) == 0 {
		if reply.Note == "" {
			reply.Note = "name a type to see its signatures and your call sites"
		}
	}
	return reply, nil
}

// --- coordinate resolution ----------------------------------------------------

type apiCoord struct {
	group, artifact, version, tier, source string
}

// uniqueCoords distils deps rows into candidate coordinates. One entry per
// distinct g:a@v survives; the answer is only "the" version when exactly one
// does. Rows whose version never resolved are kept separate so the reply can
// say *why* nothing was pinned instead of just nothing.
func uniqueCoords(rows []ir.Dep) (coords []apiCoord, unresolved []string) {
	seen := map[string]bool{}
	for _, d := range rows {
		key := d.Group + ":" + d.Artifact + "@" + d.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		if d.Version == "" {
			unresolved = append(unresolved, fmt.Sprintf("%s:%s (%s: %s)", d.Group, d.Artifact, d.Tier, d.Note))
			continue
		}
		coords = append(coords, apiCoord{
			group: d.Group, artifact: d.Artifact, version: d.Version,
			tier: d.Tier, source: d.Source,
		})
	}
	return coords, unresolved
}

func coordLabel(c apiCoord) string {
	return c.group + ":" + c.artifact + " " + c.version + "  " + c.tier + "  " + c.source
}

// splitTypeMember reads "Type.member": the last dotted segment is the member
// when it starts lowercase, which is Java's own naming law.
func splitTypeMember(s string) (typ, member string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.LastIndexByte(s, '.'); i > 0 {
		last := s[i+1:]
		if last != "" && last[0] >= 'a' && last[0] <= 'z' && !strings.Contains(s, "$") {
			return s[:i], last
		}
	}
	return s, ""
}

// --- artifact location ---------------------------------------------------------

// jarPath finds <artifact>[-<classifier>].jar for one exact version in either
// cache layout: Maven stores it directly under the version directory, Gradle
// one hash directory deeper.
func (c depCache) jarPath(group, artifact, version, classifier string) (string, bool) {
	name := artifact + "-" + version
	if classifier != "" {
		name += "-" + classifier
	}
	name += ".jar"
	if c.m2 != "" {
		p := filepath.Join(c.m2, strings.ReplaceAll(group, ".", "/"), artifact, version, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	if c.gradle != "" {
		base := filepath.Join(c.gradle, "files-2.1", group, artifact, version)
		ents, err := os.ReadDir(base)
		if err == nil {
			names := make([]string, 0, len(ents))
			for _, e := range ents {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names) // the hash dirs are opaque; keep the pick deterministic
			for _, n := range names {
				p := filepath.Join(base, n, name)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p, true
				}
			}
		}
	}
	return "", false
}

// --- zip plumbing ---------------------------------------------------------------

// jarEntry is one selected zip member, already read into memory.
type jarEntry struct {
	name string // raw entry path
	data []byte
}

// eachJarEntry streams matching entries of a jar through fn, stopping early when
// fn reports stop or the cap is reached. Only the needed entries are ever read:
// a sources jar can carry 5k files and the query wants one.
func eachJarEntry(jarPath, suffix string, wantNames func(string) bool, max int, fn func(jarEntry) bool) error {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	count := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, suffix) {
			continue
		}
		if wantNames != nil && !wantNames(f.Name) {
			continue
		}
		if max > 0 && count >= max {
			return nil
		}
		if f.UncompressedSize64 > apiMaxEntry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, apiMaxEntry))
		rc.Close()
		if err != nil {
			continue
		}
		count++
		if !fn(jarEntry{name: f.Name, data: data}) {
			return nil
		}
	}
	return nil
}

// entryFQN renders an entry path as a dotted type name ("com/acme/Foo.java" ->
// "com.acme.Foo"); the suffix is the extension to strip.
func entryFQN(entry, suffix string) string {
	s := strings.TrimSuffix(entry, suffix)
	return strings.ReplaceAll(s, "/", ".")
}

// typeEntryMatches answers whether an entry's FQN is the queried type: exact,
// by simple name, or an inner class of the matched top-level.
func typeEntryMatches(fqn, want string) bool {
	if want == "" {
		return false
	}
	if fqn == want || strings.HasSuffix(fqn, "."+want) {
		return true
	}
	base := fqn
	if i := strings.IndexByte(base, '$'); i >= 0 {
		base = fqn[:i]
	} else {
		return false
	}
	return base == want || strings.HasSuffix(base, "."+want)
}

// headlineNames lists top-level type names in a jar (inner classes, `$`, and
// module/package-info excluded), sorted and capped. Names only: reading every
// entry to classify hundreds of types is the work a bare `api` refuses.
func headlineNames(jarPath, suffix string, limit int) (names []string, total int, err error) {
	var all []string
	err = eachJarEntry(jarPath, suffix, func(name string) bool {
		base := name[strings.LastIndexByte(name, '/')+1:]
		if strings.Contains(base, "$") || base == "module-info"+suffix || base == "package-info"+suffix {
			return false
		}
		return true
	}, 0, func(e jarEntry) bool {
		all = append(all, entryFQN(e.name, suffix))
		return true
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(all)
	total = len(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, total, nil
}

// --- sources tier ----------------------------------------------------------------

// surfaceFromSources renders declarations from the -sources.jar: real Java, run
// through the same extractor as workspace code, plus each item's doc-comment
// first line. That is the richest answer cxj can get without a network.
func surfaceFromSources(jarPath, want, member string, limit, memberLimit int) ([]ir.ApiType, int, string, error) {
	if want == "" {
		names, total, err := headlineNames(jarPath, ".java", limit)
		if err != nil {
			return nil, 0, "", err
		}
		types := make([]ir.ApiType, 0, len(names))
		for _, n := range names {
			types = append(types, ir.ApiType{Name: n})
		}
		return types, total, "", nil
	}

	ext := java.NewExtractor()
	defer ext.Close()

	var types []ir.ApiType
	var scanned int
	var note string
	err := eachJarEntry(jarPath, ".java", func(name string) bool {
		fqn := entryFQN(name, ".java")
		return typeEntryMatches(fqn, want)
	}, apiMaxFiles, func(e jarEntry) bool {
		scanned++
		facts, err := ext.Extract(e.name, e.data)
		if err != nil {
			return true
		}
		lines := strings.Split(string(e.data), "\n")
		for _, d := range facts.Definitions {
			switch d.Kind {
			case ir.SymbolClass, ir.SymbolInterface, ir.SymbolEnum:
				fqn := d.Qualified
				if fqn == "" {
					fqn = joinPkg(facts.Package, d.Name)
				}
				if !typeEntryMatches(fqn, want) && !typeEntryMatches(d.Name, want) {
					continue // the file carried a different top-level too
				}
				t := ir.ApiType{
					Name: fqn, Kind: string(d.Kind), Path: e.name,
					Doc: docAbove(lines, d.Span.Line),
				}
				for _, m := range facts.Definitions {
					if m.Owner != d.Qualified || m.Kind == ir.SymbolClass {
						continue
					}
					if member != "" && !memberMatches(m, member) {
						continue
					}
					if !declPublic(lines, m.Span.Line, string(d.Kind), m) {
						continue
					}
					t.MembersTotal++
					if len(t.Members) >= memberLimit {
						continue
					}
					t.Members = append(t.Members, ir.ApiMember{
						Name:      m.Name,
						Signature: m.Name + m.Signature,
						Kind:      string(m.Kind),
						Doc:       docAbove(lines, m.Span.Line),
					})
				}
				types = append(types, t)
			}
		}
		return true
	})
	if err != nil {
		return nil, 0, "", err
	}
	if scanned == 0 {
		note = "no type " + want + " in the sources jar"
		if close := closeNames(jarPath, ".java", want, 5); close != "" {
			note += "; closest: " + close
		}
	}
	if types == nil {
		types = []ir.ApiType{}
	}
	return types, len(types), note, nil
}

// --- class tier -------------------------------------------------------------------

// surfaceFromClasses renders the compiled public surface: constant-pool member
// tables through the bounded class reader. Types honest, parameter names absent
// — descriptors never carried them, and the output says which tier it came from.
func surfaceFromClasses(jarPath, want, member string, limit, memberLimit int) ([]ir.ApiType, int, string, error) {
	if want == "" {
		names, total, err := headlineNames(jarPath, ".class", limit)
		if err != nil {
			return nil, 0, "", err
		}
		types := make([]ir.ApiType, 0, len(names))
		for _, n := range names {
			types = append(types, ir.ApiType{Name: n})
		}
		return types, total, "", nil
	}

	var types []ir.ApiType
	var scanned int
	err := eachJarEntry(jarPath, ".class", func(name string) bool {
		return typeEntryMatches(entryFQN(name, ".class"), want)
	}, apiMaxFiles*4, func(e jarEntry) bool {
		scanned++
		cf, err := parseClassFile(e.data)
		if err != nil {
			return true
		}
		short := cf.Name[strings.LastIndexByte(cf.Name, '.')+1:]
		short = strings.ReplaceAll(short, "$", ".")
		t := ir.ApiType{Name: cf.Name, Kind: cf.kindWord(), Path: e.name}
		add := func(m classMember, kind string) {
			sig := prettyMethod(short, m)
			if kind == "field" {
				sig = prettyField(m)
			}
			if sig == "" {
				return
			}
			t.MembersTotal++
			if len(t.Members) >= memberLimit {
				return
			}
			t.Members = append(t.Members, ir.ApiMember{Name: m.Name, Signature: sig, Kind: kind})
		}
		for _, m := range cf.publicFields() {
			if member == "" || memberMatchesName(m.Name, member, short) {
				add(m, "field")
			}
		}
		for _, m := range cf.publicMethods() {
			if member == "" || memberMatchesName(m.Name, member, short) {
				add(m, "method")
			}
		}
		types = append(types, t)
		return true
	})
	if err != nil {
		return nil, 0, "", err
	}
	note := ""
	if scanned == 0 {
		note = "no type " + want + " in the class jar"
		if close := closeNames(jarPath, ".class", want, 5); close != "" {
			note += "; closest: " + close
		}
	}
	if types == nil {
		types = []ir.ApiType{}
	}
	return types, len(types), note, nil
}

// --- member and modifier judgment -------------------------------------------------

func memberMatches(m ir.Definition, want string) bool {
	return memberMatchesName(m.Name, want, "")
}

// memberMatchesName also reads a lowercase query for a constructor as the class
// name, because that is what the source calls it.
func memberMatchesName(name, want, classShort string) bool {
	if name == want {
		return true
	}
	if name == "<init>" && classShort != "" {
		i := strings.LastIndexByte(classShort, '.')
		return classShort[i+1:] == want
	}
	return false
}

// declPublic answers from the declaration line itself: "public" says yes;
// interface members are implicitly public; so are enum constants. Everything
// else — package-private, protected, private — is not the dependency's API.
// Annotations and javadoc sit above the line, so they cannot confuse the check.
func declPublic(lines []string, line int, ownerKind string, m ir.Definition) bool {
	if line < 1 || line > len(lines) {
		return true // no line to judge by: keep the member, the tier is already labeled
	}
	text := strings.TrimSpace(lines[line-1])
	switch {
	case hasWord(text, "public"):
		return true
	case hasWord(text, "private"), hasWord(text, "protected"):
		return false
	case ownerKind == string(ir.SymbolInterface):
		return true
	case ownerKind == string(ir.SymbolEnum) && m.Kind == ir.SymbolField && isConstantName(m.Name):
		return true
	default:
		return false
	}
}

func hasWord(s, word string) bool {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	}) {
		if f == word {
			return true
		}
	}
	return false
}

func isConstantName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			return false
		}
	}
	return true
}

// docAbove takes the first content line of the doc comment above a declaration:
// a first sentence, not the whole javadoc — the row is a pointer, `show` on a
// workspace copy is the reading.
func docAbove(lines []string, line int) string {
	i := line - 2 // line above the declaration, 0-based
	for i >= 0 && i >= line-8 {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			break
		}
		if strings.HasPrefix(t, "@") { // annotations sit between doc and declaration
			i--
			continue
		}
		break
	}
	if i < 0 || !strings.HasSuffix(strings.TrimSpace(lines[i]), "*/") {
		return ""
	}
	for ; i >= 0 && i >= line-40; i-- {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "/*") {
			continue
		}
		// Walk down from the opener for the first line that is content.
		for j := i; j < line-1 && j <= i+30; j++ {
			c := strings.TrimSpace(lines[j])
			c = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c, "/*"), "/**"))
			c = strings.TrimSpace(strings.TrimPrefix(c, "*"))
			if c == "" || strings.HasPrefix(c, "@") || c == "/" || strings.HasPrefix(c, "*/") {
				continue
			}
			if r := []rune(c); len(r) > 120 {
				c = string(r[:117]) + "..."
			}
			return c
		}
		return ""
	}
	return ""
}

// --- suggestions -------------------------------------------------------------------

// closeNames answers "Type not found" with the jar's own vocabulary: entries
// whose lowercase name contains the query's last segment, capped.
func closeNames(jarPath, suffix, want string, max int) string {
	seg := want
	if i := strings.LastIndexByte(seg, '.'); i >= 0 {
		seg = seg[i+1:]
	}
	seg = strings.ToLower(strings.ReplaceAll(seg, "$", "."))
	var hits []string
	seen := map[string]bool{}
	eachJarEntry(jarPath, suffix, nil, 0, func(e jarEntry) bool {
		fqn := strings.ToLower(entryFQN(e.name, suffix))
		base := fqn[strings.LastIndexByte(fqn, '.')+1:]
		if len(hits) >= max || seen[base] || !strings.Contains(base, seg) {
			return true
		}
		seen[base] = true
		hits = append(hits, strings.TrimSuffix(e.name[strings.LastIndexByte(e.name, '/')+1:], suffix))
		return true
	})
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return strings.Join(hits, ", ")
}

func joinPkg(pkg, name string) string {
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}
