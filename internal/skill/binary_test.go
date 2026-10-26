package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallBinary_macOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	home := t.TempDir()
	dest, err := InstallBinary(home)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "bin", "cxj")
	if dest != want {
		t.Fatalf("dest=%q want=%q", dest, want)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0111 == 0 {
		t.Fatalf("not executable: %v", st.Mode())
	}
	// Second install from already-installed path should be a no-op success.
	// Simulate by replacing os.Executable is hard; just re-copy from current exe again.
	dest2, err := InstallBinary(home)
	if err != nil {
		t.Fatal(err)
	}
	if dest2 != want {
		t.Fatalf("second dest=%q want=%q", dest2, want)
	}
}

func TestInstallBinary_emptyHome(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := InstallBinary(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestBinOnPATH(t *testing.T) {
	home := t.TempDir()
	dir := BinDir(home)
	if BinOnPATH(dir, "/usr/bin:"+dir+":/bin") {
		// ok — path contains dir
	} else {
		t.Fatal("expected on PATH")
	}
	if BinOnPATH(dir, "/usr/bin:/bin") {
		t.Fatal("expected not on PATH")
	}
}
