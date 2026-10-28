package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P3 restructured Scan into walk-then-parallel-validate with a capped read.
// These pin the contract the restructure must not break: the oversize decision
// lands on the same boundary the stat made, kept bytes are exactly the file's
// bytes, and rejected content never leaks into Files.

func TestScanOversizeBoundary(t *testing.T) {
	root := t.TempDir()
	const cap = 64
	write := func(name string, size int) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Repeat("x", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("exact.java", cap)   // exactly at the cap: kept (the old stat asked size > max)
	write("over.java", cap+1)  // one byte over: oversize, unread beyond cap+1
	write("under.java", cap-1) // under: kept

	res, err := Scan(root, Options{MaxBytes: cap})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.SkippedOversize != 1 {
		t.Errorf("SkippedOversize = %d, want 1", res.Stats.SkippedOversize)
	}
	if res.Stats.IndexedFiles != 2 {
		t.Errorf("IndexedFiles = %d, want 2", res.Stats.IndexedFiles)
	}
}

func TestScanCarriesValidatedBytes(t *testing.T) {
	root := t.TempDir()
	body := []byte("package com.x;\n// é valid utf-8 ✓\n")
	if err := os.WriteFile(filepath.Join(root, "Ok.java"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// Invalid UTF-8 and NUL-bearing files must land in their stats buckets and
	// never appear in Files — rejected content cannot ride in on Data.
	if err := os.WriteFile(filepath.Join(root, "Bad.java"), []byte{0xff, 0xfe, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Bin.java"), []byte("a\x00b"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 || res.Files[0].RelPath != "Ok.java" {
		t.Fatalf("files = %+v", res.Files)
	}
	if string(res.Files[0].Data) != string(body) {
		t.Errorf("carried bytes differ from disk")
	}
	if res.Files[0].Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", res.Files[0].Size, len(body))
	}
	if res.Stats.SkippedInvalid != 1 || res.Stats.SkippedBinary != 1 {
		t.Errorf("stats = %+v, want 1 invalid + 1 binary", res.Stats)
	}
}

// TestScanWalkOrderStable: collection order is the walk order, not the workers'
// finish order — downstream byte-for-byte comparisons depend on it. (The
// schedule itself can still shuffle verdict timing, so this pins the ordering
// law at the one place it is decided.)
func TestScanWalkOrderStable(t *testing.T) {
	root := t.TempDir()
	const n = 200
	for i := 0; i < n; i++ {
		name := filepath.Join(root, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(name, []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != n || len(second.Files) != n {
		t.Fatalf("got %d/%d files, want %d", len(first.Files), len(second.Files), n)
	}
	for i := range first.Files {
		if first.Files[i].RelPath != second.Files[i].RelPath {
			t.Fatalf("order differs at %d: %s vs %s", i, first.Files[i].RelPath, second.Files[i].RelPath)
		}
	}
	// And it is sorted-walk order (os.ReadDir sorts), i.e. deterministic across runs.
	if got := first.Files[0].RelPath; got != "f000.txt" {
		t.Errorf("first file %q, want f000.txt", got)
	}
}
