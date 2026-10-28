// Package version holds the cxj CLI version stamped at link time.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is the release tag (e.g. v0.2.0). Override with
// -ldflags="-X github.com/umuterturk/cxj/internal/version.Version=vX.Y.Z".
var Version = "dev"

// Line is the compact `cxj version` text.
func Line() string {
	return "cxj " + Current()
}

// Current returns the resolved version string.
func Current() string {
	v := strings.TrimSpace(Version)
	if v != "" && v != "dev" {
		return normalize(v)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if mv := strings.TrimSpace(info.Main.Version); mv != "" && mv != "(devel)" {
			return normalize(mv)
		}
	}
	if v == "" {
		return "dev"
	}
	return v
}

// Normalize strips a leading v for comparisons.
func Normalize(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return v
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// Same reports whether two version strings refer to the same release.
func Same(a, b string) bool {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return false
	}
	if na == "dev" || nb == "dev" {
		return false
	}
	return na == nb
}
