// Package scan walks a workspace root and classifies indexable files.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const defaultMaxFileBytes = 1 << 20 // 1 MiB

// defaultExcludeDir names that are skipped at every directory level.
var defaultExcludeDir = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	".idea":        {},
	"target":       {},
	"build":        {},
	"out":          {},
	".gradle":      {},
}

// File describes one indexable text file relative to root.
type File struct {
	RelPath string // forward-slash relative path
	AbsPath string
	IsJava  bool
	Size    int64
}

// Stats summarizes a scan.
type Stats struct {
	IndexedFiles     int
	IndexedJavaFiles int
	SkippedBinary    int
	SkippedOversize  int
	SkippedInvalid   int
}

// Result is the outcome of Scan.
type Result struct {
	Files []File
	Stats Stats
}

// ValidateRoot ensures root exists and is a directory.
func ValidateRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("root does not exist: %s", root)
		}
		return fmt.Errorf("root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("root is not a directory: %s", root)
	}
	return nil
}

// Scan walks root, skipping default exclude dirs, symlinks, binaries, and oversized files.
func Scan(root string) (Result, error) {
	if err := ValidateRoot(root); err != nil {
		return Result{}, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}

	var out Result
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path == absRoot {
				return nil
			}
			if _, skip := defaultExcludeDir[name]; skip {
				return filepath.SkipDir
			}
			// Never follow symlinked directories.
			if d.Type()&fs.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > defaultMaxFileBytes {
			out.Stats.SkippedOversize++
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !utf8.Valid(data) || looksBinary(data) {
			if !utf8.Valid(data) {
				out.Stats.SkippedInvalid++
			} else {
				out.Stats.SkippedBinary++
			}
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		isJava := strings.HasSuffix(strings.ToLower(rel), ".java")
		out.Files = append(out.Files, File{
			RelPath: rel,
			AbsPath: path,
			IsJava:  isJava,
			Size:    info.Size(),
		})
		out.Stats.IndexedFiles++
		if isJava {
			out.Stats.IndexedJavaFiles++
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return out, nil
}

func looksBinary(data []byte) bool {
	// NUL byte is a strong binary signal for source tooling.
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
