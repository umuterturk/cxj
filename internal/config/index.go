package config

import (
	"sort"
	"strings"

	"example.com/cx/internal/ir"
)

// Reply is the GET /v1/config payload.
type Reply struct {
	Definitions     []DefOut `json:"definitions"`
	Bindings        []string `json:"bindings"`
	RepositoryValue string   `json:"repository_value,omitempty"`
	Resolution      string   `json:"resolution"`
}

// DefOut is one exported definition.
type DefOut struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Value   string `json:"value"`
	Profile string `json:"profile"`
}

// Index answers config queries.
type Index struct {
	defs     []Definition
	bindings []Binding
	imports  map[string][]string // importer path -> imported basenames
}

// Build constructs a config index.
func Build(files []FileFacts, bindings []Binding) *Index {
	idx := &Index{imports: map[string][]string{}}
	for _, f := range files {
		idx.defs = append(idx.defs, f.Definitions...)
		if len(f.Imports) > 0 {
			idx.imports[f.Path] = append(idx.imports[f.Path], f.Imports...)
		}
	}
	idx.bindings = bindings
	return idx
}

// Query resolves a key for an optional profile.
func (idx *Index) Query(key, profile string) Reply {
	key = strings.TrimSpace(key)
	profile = strings.TrimSpace(profile)
	forms := RelaxedForms(key)

	var defs []Definition
	for _, d := range idx.defs {
		if keyMatches(d.Key, forms) {
			defs = append(defs, d)
		}
	}

	var outDefs []DefOut
	for _, d := range defs {
		outDefs = append(outDefs, DefOut{
			Path: d.Path, Line: d.Line,
			Value:   RedactValue(d.Key, d.Value),
			Profile: d.Profile,
		})
	}
	sort.SliceStable(outDefs, func(i, j int) bool {
		if outDefs[i].Path != outDefs[j].Path {
			return outDefs[i].Path < outDefs[j].Path
		}
		return outDefs[i].Line < outDefs[j].Line
	})

	bindings := idx.bindingSymbols(forms)
	repo, res := idx.resolve(defs, profile, forms)
	if IsSecretKey(key) || secretAmong(defs) {
		repo = RedactValue(key, repo)
	}
	return Reply{
		Definitions:     outDefs,
		Bindings:        bindings,
		RepositoryValue: repo,
		Resolution:      res,
	}
}

func secretAmong(defs []Definition) bool {
	for _, d := range defs {
		if IsSecretKey(d.Key) {
			return true
		}
	}
	return false
}

func keyMatches(defKey string, forms []string) bool {
	df := RelaxedForms(defKey)
	set := map[string]struct{}{}
	for _, f := range df {
		set[strings.ToLower(f)] = struct{}{}
	}
	for _, f := range forms {
		if _, ok := set[strings.ToLower(f)]; ok {
			return true
		}
	}
	return false
}

func (idx *Index) bindingSymbols(forms []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, b := range idx.bindings {
		bf := RelaxedForms(b.Key)
		match := false
		for _, f := range forms {
			for _, x := range bf {
				if strings.EqualFold(f, x) || strings.HasPrefix(strings.ToLower(f), strings.ToLower(x)+".") || strings.HasPrefix(strings.ToLower(x), strings.ToLower(f)+".") {
					match = true
				}
				// prefix binding: ConfigurationProperties prefix "billing" matches billing.max-retries
				if b.Kind == "configuration_properties" && x != "" && strings.HasPrefix(strings.ToLower(f), strings.ToLower(x)+".") {
					match = true
				}
				if b.Kind == "configuration_properties" && strings.EqualFold(f, x) {
					match = true
				}
			}
		}
		if !match {
			continue
		}
		sym := b.Symbol
		if i := strings.IndexByte(sym, '.'); i > 0 && !strings.Contains(sym[:i], "/") {
			// keep type name for record/class bindings
			root := sym
			if j := strings.IndexByte(sym, '.'); j >= 0 {
				root = sym[:j]
			}
			sym = root
		}
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}
	sort.Strings(out)
	return out
}

