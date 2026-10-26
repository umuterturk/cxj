package skill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultBinDir = "/usr/local/bin"

// InstallBinDir is the on-PATH directory for the cxj executable.
// CXJ_BIN_DIR overrides for tests. Prefer /usr/local/bin; if that is not
// writable, use /opt/homebrew/bin when that directory exists.
func InstallBinDir() string {
	if d := strings.TrimSpace(os.Getenv("CXJ_BIN_DIR")); d != "" {
		return d
	}
	if dirWritable(defaultBinDir) {
		return defaultBinDir
	}
	if dirWritable("/opt/homebrew/bin") {
		return "/opt/homebrew/bin"
	}
	return defaultBinDir
}

// BinDir is the directory that will receive the binary. home is unused;
// kept so existing callers compile.
func BinDir(home string) string {
	_ = home
	return InstallBinDir()
}

// InstallBinary copies the running executable to InstallBinDir()/cxj,
// overwriting any existing file or symlink. Skipped on OSes without a
// conventional /usr/local/bin.
func InstallBinary(home string) (string, error) {
	_ = home
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	src := exe
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		src = real
	}
	dir := InstallBinDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	dest := filepath.Join(dir, "cxj")
	if samePath(src, dest) {
		_ = os.Chmod(dest, 0755)
		return dest, nil
	}
	if err := overwriteFile(src, dest); err != nil {
		return "", err
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

func dirWritable(dir string) bool {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".cxj-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
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

func overwriteFile(src, dest string) error {
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
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Remove(dest); err != nil {
			return fmt.Errorf("overwrite %s: %w", dest, err)
		}
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install %s: %w", dest, err)
	}
	return nil
}
