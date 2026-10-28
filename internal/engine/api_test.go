package engine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/scan"
)

// B2 fixtures are built at test time exactly as the plan prescribed: a sources
// jar zipped from inline .java, and a class jar carrying the hand-authored
// .class bytes from classfile_test.go. The "cache" is a t.TempDir laid out in
// the real Maven shape. No network, no mvn, no gradle.

func zipJar(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic archives: a failing test must repeat
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(files[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

const barSources = `package com.acme;

/**
 * Does rich things.
 */
public class Bar {

    /** The one constant. */
    public static final String VERSION = "1.0";

    private String hidden = "x";

    /**
     * Computes a value.
     */
    public String compute(int n) {
        return privateHelper() + n;
    }

    private String privateHelper() {
        return "h";
    }
}
`

type apiFixture struct {
	root string
	m2   string
}

// newAPIFixture lays out a workspace with three declared dependencies at
// pinned versions, plus a fake m2 holding: rich with a sources jar, bare with
// only a class jar, and missing with nothing.
func newAPIFixture(t *testing.T) apiFixture {
	t.Helper()
	root := t.TempDir()
	depsFile(t, root, "build.gradle", `
dependencies {
  implementation "com.acme:rich:1.0"
  implementation "com.acme:bare:1.0"
  implementation "com.acme:missing:1.0"
}
`)
	m2 := filepath.Join(root, "fake-m2")
	zipJar(t, filepath.Join(m2, "com/acme/rich/1.0/rich-1.0-sources.jar"),
		map[string][]byte{"com/acme/Bar.java": []byte(barSources)})
	classData := buildTestClass(t) // com.acme.Foo from the class-reader fixture
	zipJar(t, filepath.Join(m2, "com/acme/bare/1.0/bare-1.0.jar"),
		map[string][]byte{"com/acme/Foo.class": classData})
	return apiFixture{root: root, m2: m2}
}

func (f apiFixture) opts() ApiOptions {
	return ApiOptions{Root: f.root, M2Dir: f.m2, GradleCache: filepath.Join(f.root, "no-gradle"), SkipUsage: true}
}

func TestApiSourcesTier(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep, o.Symbol = "rich", "Bar"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Surface != ir.ApiSources {
		t.Fatalf("surface = %q, want sources (%s)", reply.Surface, reply.Note)
	}
	if reply.Version != "1.0" || reply.Group != "com.acme" || reply.Artifact != "rich" {
		t.Errorf("coordinate = %s:%s@%s", reply.Group, reply.Artifact, reply.Version)
	}
	if reply.Tier != ir.DepDeclared {
		t.Errorf("tier = %q, want declared", reply.Tier)
	}
	if len(reply.Types) != 1 {
		t.Fatalf("types = %+v", reply.Types)
	}
	ty := reply.Types[0]
	if ty.Name != "com.acme.Bar" || ty.Kind != "class" {
		t.Errorf("type = %s %s", ty.Kind, ty.Name)
	}
	if ty.Doc != "Does rich things." {
		t.Errorf("type doc = %q, want the javadoc first line", ty.Doc)
	}
	var compute, helper, hidden, version bool
	for _, m := range ty.Members {
		switch m.Name {
		case "compute":
			compute = true
			if !strings.Contains(m.Signature, "compute(int)") {
				t.Errorf("compute signature = %q", m.Signature)
			}
			if m.Doc != "Computes a value." {
				t.Errorf("compute doc = %q", m.Doc)
			}
		case "privateHelper":
			helper = true
		case "hidden":
			hidden = true
		case "VERSION":
			version = true
		}
	}
	if !compute {
		t.Errorf("public compute missing from %+v", ty.Members)
	}
	if !version {
		t.Errorf("public static final VERSION missing from %+v", ty.Members)
	}
	if helper || hidden {
		t.Error("private members leaked into the public surface")
	}
}

func TestApiClassTier(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep, o.Symbol = "bare", "Foo"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Surface != ir.ApiClasses {
		t.Fatalf("surface = %q, want class (%s)", reply.Surface, reply.Note)
	}
	if len(reply.Types) != 1 || reply.Types[0].Name != "com.acme.Foo" {
		t.Fatalf("types = %+v", reply.Types)
	}
	if reply.Types[0].Kind != "class" {
		t.Errorf("kind = %q", reply.Types[0].Kind)
	}
	got := map[string]bool{}
	for _, m := range reply.Types[0].Members {
		got[m.Signature] = true
	}
	for _, want := range []string{"String bar(int, String)", "static void main(String[]) (varargs)"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, reply.Types[0].Members)
		}
	}
	for _, m := range reply.Types[0].Members {
		if m.Name == "secret" {
			t.Error("private method leaked")
		}
	}
}

// TestApiGradleCacheLayout proves the class tier reaches the gradle files-2.1
// layout too (one hash directory deeper), not just the Maven shape.
func TestApiGradleCacheLayout(t *testing.T) {
	f := newAPIFixture(t)
	gradle := filepath.Join(f.root, "modules-2")
	classData := buildTestClass(t)
	zipJar(t, filepath.Join(gradle, "files-2.1/com.acme/bare/1.0/hash123/bare-1.0.jar"),
		map[string][]byte{"com/acme/Foo.class": classData})
	// Remove the Maven copy so only the gradle layout can answer.
	os.RemoveAll(filepath.Join(f.m2, "com/acme/bare"))

	o := f.opts()
	o.GradleCache = gradle
	o.Dep, o.Symbol = "bare", "Foo"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Surface != ir.ApiClasses || len(reply.Types) != 1 {
		t.Fatalf("surface=%q types=%+v note=%q", reply.Surface, reply.Types, reply.Note)
	}
}

func TestApiHeadline(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep = "rich"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Surface != ir.ApiSources {
		t.Fatalf("surface = %q (%s)", reply.Surface, reply.Note)
	}
	if len(reply.Types) != 1 || reply.Types[0].Name != "com.acme.Bar" {
		t.Fatalf("headline = %+v", reply.Types)
	}
	if len(reply.Types[0].Members) != 0 {
		t.Error("a bare `api` must not pay to read members across the jar")
	}
}

func TestApiAmbiguousLists(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep = "com.acme:" // matches rich, bare, and missing
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Ambiguous) != 3 {
		t.Fatalf("ambiguous = %v, want three coordinates listed", reply.Ambiguous)
	}
	if reply.Surface != ir.ApiNone || len(reply.Types) != 0 {
		t.Error("ambiguity must stop the surface read, not pick a winner")
	}
	if !strings.Contains(reply.Note, "narrow") {
		t.Errorf("note = %q, want guidance to narrow", reply.Note)
	}
}

