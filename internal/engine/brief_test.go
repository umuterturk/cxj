package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/ir"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func openBrief(t *testing.T, files map[string]string) ir.BriefReply {
	t.Helper()
	eng, err := Open(writeTree(t, files), Options{Caps: CapsBrief})
	if err != nil {
		t.Fatal(err)
	}
	return eng.Brief()
}

func TestSelectBriefPackages_bootRootPlusDepth1(t *testing.T) {
	pkgTypes := map[string]int{
		"com.example.app":           1,
		"com.example.app.owner":     5,
		"com.example.app.vet":       3,
		"com.example.app.owner.dto": 2,
		"com.other.mod":             4,
	}
	roots := briefRoots([]ir.Entrypoint{{
		Kind: "boot", Symbol: "com.example.app.PetClinicApplication",
	}}, pkgTypes)
	if len(roots) != 1 || roots[0] != "com.example.app" {
		t.Fatalf("roots=%v", roots)
	}
	got := selectBriefPackages(pkgTypes, roots, "main")
	want := []ir.BriefPackage{
		{Name: "com.example.app", Scope: "main", Types: 1},
		{Name: "com.example.app.owner", Scope: "main", Types: 7}, // owner + owner.dto
		{Name: "com.example.app.vet", Scope: "main", Types: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSelectBriefPackages_synthesizeEmptyIntermediates(t *testing.T) {
	pkgTypes := map[string]int{
		"com.example.app":                           1,
		"com.example.app.application.usecase":       4,
		"com.example.app.infrastructure.adapter":    6,
		"com.example.app.infrastructure.adapter.db": 2,
	}
	got := selectBriefPackages(pkgTypes, []string{"com.example.app"}, "main")
	want := []ir.BriefPackage{
		{Name: "com.example.app", Scope: "main", Types: 1},
		{Name: "com.example.app.application", Scope: "main", Types: 4},
		{Name: "com.example.app.infrastructure", Scope: "main", Types: 8},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSelectBriefPackages_testScope(t *testing.T) {
	mainTypes := map[string]int{"com.example.app": 1, "com.example.app.owner": 2}
	testTypes := map[string]int{"com.example.app.owner": 4, "com.example.app.owner.dto": 1}
	roots := []string{"com.example.app"}
	main := selectBriefPackages(mainTypes, roots, "main")
	test := selectBriefPackages(testTypes, roots, "test")
	if len(main) != 2 || main[0].Scope != "main" {
		t.Fatalf("main=%v", main)
	}
	if len(test) != 1 || test[0].Name != "com.example.app.owner" || test[0].Scope != "test" || test[0].Types != 5 {
		t.Fatalf("test=%v", test)
	}
}

func TestBriefRoots_fallbackShallow(t *testing.T) {
	pkgTypes := map[string]int{
		"com.acme":        1,
		"com.acme.api":    2,
		"com.acme.api.v1": 3,
	}
	roots := briefRoots(nil, pkgTypes)
	if len(roots) != 1 || roots[0] != "com.acme" {
		t.Fatalf("roots=%v", roots)
	}
	got := selectBriefPackages(pkgTypes, roots, "main")
	if len(got) != 2 || got[0].Name != "com.acme" || got[0].Types != 1 ||
		got[1].Name != "com.acme.api" || got[1].Types != 5 {
		t.Fatalf("got=%v", got)
	}
}

func TestPackageOfSymbol(t *testing.T) {
	if g := packageOfSymbol("com.example.App"); g != "com.example" {
		t.Fatalf("got %q", g)
	}
	if g := packageOfSymbol("App"); g != "" {
		t.Fatalf("got %q", g)
	}
}

func TestIsTestPath(t *testing.T) {
	if !isTestPath("src/test/java/Foo.java") {
		t.Fatal("expected test")
	}
	if !isTestPath("module/src/test/java/Foo.java") {
		t.Fatal("expected test")
	}
	if isTestPath("src/main/java/Foo.java") {
		t.Fatal("expected main")
	}
}

func TestBrief_kindSpringOnlyWithBoot(t *testing.T) {
	plain := openBrief(t, map[string]string{
		"src/main/java/com/example/App.java": `
package com.example;
class App {
  public static void main(String[] args) {}
}
`,
	})
	if plain.Kind != "java" {
		t.Fatalf("plain kind=%q", plain.Kind)
	}
	boot := openBrief(t, map[string]string{
		"src/main/java/com/example/app/PetClinicApplication.java": `
package com.example.app;
@SpringBootApplication
class PetClinicApplication {
  public static void main(String[] args) {}
}
`,
	})
	if boot.Kind != "spring" {
		t.Fatalf("boot kind=%q", boot.Kind)
	}
}

func TestBrief_versionsFromPomAndGradle(t *testing.T) {
	pom := openBrief(t, map[string]string{
		"pom.xml": `
<project>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
    <version>3.2.0</version>
  </parent>
  <properties><java.version>17</java.version></properties>
</project>
`,
		"src/main/java/com/example/app/App.java": `
package com.example.app;
@SpringBootApplication
class App {}
`,
	})
	if pom.Java != "17" || pom.SpringBoot != "3.2.0" {
		t.Fatalf("pom versions java=%q boot=%q", pom.Java, pom.SpringBoot)
	}
	gradle := openBrief(t, map[string]string{
		"build.gradle": `
plugins { id 'org.springframework.boot' version '3.1.5' }
java { toolchain { languageVersion = JavaLanguageVersion.of(21) } }
`,
		"src/main/java/com/example/app/App.java": `
package com.example.app;
@SpringBootApplication
class App {}
`,
	})
	if gradle.Java != "21" || gradle.SpringBoot != "3.1.5" {
		t.Fatalf("gradle versions java=%q boot=%q", gradle.Java, gradle.SpringBoot)
	}
	none := openBrief(t, map[string]string{
		"src/main/java/com/example/App.java": "package com.example; class App {}",
	})
	if none.Java != "" || none.SpringBoot != "" {
		t.Fatalf("expected empty versions, got java=%q boot=%q", none.Java, none.SpringBoot)
	}
}

func TestBrief_sizeMainOnly(t *testing.T) {
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": `
package com.example.app;
class App {}
interface Marker {}
`,
		"src/main/java/com/example/app/owner/Owner.java": `
package com.example.app.owner;
class Owner {}
`,
		"src/test/java/com/example/app/AppTest.java": `
package com.example.app;
class AppTest {}
`,
	})
	if got.MainFiles != 2 {
		t.Fatalf("main_files=%d", got.MainFiles)
	}
	if got.Types != 3 {
		t.Fatalf("types=%d", got.Types)
	}
	if got.TestClasses != 1 {
		t.Fatalf("test_classes=%d", got.TestClasses)
	}
}

func TestBrief_packagesMainOnly(t *testing.T) {
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": `
package com.example.app;
@SpringBootApplication
class App {}
`,
		"src/main/java/com/example/app/owner/Owner.java": `
package com.example.app.owner;
class Owner {}
`,
		"src/main/java/com/example/app/owner/dto/OwnerDto.java": `
package com.example.app.owner.dto;
class OwnerDto {}
`,
		"src/test/java/com/example/app/owner/OwnerTest.java": `
package com.example.app.owner;
class OwnerTest {}
`,
	})
	var names []string
	for _, p := range got.Packages {
		if p.Scope == "test" {
			t.Fatalf("test package leaked: %+v", p)
		}
		names = append(names, p.Name)
	}
	want := []string{"com.example.app", "com.example.app.owner"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("packages=%v want %v", names, want)
	}
}

func TestBrief_httpMethodAndRoute(t *testing.T) {
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/owner/OwnerController.java": `
package com.example.app.owner;
@RestController
@RequestMapping("/owners")
class OwnerController {
  @GetMapping
  Object list() { return null; }
  @PostMapping
  Object create() { return null; }
}
`,
	})
	want := []ir.BriefHTTP{{Method: "GET", Route: "/owners"}, {Method: "POST", Route: "/owners"}}
	if !reflect.DeepEqual(got.HTTP, want) {
		t.Fatalf("http=%+v want %+v", got.HTTP, want)
	}
}

func TestBrief_ingressOtherOmitEmpty(t *testing.T) {
	empty := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": `
package com.example.app;
@SpringBootApplication
class App {}
`,
	})
	if len(empty.Ingress) != 0 {
		t.Fatalf("expected no ingress, got %+v", empty.Ingress)
	}
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/DemoRunner.java": `
package com.example.app;
class DemoRunner implements CommandLineRunner {}
`,
		"src/main/java/com/example/app/Jobs.java": `
package com.example.app;
class Jobs {
  @Scheduled
  void cleanup() {}
}
`,
		"src/main/java/com/example/app/Mail.java": `
package com.example.app;
class Mail {
  @EventListener
  void onReady() {}
}
`,
	})
	want := []ir.BriefIngress{
		{Kind: "event", Symbol: "Mail.onReady"},
		{Kind: "runner", Symbol: "DemoRunner"},
		{Kind: "scheduled", Symbol: "Jobs.cleanup"},
	}
	if !reflect.DeepEqual(got.Ingress, want) {
		t.Fatalf("ingress=%+v want %+v", got.Ingress, want)
	}
	for _, in := range got.Ingress {
		if in.Kind == "boot" || in.Kind == "advice" || in.Kind == "main" {
			t.Fatalf("boot/advice/main leaked: %+v", in)
		}
	}
}

func TestBrief_outboundHTTP(t *testing.T) {
	none := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": "package com.example.app; class App {}",
	})
	if len(none.OutboundHTTP) != 0 {
		t.Fatalf("expected no out_http, got %v", none.OutboundHTTP)
	}
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/PaymentsClient.java": `
package com.example.app;
@FeignClient(name = "payments")
interface PaymentsClient {}
`,
		"src/main/java/com/example/app/BillingClient.java": `
package com.example.app;
@FeignClient("billing")
interface BillingClient {}
`,
		"src/main/java/com/example/app/Gateway.java": `
package com.example.app;
class Gateway {
  private final RestTemplate restTemplate;
  Gateway(RestTemplate restTemplate) { this.restTemplate = restTemplate; }
}
`,
		"src/main/java/com/example/app/ZipRestAdapter.java": `
package com.example.app;
class ZipRestAdapter {
  private final RestClient restClient;
}
`,
	})
	want := []string{"Gateway", "ZipRestAdapter", "billing", "payments"}
	if !reflect.DeepEqual(got.OutboundHTTP, want) {
		t.Fatalf("out_http=%v want %v", got.OutboundHTTP, want)
	}
}

func TestBrief_outboundDB(t *testing.T) {
	none := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": "package com.example.app; class App {}",
	})
	if len(none.OutboundDB) != 0 {
		t.Fatalf("expected no out_db, got %v", none.OutboundDB)
	}
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/OwnerRepository.java": `
package com.example.app;
interface OwnerRepository extends JpaRepository<Owner, Long> {}
`,
		"src/main/java/com/example/app/VetRepository.java": `
package com.example.app;
interface VetRepository extends CrudRepository<Vet, Integer> {}
`,
		"src/main/java/com/example/app/OwnerRepo2.java": `
package com.example.app;
interface OwnerRepo2 extends JpaRepository<Owner, Long> {}
`,
	})
	want := []string{"Owner", "Vet"}
	if !reflect.DeepEqual(got.OutboundDB, want) {
		t.Fatalf("out_db=%v want %v", got.OutboundDB, want)
	}
}

func TestBrief_textOmitsVerboseEntrypoints(t *testing.T) {
	got := openBrief(t, map[string]string{
		"src/main/java/com/example/app/App.java": `
package com.example.app;
@SpringBootApplication
class App {}
`,
		"src/main/java/com/example/app/OwnerController.java": `
package com.example.app;
@RestController
class OwnerController {
  @GetMapping("/owners")
  Object list() { return null; }
}
`,
	})
	text := ir.FormatBrief(got)
	if strings.Contains(text, ".java:") || strings.Contains(text, "boot ") {
		t.Fatalf("verbose dump leaked:\n%s", text)
	}
}
