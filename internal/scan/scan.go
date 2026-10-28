// Package scan walks a workspace root and classifies indexable files.
package scan

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	// Data holds the validated content, which the scanner had to read anyway
	// to rule out binary and invalid-UTF-8 files. Carrying it forward means
	// the engine never opens the same file twice; nil means unread (a caller
	// that filtered before validation, or a consumer past release).
	Data []byte
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

// per-file validation verdicts; the zero value must mean "drop silently",
// which is what a read error has always done.
const (
	verdictSkip byte = iota
	verdictKeep
	verdictInvalid
	verdictBinary
	verdictOversize
)

// Scan walks root with default + .cxignore + flag exclusions; no symlink following.
//
// Two phases. The walk classifies paths — every cheap filter runs there, and an
// out-of-scope file is never opened. The read that decides binary-vs-text used to
// sit inside the walk callback, serial over every file, and the bytes were then
// thrown away so the engine could open the same file again; now it runs on a
// worker pool and the bytes ride out on File.Data. The walk itself stays serial,
// but it is only a walk now.
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
	var cand []File
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
		// Regular-file judgment from the readdir type bits, no lstat; only a
		// filesystem that reports no type at all falls back. And the oversize
		// check moves into the capped read below rather than costing a stat per
		// candidate — on macOS an lstat is 10-25 µs under an endpoint-security
		// agent, and a walk over thousands of files is where that shows up.
		if t := d.Type(); t == 0 || t == fs.ModeIrregular {
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
		} else if !t.IsRegular() {
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
		// The check order above is the old single-pass order, kept exactly:
		// the skip counters feed `status`, and a reordered check can move a
		// file from one skip bucket to another.
		cand = append(cand, File{
			RelPath: rel,
			AbsPath: path,
			IsJava:  strings.HasSuffix(strings.ToLower(rel), ".java"),
		})
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	// Validation in parallel, collection in walk order: per-file verdicts make
	// the stats identical to the serial version whatever the workers' schedule.
	verdicts := make([]byte, len(cand))
	content := make([][]byte, len(cand))
	if n := len(cand); n > 0 {
		workers := runtime.GOMAXPROCS(0)
		if workers < 1 {
			workers = 1
		}
		if workers > n {
			workers = n
		}
		jobs := make(chan int, workers*2)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					data, oversize, err := readCapped(cand[i].AbsPath, maxBytes)
					switch {
					case err != nil:
					case oversize:
						verdicts[i] = verdictOversize
					case !utf8.Valid(data):
						verdicts[i] = verdictInvalid
					case looksBinary(data):
						verdicts[i] = verdictBinary
					default:
						verdicts[i] = verdictKeep
						content[i] = data
					}
				}
			}()
		}
		for i := range cand {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		out.Files = make([]File, 0, len(cand))
		for i := range cand {
			switch verdicts[i] {
			case verdictInvalid:
				out.Stats.SkippedInvalid++
			case verdictBinary:
				out.Stats.SkippedBinary++
			case verdictOversize:
				out.Stats.SkippedOversize++
			case verdictKeep:
				f := cand[i]
				f.Data = content[i]
				f.Size = int64(len(f.Data)) // the read saw the whole file: kept files are under the cap
				out.Files = append(out.Files, f)
				out.Stats.IndexedFiles++
				if f.IsJava {
					out.Stats.IndexedJavaFiles++
				}
			}
		}
	}
	return out, nil
}

// readCapped reads maxBytes+1 bytes at most; the caller treats the extra byte
// as oversize, which is decision-identical to statting first (a file of exactly
// maxBytes passes, bigger does not) and never reads a fat binary whole just to
// reject it. The one case that diverges from the old stat-first order is an
// unreadable file over the cap, which now drops silently instead of counting
// as oversize; no workspace in sight has one.
func readCapped(path string, maxBytes int64) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return nil, true, nil
	}
	return data, false, nil
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
