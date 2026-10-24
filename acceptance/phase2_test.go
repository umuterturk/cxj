package acceptance

import (
	"os"
	"testing"
)

func TestP2TypoIsFuzzyAndExactRanksFirst(t *testing.T) {
	root := t.TempDir()
	put(t, root, "SettlementPipeline.java", `class SettlementPipeline { void dispatchTransfer() {} }
`)
	e := open(t, root)
	exact := e.search(t, "SettlementPipeline.dispatchTransfer")
	if len(exact) == 0 || exact[0].Match != "exact" {
		t.Fatalf("exact symbol must rank first: %#v", exact)
	}
	typo := e.search(t, "SettlmentPipeline.dispatchTransfer")
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
	e := open(t, root)
	r := e.search(t, "signing certificate")
	requirePath(t, r, "runbook.md")
	for _, x := range r {
		if x.Kind != "text" {
			t.Fatalf("plain text must stay text: %+v", x)
		}
	}
	requirePath(t, e.search(t, "quarantined"), "migration.sql")
}

func TestP2ExcludeFlagAndCxIgnoreApplyBeforeIndexing(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/RealInvoice.java", "class RealInvoice {}\n")
	put(t, root, "generated/DecoyInvoice.java", "class DecoyInvoice {}\n")
	put(t, root, "private/notes.txt", "secret marker incomparable_lorem_13\n")
	put(t, root, ".cxignore", "# personal files\nprivate/**\n")
	e := open(t, root, "generated/**")
	requirePath(t, e.search(t, "RealInvoice"), "RealInvoice.java")
	requireNoPath(t, e.search(t, "DecoyInvoice"), "DecoyInvoice.java")
	requireNoPath(t, e.search(t, "incomparable_lorem_13"), "notes.txt")
}

func TestP2BinaryAndLargeFilesAreSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/blob.bin", []byte{0, 1, 2, 0, 255}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/huge.txt", []byte(string(make([]byte, 1024*1024+1))), 0644); err != nil {
		t.Fatal(err)
	}
	e := open(t, root)
	requireNoPath(t, e.search(t, "blob"), "blob.bin")
	if e.status(t).IndexedFiles != 0 {
		t.Fatalf("binary and oversized text must be excluded from index: %+v", e.status(t))
	}
}

func TestP2DefaultBuildOutputFoldersAreExcluded(t *testing.T) {
	root := t.TempDir()
	put(t, root, "target/generated-sources/FakeGenerated.java", "class FakeGenerated {}\n")
	put(t, root, "build/cache.txt", "do not index build-private-value\n")
	put(t, root, "src/Included.java", "class Included {}\n")
	e := open(t, root)
	requirePath(t, e.search(t, "Included"), "Included.java")
	requireNoPath(t, e.search(t, "FakeGenerated"), "FakeGenerated.java")
	requireNoPath(t, e.search(t, "build-private-value"), "cache.txt")
}
