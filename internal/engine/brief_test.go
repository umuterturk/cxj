package engine

import (
	"testing"

	"example.com/cx/internal/ir"
)

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
	want := []string{"com.example.app", "com.example.app.owner", "com.example.app.vet"}
	if len(got) != len(want) {
		t.Fatalf("got %v want names %v", got, want)
	}
	for i, name := range want {
		if got[i].Name != name || got[i].Scope != "main" {
			t.Fatalf("got[%d]=%+v want %q main", i, got[i], name)
		}
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
	if len(test) != 1 || test[0].Name != "com.example.app.owner" || test[0].Scope != "test" || test[0].Types != 4 {
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
	if len(got) != 2 || got[0].Name != "com.acme" || got[1].Name != "com.acme.api" {
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
