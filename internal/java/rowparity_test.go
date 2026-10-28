package java

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umuterturk/cxj/internal/benchfixture"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// P5's premise is that the parser already knows which row every node starts on, so the
// line-number scan can be deleted rather than merely sped up. That is only safe if a
// tree-sitter row and a newline count are the same number, everywhere, always.
//
// lineByCountingNewlines is deliberately a second, dumb implementation rather than a
// call to the code being replaced: comparing the new path against the old one proves
// they agree only while both exist, and stops proving anything the moment the old one is
// deleted. Ground truth survives the refactor.

func lineByCountingNewlines(src []byte, off uint) int {
	if off > uint(len(src)) {
		off = uint(len(src))
	}
	line := 1
	for i := uint(0); i < off; i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

func assertRowsMatchNewlines(t *testing.T, label string, src []byte, n *tree_sitter.Node) int {
	t.Helper()
	if n == nil {
		return 0
	}
	seen := 1
	for _, tc := range []struct {
		what string
		off  uint
		row  uint
	}{
		{"start", n.StartByte(), n.StartPosition().Row},
		{"end", n.EndByte(), n.EndPosition().Row},
	} {
		if want, got := lineByCountingNewlines(src, tc.off), int(tc.row)+1; want != got {
			t.Errorf("%s: %s of %q at byte %d: tree-sitter says line %d, newline count says %d",
				label, tc.what, n.Kind(), tc.off, got, want)
		}
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		seen += assertRowsMatchNewlines(t, label, src, n.Child(i))
	}
	return seen
}

func TestTreeSitterRowsAreTheLineNumbersWeReport(t *testing.T) {
	files := fixtureJavaFiles(t)
	if len(files) == 0 {
		t.Fatal("no java sources to compare: the parity claim would be vacuous")
	}
	ext := NewExtractor()
	defer ext.Close()
	checked := 0
	for path, data := range files {
		tree := ext.parser.Parse(data, nil)
		if tree == nil {
			t.Fatalf("parse failed: %s", path)
		}
		checked += assertRowsMatchNewlines(t, path, data, tree.RootNode())
		tree.Close()
	}
	t.Logf("compared %d node positions across %d files", checked, len(files))
	if checked < 1000 {
		t.Errorf("only %d positions compared: the corpus is too small to prove this", checked)
	}
}

// TestTreeSitterRowsOnAwkwardSources covers the cases where a row count and a newline
// count could plausibly disagree: CRLF endings, multi-byte characters, a text block with
// blank lines, a byte-order mark, and a file whose last line has no terminator. The
// fixture is synthetic and uniformly LF/ASCII, so without this the claim above would
// only cover the easy case.
func TestTreeSitterRowsOnAwkwardSources(t *testing.T) {
	cases := map[string]string{
		"crlf":        "package a;\r\n\r\npublic class C {\r\n    void m() {}\r\n}\r\n",
		"multibyte":   "package a;\n\n// Ünicode — 注释 — 🎉\npublic class M {\n    String s = \"日本語\";\n}\n",
		"textblock":   "package a;\n\npublic class T {\n    String s = \"\"\"\n        line1\n\n        line3\n        \"\"\";\n}\n",
		"no-final-nl": "package a;\npublic class N { }",
		"tabs":        "package a;\n\npublic class B {\n\tvoid m() {\n\t\tm();\n\t}\n}\n",
		"bom":         "\ufeffpackage a;\npublic class Z { }\n",
	}
	ext := NewExtractor()
	defer ext.Close()
	for name, body := range cases {
		src := []byte(body)
		tree := ext.parser.Parse(src, nil)
		if tree == nil {
			t.Fatalf("%s: parse failed", name)
		}
		assertRowsMatchNewlines(t, name, src, tree.RootNode())
		tree.Close()
	}
}

// fixtureJavaFiles collects java sources from the benchmark fixture, or from
// CXJ_JAVA_DIR so the parity check can be run over a real repository.
func fixtureJavaFiles(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	collect := func(dir string) {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".java") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			out[p] = data
			return nil
		})
	}
	if dir := os.Getenv("CXJ_JAVA_DIR"); dir != "" {
		collect(dir)
		return out
	}
	root := filepath.Join(t.TempDir(), "fx")
	if _, err := benchfixture.Write(root, benchfixture.Options{
		Modules: 2, TypesPerModule: 4, TestTypesPerModule: 2,
	}); err != nil {
		t.Fatal(err)
	}
	collect(root)
	return out
}
