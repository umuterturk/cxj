package acceptance

import (
	"os"
	"testing"
)

func TestP2TypoIsFuzzyAndExactRanksFirst(t *testing.T) {
	root := t.TempDir()
	put(t, root, "SettlementPipeline.java", `class SettlementPipeline { void dispatchTransfer() {} }
`)
	e := start(t, root, false)
	exact := e.search(t, "SettlementPipeline.dispatchTransfer", "lexical")
	if len(exact) == 0 || exact[0].Match != "exact" {
		t.Fatalf("exact symbol must rank first: %#v", exact)
	}
	typo := e.search(t, "SettlmentPipeline.dispatchTransfer", "lexical")
	if !containsSymbol(typo, "dispatchTransfer") {
		t.Fatalf("misspelled class not found: %#v", typo)
	}
	if typo[0].Match == "exact" {
		t.Fatalf("typo cannot be called exact: %#v", typo)
	}
}
func TestP2PlainTextIsSearchableWithoutJavaSymbols(t *testing.T) {
	root := t.TempDir()
	put(t, root, "docs/runbook.md", "Deploy runbook\nrotate signing certificate before release\n")
	put(t, root, "db/migration.sql", "SELECT * FROM ledger WHERE status = 'quarantined';\n")
	e := start(t, root, false)
	r := e.search(t, "signing certificate", "lexical")
	requirePath(t, r, "runbook.md")
	for _, x := range r {
		if x.Kind != "text" {
			t.Fatalf("plain text must stay text: %+v", x)
		}
	}
	requirePath(t, e.search(t, "quarantined", "lexical"), "migration.sql")
}
func TestP2ExcludeFlagAndCxIgnoreApplyBeforeIndexing(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/RealInvoice.java", "class RealInvoice {}\n")
	put(t, root, "generated/DecoyInvoice.java", "class DecoyInvoice {}\n")
	put(t, root, "private/notes.txt", "secret marker incomparable_lorem_13\n")
	put(t, root, ".cxignore", "# personal files\nprivate/**\n")
	e := start(t, root, false, "generated/**")
	requirePath(t, e.search(t, "RealInvoice", "lexical"), "RealInvoice.java")
	requireNoPath(t, e.search(t, "DecoyInvoice", "lexical"), "DecoyInvoice.java")
	requireNoPath(t, e.search(t, "incomparable_lorem_13", "lexical"), "notes.txt")
}
func TestP2BinaryAndLargeFilesAreSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/blob.bin", []byte{0, 1, 2, 0, 255}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/huge.txt", []byte(string(make([]byte, 1024*1024+1))), 0644); err != nil {
		t.Fatal(err)
	}
	e := start(t, root, false)
	requireNoPath(t, e.search(t, "blob", "lexical"), "blob.bin")
	if e.status(t).IndexedFiles != 0 {
		t.Fatalf("binary and oversized text must be excluded from index: %+v", e.status(t))
	}
}

func TestP2DefaultBuildOutputFoldersAreExcluded(t *testing.T) {
	root := t.TempDir()
	put(t, root, "target/generated-sources/FakeGenerated.java", "class FakeGenerated {}\n")
	put(t, root, "build/cache.txt", "do not index build-private-value\n")
	put(t, root, "src/Included.java", "class Included {}\n")
	e := start(t, root, false)
	requirePath(t, e.search(t, "Included", "lexical"), "Included.java")
	requireNoPath(t, e.search(t, "FakeGenerated", "lexical"), "FakeGenerated.java")
	requireNoPath(t, e.search(t, "build-private-value", "lexical"), "cache.txt")
}
