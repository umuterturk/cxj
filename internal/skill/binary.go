package skill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BinDir is the user-local install directory for the cxj binary ($HOME/bin).
func BinDir(home string) string {
	return filepath.Join(home, "bin")
}

// InstallBinary copies the running executable to $HOME/bin/cxj on macOS.
// Other platforms are skipped (empty path, nil error).
func InstallBinary(home string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", nil
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	src := exe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		src = real
	}
	dir := BinDir(home)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	dest := filepath.Join(dir, "cxj")
	if samePath(src, dest) {
		return dest, nil
	}
	if err := copyFile(src, dest); err != nil {
		return "", err
	}
	if err := os.Chmod(dest, 0755); err != nil {
		return "", fmt.Errorf("chmod %s: %w", dest, err)
	}
	return dest, nil
}

// BinOnPATH reports whether dir appears as a PATH entry.
func BinOnPATH(dir, pathEnv string) bool {
	if dir == "" {
		return false
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		want = dir
	}
	for _, p := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if p == "" {
			continue
		}
		got, err := filepath.Abs(p)
		if err != nil {
			got = p
		}
		if got == want {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return aa == bb
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".cxj-install-*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("copy: %w", err)
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install %s: %w", dest, err)
	}
	return nil
}
