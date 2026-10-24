package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestP0ServerReadyOnLocalWorkspace(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/Main.java", "class Main {}\n")
	e := start(t, root, false)
	s := e.status(t)
	if s.IndexedFiles < 1 || s.IndexedJavaFiles != 1 {
		t.Fatalf("wrong file counters: %+v", s)
	}
	if s.SemanticState != "off" {
		t.Fatalf("unexpected semantic mode: %s", s.SemanticState)
	}
}
func TestP0RejectNonexistentRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cmd := exec.Command(binary(t), "serve", "--root", missing, "--http", "127.0.0.1:0")
	err := cmd.Run()
	if err == nil {
		t.Fatal("invalid root must fail immediately with nonzero exit")
	}
}
func TestP0NeverPersistsGeneratedIndex(t *testing.T) {
	root := t.TempDir()
	put(t, root, "A.java", "class A {}\n")
	_ = start(t, root, false)
	for _, name := range []string{".cx", ".cxindex", "index.db", "vectors.bin"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("generated index persisted as %s", name)
		}
	}
}
