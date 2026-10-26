package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallBinDir_override(t *testing.T) {
	t.Setenv("CXJ_BIN_DIR", "/opt/tools/bin")
	if got := InstallBinDir(); got != "/opt/tools/bin" {
		t.Fatalf("got %q", got)
	}
}

func TestInstallBinDir_default(t *testing.T) {
	t.Setenv("CXJ_BIN_DIR", "")
	got := InstallBinDir()
	if got != defaultBinDir && got != "/opt/homebrew/bin" {
		t.Fatalf("got %q", got)
	}
}

func TestInstallBinary_overwrites(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unix only")
	}
	dir := t.TempDir()
	t.Setenv("CXJ_BIN_DIR", dir)
	dest := filepath.Join(dir, "cxj")
	if err := os.WriteFile(dest, []byte("old-binary"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := InstallBinary("")
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("dest=%q want=%q", got, dest)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0111 == 0 {
		t.Fatalf("not executable: %v", st.Mode())
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "old-binary" {
		t.Fatal("did not overwrite existing dest")
	}
	got2, err := InstallBinary("")
	if err != nil {
		t.Fatal(err)
	}
	if got2 != dest {
		t.Fatalf("second dest=%q", got2)
	}
}

func TestInstallBinary_overwritesSymlink(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unix only")
	}
	dir := t.TempDir()
	t.Setenv("CXJ_BIN_DIR", dir)
	target := filepath.Join(dir, "old")
	if err := os.WriteFile(target, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "cxj")
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallBinary(""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("left a symlink in place")
	}
	stale, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(stale) != "stale" {
		t.Fatal("overwrote symlink target instead of replacing the link")
	}
}

func TestBinOnPATH(t *testing.T) {
	dir := t.TempDir()
	if !BinOnPATH(dir, "/usr/bin:"+dir+":/bin") {
		t.Fatal("expected on PATH")
	}
	if BinOnPATH(dir, "/usr/bin:/bin") {
		t.Fatal("expected not on PATH")
	}
}
