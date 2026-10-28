// Package scan walks a workspace root and classifies indexable files.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"example.com/cx/internal/exclude"
	"example.com/cx/internal/ir"
)

const defaultMaxFileBytes = 1 << 20 // 1 MiB

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
	SkippedExcluded  int
	SkippedScope     int
}

// Result is the outcome of Scan.
type Result struct {
	Files []File
	Stats Stats
}

// Options control scanning.
type Options struct {
	Excludes []string // --exclude patterns
	MaxBytes int64
	Scope    Scope // which source tree to include (default ScopeAll)
}

// Scope selects which source tree the scanner keeps. The zero value, ScopeAll,
// preserves the historical "scan everything" behavior.
type Scope int

const (
	ScopeAll  Scope = iota // main + test (default)
	ScopeMain              // main sources only
	ScopeTest              // test sources only
)

// keep reports whether a relative path is in scope.
func (s Scope) keep(rel string) bool {
	switch s {
	case ScopeMain:
		return !isTestPath(rel)
	case ScopeTest:
		return isTestPath(rel)
	default:
		return true
	}
}

func isTestPath(p string) bool { return ir.IsTestPath(p) }

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

// Scan walks root with default + .cxignore + flag exclusions; no symlink following.
func Scan(root string, opts Options) (Result, error) {
	if err := ValidateRoot(root); err != nil {
		return Result{}, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	ignoreLines, err := exclude.LoadCxIgnore(absRoot)
	if err != nil {
		return Result{}, err
	}
	matcher := exclude.New(ignoreLines, opts.Excludes)
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}

	var out Result
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}

		if d.IsDir() {
			if path == absRoot {
				return nil
			}
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
			if matcher.Match(rel) {
				out.Stats.SkippedExcluded++
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
		if matcher.Match(rel) {
			out.Stats.SkippedExcluded++
			return nil
		}
		if !opts.Scope.keep(rel) {
			out.Stats.SkippedScope++
			return nil
		}
		if info.Size() > maxBytes {
			out.Stats.SkippedOversize++
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !utf8.Valid(data) {
			out.Stats.SkippedInvalid++
			return nil
		}
		if looksBinary(data) {
			out.Stats.SkippedBinary++
			return nil
		}
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
