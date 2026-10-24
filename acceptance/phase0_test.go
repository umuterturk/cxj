package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestP0StatusOnLocalWorkspace(t *testing.T) {
	root := t.TempDir()
	put(t, root, "src/Main.java", "class Main {}\n")
	e := open(t, root)
	s := e.status(t)
	if s.Status != "ready" {
		t.Fatalf("expected ready, got %+v", s)
	}
	if s.IndexedFiles < 1 || s.IndexedJavaFiles != 1 {
		t.Fatalf("wrong file counters: %+v", s)
	}
}

func TestP0RejectNonexistentRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cmd := exec.Command(binary(t), "status", "--root", missing, "--json")
	err := cmd.Run()
	if err == nil {
		t.Fatal("invalid root must fail immediately with nonzero exit")
	}
}

func TestP0NeverPersistsGeneratedIndex(t *testing.T) {
	root := t.TempDir()
	put(t, root, "A.java", "class A {}\n")
	_ = open(t, root).status(t)
	for _, name := range []string{".cx", ".cxindex", "index.db", "vectors.bin"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("generated index persisted as %s", name)
		}
	}
}