func TestApiNoJarStillResolves(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep, o.Symbol = "missing", "Ghost"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Version != "1.0" || reply.Surface != ir.ApiNone {
		t.Fatalf("coordinate %+v surface %q", reply, reply.Surface)
	}
	if !strings.Contains(reply.Note, "mvn dependency:sources") {
		t.Errorf("note = %q, want the fetch-sources guidance", reply.Note)
	}
}

func TestApiTypeNotFoundSuggests(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep, o.Symbol = "rich", "Barr"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Note, "no type Barr") || !strings.Contains(reply.Note, "Bar") {
		t.Errorf("note = %q, want closest-name guidance", reply.Note)
	}
}

func TestApiMemberFilter(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts()
	o.Dep, o.Symbol = "bare", "Foo.bar"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Types) != 1 {
		t.Fatalf("types = %+v", reply.Types)
	}
	if len(reply.Types[0].Members) != 1 || reply.Types[0].Members[0].Name != "bar" {
		t.Errorf("members = %+v, want only bar", reply.Types[0].Members)
	}
}

// TestApiUsageTier: the workspace half. A tiny app that calls Bar.compute must
// come back as call sites, with the same RefResult shape `references` returns.
func TestApiUsageTier(t *testing.T) {
	f := newAPIFixture(t)
	depsFile(t, f.root, "src/main/java/com/example/App.java", `
package com.example;

import com.acme.Bar;

public class App {
    public String go() {
        Bar b = new Bar();
        return b.compute(1);
    }
}
`)
	eng, err := Open(f.root, Options{Caps: CapsSymbols, Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}
	o := f.opts()
	o.SkipUsage = false
	o.Eng = eng
	o.Dep, o.Symbol = "rich", "Bar.compute"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.UsageTotal == 0 || len(reply.Usage) == 0 {
		t.Fatalf("usage = %+v, want the App.java call site", reply.Usage)
	}
	var found bool
	for _, u := range reply.Usage {
		if strings.Contains(u.Path, "App.java") {
			found = true
		}
	}
	if !found {
		t.Errorf("usage = %+v, want an App.java row", reply.Usage)
	}
}

// TestApiSourcesOnlyNeedsNoEngine is the no-state-law shape of the CLI: the
// artifact half works with Eng == nil (the CLI skips the Open entirely).
func TestApiSourcesOnlyNeedsNoEngine(t *testing.T) {
	f := newAPIFixture(t)
	o := f.opts() // SkipUsage true, Eng nil
	o.Dep, o.Symbol = "rich", "Bar"
	reply, err := Api(o)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Surface != ir.ApiSources {
		t.Fatalf("no engine, no answer: %+v", reply)
	}
}

func TestSplitTypeMember(t *testing.T) {
	cases := []struct{ in, typ, member string }{
		{"ObjectMapper.readValue", "ObjectMapper", "readValue"},
		{"com.acme.Foo", "com.acme.Foo", ""},
		{"Foo", "Foo", ""},
		{"com.acme.Foo.bar", "com.acme.Foo", "bar"},
		{"Foo$Bar", "Foo$Bar", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		typ, member := splitTypeMember(c.in)
		if typ != c.typ || member != c.member {
			t.Errorf("splitTypeMember(%q) = (%q,%q), want (%q,%q)", c.in, typ, member, c.typ, c.member)
		}
	}
}
