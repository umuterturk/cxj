package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/cx/internal/ir"
)

// B1 fixtures are written into t.TempDir rather than a checked-in testdata tree:
// the cache cross-check needs real directories to list, the flags under test are
// the cache overrides, and each test wants its own hermetic world.

func depsFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func depsJar(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(dir)
	if err := os.WriteFile(filepath.Join(dir, base+".jar"), []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findDep(t *testing.T, reply ir.DepsReply, coord string) ir.Dep {
	t.Helper()
	for _, d := range reply.Deps {
		if d.Group+":"+d.Artifact == coord {
			return d
		}
	}
	t.Fatalf("no row for %s; rows:\n%s", coord, depsDump(reply))
	return ir.Dep{}
}

func depsDump(reply ir.DepsReply) string {
	var b strings.Builder
	for _, d := range reply.Deps {
		b.WriteString(d.Group + ":" + d.Artifact + " " + d.Version + " " + d.Tier + "\n")
	}
	return b.String()
}

const pomFixture = `
<project>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.2.0</version>
  </parent>
  <properties>
    <java.version>17</java.version>
    <widget.version>1.2.3</widget.version>
  </properties>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.acme</groupId>
        <artifactId>widget</artifactId>
        <version>${widget.version}</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>com.acme</groupId>
      <artifactId>widget</artifactId>
    </dependency>
    <dependency>
      <groupId>com.acme</groupId>
      <artifactId>gadget</artifactId>
      <version>9.9</version>
    </dependency>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
    </dependency>
    <dependency>
      <groupId>com.acme</groupId>
      <artifactId>mystery</artifactId>
      <version>${released.only.in.ci}</version>
    </dependency>
  </dependencies>
</project>
`

func TestDepsPomTiersAndSubstitution(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "pom.xml", pomFixture)
	m2 := filepath.Join(root, "fake-m2")
	depsJar(t, filepath.Join(m2, "com", "acme", "gadget", "9.9"))
	depsJar(t, filepath.Join(m2, "org", "springframework", "boot", "spring-boot-starter-web", "3.2.0"))

	reply, err := Deps(DepsOptions{Root: root, M2Dir: m2, GradleCache: filepath.Join(root, "no-such-gradle")})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Java != "17" || reply.Boot != "3.2.0" {
		t.Errorf("headline java=%q boot=%q, want 17 / 3.2.0", reply.Java, reply.Boot)
	}

	// Property substitution through dependencyManagement, and the declared
	// dependency inheriting the managed version.
	w := findDep(t, reply, "com.acme:widget")
	if w.Tier != ir.DepManaged || w.Version != "1.2.3" {
		t.Errorf("widget = %s %s, want managed 1.2.3 (property substituted)", w.Version, w.Tier)
	}
	// A literal version at the dependency itself is `declared`; the cache has it
	// and says so without changing the tier.
	g := findDep(t, reply, "com.acme:gadget")
	if g.Tier != ir.DepDeclared || g.Version != "9.9" || !strings.Contains(g.Note, "cached") {
		t.Errorf("gadget = %+v, want declared 9.9 with a cached note", g)
	}
	// No version anywhere on disk, but the cache holds exactly one: cached,
	// with the version it supplied.
	web := findDep(t, reply, "org.springframework.boot:spring-boot-starter-web")
	if web.Tier != ir.DepCached || web.Version != "3.2.0" {
		t.Errorf("starter-web = %+v, want cached 3.2.0 from the only cached version", web)
	}
	// An unresolvable ${placeholder} is unknown, not a blank or a guess.
	m := findDep(t, reply, "com.acme:mystery")
	if m.Tier != ir.DepUnknown || m.Version != "" {
		t.Errorf("mystery = %+v, want unknown with no version", m)
	}
	// The parent BOM itself is a row: it is the reason most versions are unknown.
	p := findDep(t, reply, "org.springframework.boot:spring-boot-starter-parent")
	if p.Tier != ir.DepManaged || p.Version != "3.2.0" || p.Note != "parent" {
		t.Errorf("parent = %+v, want managed 3.2.0 parent", p)
	}
	if !strings.HasPrefix(web.Source, "pom.xml:") {
		t.Errorf("source %q, want pom.xml:<line>", web.Source)
	}
	// Headline frameworks sort first.
	if len(reply.Deps) > 1 && !strings.HasPrefix(reply.Deps[0].Group, "org.springframework") {
		t.Errorf("first row is %s, want a spring framework leading the table", reply.Deps[0].Group)
	}
}

func TestDepsCacheAmbiguousVersions(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "pom.xml", `
<project>
  <dependencies>
    <dependency>
      <groupId>com.acme</groupId>
      <artifactId>two</artifactId>
    </dependency>
  </dependencies>
</project>
`)
	m2 := filepath.Join(root, "fake-m2")
	depsJar(t, filepath.Join(m2, "com", "acme", "two", "1.0"))
	depsJar(t, filepath.Join(m2, "com", "acme", "two", "2.0"))

	reply, err := Deps(DepsOptions{Root: root, M2Dir: m2})
	if err != nil {
		t.Fatal(err)
	}
	d := findDep(t, reply, "com.acme:two")
	if d.Version != "" || !strings.Contains(d.Note, "ambiguous") {
		t.Errorf("two = %+v, want ambiguous across the cached versions, not a pick", d)
	}
}