func (idx *Index) resolve(defs []Definition, profile string, forms []string) (string, string) {
	if len(defs) == 0 {
		return "", "unknown"
	}
	// Prefer profile-specific definitions when profile requested.
	var candidates []Definition
	for _, d := range defs {
		if profile == "" {
			if d.Profile == "" {
				candidates = append(candidates, d)
			}
			continue
		}
		if d.Profile == profile || d.Profile == "" {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		candidates = defs
	}

	// Ranking for repository_value:
	// 1) spring.config.import targets (imported file wins over importer)
	// 2) application-{profile}.*
	// 3) multi-doc / same-file profile == requested
	// 4) default profile empty
	best := pickBest(candidates, profile, idx.imports)
	if best == nil {
		return "", "unknown"
	}
	val := best.Value
	if isPlaceholder(val) {
		return "", "external"
	}
	if IsSecretKey(best.Key) {
		return Redacted, "known"
	}
	return val, "known"
}

// Profiles returns distinct profile names (empty default omitted).
func (idx *Index) Profiles() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, d := range idx.defs {
		if d.Profile == "" {
			continue
		}
		if _, ok := seen[d.Profile]; ok {
			continue
		}
		seen[d.Profile] = struct{}{}
		out = append(out, d.Profile)
	}
	sort.Strings(out)
	return out
}

// Keys lists unique config keys without values.
func (idx *Index) Keys() ir.KeysReply {
	type agg struct {
		profiles map[string]struct{}
		defs     []Definition
	}
	byKey := map[string]*agg{}
	for _, d := range idx.defs {
		a := byKey[d.Key]
		if a == nil {
			a = &agg{profiles: map[string]struct{}{}}
			byKey[d.Key] = a
		}
		a.defs = append(a.defs, d)
		if d.Profile != "" {
			a.profiles[d.Profile] = struct{}{}
		} else {
			a.profiles["default"] = struct{}{}
		}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ir.KeysReply{Profiles: idx.Profiles(), Keys: make([]ir.KeyRow, 0, len(keys))}
	for _, k := range keys {
		a := byKey[k]
		profs := make([]string, 0, len(a.profiles))
		for p := range a.profiles {
			profs = append(profs, p)
		}
		sort.Strings(profs)
		_, res := idx.resolve(a.defs, "", RelaxedForms(k))
		out.Keys = append(out.Keys, ir.KeyRow{
			Key:        k,
			Profiles:   profs,
			Resolution: res,
			Secret:     IsSecretKey(k) || secretAmong(a.defs),
		})
	}
	return out
}

func pickBest(defs []Definition, profile string, imports map[string][]string) *Definition {
	imported := map[string]struct{}{}
	for _, list := range imports {
		for _, name := range list {
			imported[name] = struct{}{}
		}
	}
	score := func(d Definition) int {
		s := 0
		base := baseName(d.Path)
		if _, ok := imported[base]; ok {
			s += 100
		}
		if profile != "" && d.Profile == profile {
			s += 50
		}
		if profile != "" && strings.HasPrefix(base, "application-"+profile+".") {
			s += 40
		}
		if d.Profile == "" {
			s += 10
		}
		// Prefer properties/yml specificity already handled; later files not ordered.
		return s
	}
	var best *Definition
	bestScore := -1
	for i := range defs {
		d := defs[i]
		if profile != "" && d.Profile != "" && d.Profile != profile {
			continue
		}
		sc := score(d)
		if sc > bestScore {
			bestScore = sc
			best = &defs[i]
		}
	}
	// If profile requested and only profile-mismatched left, ignore.
	if best == nil {
		for i := range defs {
			if defs[i].Profile == "" {
				best = &defs[i]
				break
			}
		}
	}
	return best
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func isPlaceholder(v string) bool {
	v = strings.TrimSpace(v)
	return strings.Contains(v, "${") && strings.Contains(v, "}")
}
