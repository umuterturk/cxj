// Command gen materializes the deterministic benchmark fixture so scripts and CI
// can time the real binary against the same workspace the Go benchmarks use.
//
//	go run ./internal/benchfixture/gen -out /tmp/cxj-bench-fixture
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/umuterturk/cxj/internal/benchfixture"
)

func main() {
	out := flag.String("out", "", "directory to write the fixture into (required)")
	modules := flag.Int("modules", 0, "modules (0 = fixture default)")
	types := flag.Int("types", 0, "main type families per module (0 = fixture default)")
	tests := flag.Int("tests", -1, "test classes per module (-1 = fixture default)")
	quiet := flag.Bool("quiet", false, "print only the output directory")
	flag.Parse()

	if *out == "" {
		fmt.Fprintln(os.Stderr, "gen: -out is required")
		os.Exit(2)
	}
	st, err := benchfixture.Write(*out, benchfixture.Options{
		Modules:            *modules,
		TypesPerModule:     *types,
		TestTypesPerModule: *tests,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if *quiet {
		fmt.Println(*out)
		return
	}
	fmt.Printf("fixture=%s files=%d java=%d main=%d test=%d bytes=%d\n",
		*out, st.Files, st.JavaFiles, st.MainJavaFiles, st.TestJavaFiles, st.Bytes)
}