func TestDepsGradleCatalog(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "gradle/libs.versions.toml", `
[versions]
acme = "4.1.0"

[libraries]
acme-sdk = { module = "com.acme:sdk", version.ref = "acme" }
`)
	depsFile(t, root, "build.gradle", `
dependencies {
  // commented "com.noise:off:0.0" must not become a row
  implementation platform("org.springframework.boot:spring-boot-dependencies:3.2.0")
  implementation "com.acme:direct:1.0"
  implementation libs.acme.sdk
  implementation "com.acme:bomless"
}
`)

	reply, err := Deps(DepsOptions{Root: root, M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range reply.Deps {
		if d.Group == "com.noise" {
			t.Fatalf("comment leaked a row: %+v", d)
		}
	}
	bom := findDep(t, reply, "org.springframework.boot:spring-boot-dependencies")
	if bom.Tier != ir.DepManaged || !strings.Contains(bom.Note, "platform") {
		t.Errorf("platform = %+v, want managed with a platform note", bom)
	}
	direct := findDep(t, reply, "com.acme:direct")
	if direct.Tier != ir.DepDeclared || direct.Version != "1.0" {
		t.Errorf("direct = %+v, want declared 1.0", direct)
	}
	sdk := findDep(t, reply, "com.acme:sdk")
	if sdk.Tier != ir.DepManaged || sdk.Version != "4.1.0" {
		t.Errorf("sdk = %+v, want managed 4.1.0 through the catalog", sdk)
	}
	bomless := findDep(t, reply, "com.acme:bomless")
	if bomless.Tier != ir.DepUnknown || !strings.Contains(bomless.Note, "platform") {
		t.Errorf("bomless = %+v, want unknown with a platform note", bomless)
	}
}

func TestDepsGradleGradleCacheLayout(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "build.gradle.kts", `
dependencies {
  implementation("com.example:lib")
}
`)
	gradle := filepath.Join(root, "modules-2")
	depsJar(t, filepath.Join(gradle, "files-2.1", "com.example", "lib", "1.4.2", "abc123"))

	reply, err := Deps(DepsOptions{Root: root, M2Dir: filepath.Join(root, "none"), GradleCache: gradle})
	if err != nil {
		t.Fatal(err)
	}
	d := findDep(t, reply, "com.example:lib")
	if d.Tier != ir.DepCached || d.Version != "1.4.2" {
		t.Errorf("lib = %+v, want cached 1.4.2 from the gradle layout", d)
	}
}

