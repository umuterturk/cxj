package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallLocal(t *testing.T) {
	root := t.TempDir()
	paths, err := InstallLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, ".agents", "skills", Name, "SKILL.md")
	link := filepath.Join(root, ".claude", "skills", Name)
	if len(paths) != 2 || paths[0] != skill || paths[1] != link {
		t.Fatalf("paths=%v", paths)
	}
	data, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty SKILL.md")
	}
	assertClaudeLink(t, link, filepath.Join("..", "..", ".agents", "skills", Name), skill)
	// Idempotent.
	if _, err := InstallLocal(root); err != nil {
		t.Fatal(err)
	}
}

func TestInstallGlobal(t *testing.T) {
	home := t.TempDir()
	paths, err := InstallGlobal(home)
	if err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(home, ".agents", "skills", Name, "SKILL.md")
	link := filepath.Join(home, ".claude", "skills", Name)
	if paths[0] != skill || paths[1] != link {
		t.Fatalf("paths=%v", paths)
	}
	assertClaudeLink(t, link, filepath.Join("..", "..", ".agents", "skills", Name), skill)
}

func TestInstallLocal_replacesCopiedClaudeDir(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, ".claude", "skills", Name)
	if err := os.MkdirAll(old, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "SKILL.md"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallLocal(root); err != nil {
		t.Fatal(err)
	}
	assertClaudeLink(t, old, filepath.Join("..", "..", ".agents", "skills", Name),
		filepath.Join(root, ".agents", "skills", Name, "SKILL.md"))
}

func assertClaudeLink(t *testing.T, link, wantTarget, skillPath string) {
	t.Helper()
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", link)
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantTarget {
		t.Fatalf("readlink=%q want=%q", got, wantTarget)
	}
	// Resolve and confirm SKILL.md is reachable through the link.
	via := filepath.Join(link, "SKILL.md")
	data, err := os.ReadFile(via)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatal("symlink does not resolve to installed SKILL.md")
	}
}
