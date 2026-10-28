package benchfixture

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/cx/internal/engine"
	"example.com/cx/internal/scan"
	"example.com/cx/internal/spring"
)

func TestWriteDefaultScale(t *testing.T) {
	root := t.TempDir()
	st, err := Write(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.MainJavaFiles == 0 || st.TestJavaFiles == 0 {
		t.Fatalf("expected main and test java files, got %+v", st)
	}
	// The fixture is sized against payment-api (561 main / 849 total java). If
	// this drifts, the committed perf baseline no longer means what it says.
	if st.MainJavaFiles < 500 || st.MainJavaFiles > 620 {
		t.Errorf("MainJavaFiles=%d, want ~560 (perf baseline is calibrated to this)", st.MainJavaFiles)
	}
	if st.JavaFiles != st.MainJavaFiles+st.TestJavaFiles {
		t.Errorf("count mismatch: %+v", st)
	}
	for _, must := range []string{"pom.xml", "src/main/resources/application.yml"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(must))); err != nil {
			t.Errorf("missing %s: %v", must, err)
		}
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	sa, err := Write(a, Options{Modules: 2, TypesPerModule: 3, TestTypesPerModule: 2})
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Write(b, Options{Modules: 2, TypesPerModule: 3, TestTypesPerModule: 2})
	if err != nil {
		t.Fatal(err)
	}
	if sa.Bytes != sb.Bytes || sa.Files != sb.Files {
		t.Fatalf("not deterministic: %+v vs %+v", sa, sb)
	}
	probe := "src/main/java/com/example/bench/mod0/Widget0Controller.java"
	ha, err := os.ReadFile(filepath.Join(a, filepath.FromSlash(probe)))
	if err != nil {
		t.Fatal(err)
	}
	hb, err := os.ReadFile(filepath.Join(b, filepath.FromSlash(probe)))
	if err != nil {
		t.Fatal(err)
	}
	if string(ha) != string(hb) {
		t.Fatal("fixture files differ between runs")
	}
}

// TestWriteProducesAnIndexableWorkspace guards the benchmark against measuring a
// fixture the engine cannot actually resolve: routes, injections and cross-type
// calls must be present, and the main scope must drop the test classes.
func TestWriteProducesAnIndexableWorkspace(t *testing.T) {
	root := t.TempDir()
	if _, err := Write(root, Options{Modules: 2, TypesPerModule: 3, TestTypesPerModule: 2}); err != nil {
		t.Fatal(err)
	}
	eng, err := engine.Open(root, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	epsRes, ok := eng.Endpoints("", "").(map[string]any)
	if !ok {
		t.Fatal("unexpected Endpoints shape")
	}
	eps, ok := epsRes["results"].([]spring.EndpointResult)
	if !ok || len(eps) == 0 {
		t.Fatalf("fixture produced no endpoints: %#v", epsRes["results"])
	}
	trail := eng.Trail(EntrySymbol)
	if len(trail.Callees) == 0 {
		t.Fatalf("fixture entrypoint %s has no callees: %+v", EntrySymbol, trail)
	}
	callers := eng.Callers("com.example.bench.mod0.Widget0Service.load")
	if len(callers.Results) == 0 {
		t.Fatal("fixture produced no callers for service.load")
	}

	main, err := engine.Open(root, engine.Options{Scope: scan.ScopeMain})
	if err != nil {
		t.Fatal(err)
	}
	if !main.ScopeFilteredTests() {
		t.Fatal("ScopeMain should have dropped fixture test files")
	}
}