func TestDepsGradlePropertiesVersion(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "gradle.properties", "springCloudVersion=2025.1.0\n")
	depsFile(t, root, "build.gradle", `
dependencies {
  implementation platform("org.springframework.cloud:spring-cloud-dependencies:$springCloudVersion")
  implementation "com.acme:seen:${lookedUpVersion}"
  implementation "com.acme:hidden:$notWrittenDown"
}
`)
	reply, err := Deps(DepsOptions{Root: root, M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	bom := findDep(t, reply, "org.springframework.cloud:spring-cloud-dependencies")
	if bom.Version != "2025.1.0" {
		t.Errorf("BOM version = %q, want 2025.1.0 substituted from gradle.properties", bom.Version)
	}
	seen := findDep(t, reply, "com.acme:seen")
	if seen.Tier != ir.DepUnknown || !strings.Contains(seen.Note, "unresolved") {
		t.Errorf("seen = %+v, want unknown with an unresolved-property note", seen)
	}
	hidden := findDep(t, reply, "com.acme:hidden")
	if hidden.Tier != ir.DepUnknown || hidden.Version != "" {
		t.Errorf("hidden = %+v, want unknown with no invented version", hidden)
	}
}

func TestDepsLockfile(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "build.gradle", `
dependencies {
  implementation "com.acme:direct:1.0"
}
`)
	depsFile(t, root, "dependencies.lock", `{
  "dependencies": {
    "compileClasspath": [
      { "group": "com.acme", "name": "direct", "version": "1.1.0" },
      { "group": "com.other", "name": "transitive", "version": "2.0" },
      { "group": "com.acme", "name": "dynamic", "version": "5.+" }
    ]
  }
}`)

	reply, err := Deps(DepsOptions{Root: root, M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	// The lock is what the last build actually resolved, so it outranks the
	// build file — while saying what the build file claimed.
	d := findDep(t, reply, "com.acme:direct")
	if d.Tier != ir.DepResolved || d.Version != "1.1.0" || !strings.Contains(d.Note, "1.0") {
		t.Errorf("direct = %+v, want resolved 1.1.0 noting the 1.0 the build file says", d)
	}

	// A filtered run reaches the lockfile for transitives no build file declares,
	// and a dynamic pin ("5.+") is honestly not a resolved version.
	filtered, err := Deps(DepsOptions{Root: root, Filter: "com.acme", M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	dyn := findDep(t, filtered, "com.acme:dynamic")
	if dyn.Tier != ir.DepUnknown || !strings.Contains(dyn.Note, "dynamic") {
		t.Errorf("dynamic = %+v, want unknown with a dynamic-pin note", dyn)
	}
	trReply, err := Deps(DepsOptions{Root: root, Filter: "transitive", M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	tr := findDep(t, trReply, "com.other:transitive")
	if tr.Tier != ir.DepResolved || tr.Version != "2.0" {
		t.Errorf("transitive = %+v, want resolved 2.0 from the lockfile", tr)
	}
}

func TestDepsFilters(t *testing.T) {
	root := t.TempDir()
	depsFile(t, root, "pom.xml", `
<project>
  <dependencies>
    <dependency><groupId>com.acme</groupId><artifactId>one</artifactId><version>1</version></dependency>
    <dependency><groupId>org.other</groupId><artifactId>two</artifactId><version>2</version></dependency>
  </dependencies>
</project>
`)
	all, err := Deps(DepsOptions{Root: root, M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	if all.Count != 2 {
		t.Fatalf("count %d, want 2:\n%s", all.Count, depsDump(all))
	}
	// Substring filters match on group:artifact, case-insensitively.
	filtered, err := Deps(DepsOptions{Root: root, Filter: "ACME:one", M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Count != 1 || filtered.Deps[0].Artifact != "one" {
		t.Fatalf("filter result:\n%s", depsDump(filtered))
	}
	// Tier filters are exact rungs of the ladder.
	unknownOnly, err := Deps(DepsOptions{Root: root, Tier: "declared", M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	if unknownOnly.Count != 2 {
		t.Fatalf("declared count %d, want 2:\n%s", unknownOnly.Count, depsDump(unknownOnly))
	}
	none, err := Deps(DepsOptions{Root: root, Tier: "unknown", M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	if none.Count != 0 {
		t.Fatalf("unknown count %d, want 0", none.Count)
	}
	// The cap truncates Shown, never Count; "+N more" is a printer concern.
	capped, err := Deps(DepsOptions{Root: root, Limit: 1, M2Dir: filepath.Join(root, "none")})
	if err != nil {
		t.Fatal(err)
	}
	if capped.Shown != 1 || capped.Count != 2 {
		t.Errorf("shown=%d count=%d, want 1 and 2", capped.Shown, capped.Count)
	}
}

// TestDepsNoBuildFiles pins the difference between "no dependencies" and
// "this is not a JVM repo", which the reply's Files count carries.
func TestDepsNoBuildFiles(t *testing.T) {
	reply, err := Deps(DepsOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Files != 0 || reply.Count != 0 {
		t.Errorf("files=%d count=%d, want 0 and 0", reply.Files, reply.Count)
	}
}

func TestCatalogKeyCanonicalization(t *testing.T) {
	cases := map[string]string{
		"spring-cloud-gateway": "spring.cloud.gateway",
		"springCloud.gateway":  "spring.cloud.gateway",
		"acme.sdk":             "acme.sdk",
		"acme_sdk2":            "acme.sdk2",
		"SDK":                  "sdk",
	}
	for in, want := range cases {
		if got := canonCatalogKey(in); got != want {
			t.Errorf("canonCatalogKey(%q) = %q, want %q", in, got, want)
		}
	}
}
