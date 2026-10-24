package cli

import (
	"fmt"
	"strings"
)

// SplitArgs separates GNU-style flags from positional args so flags may follow
// the query (e.g. `cx find ToolSmoke --server URL --json`).
// Value-taking flags must have a value; a bare `--server` (e.g. when `$S` is
// empty) returns an error instead of letting flag.Parse steal the next flag.
func SplitArgs(args []string) (flags, positional []string, err error) {
	boolFlags := map[string]struct{}{
		"--json": {}, "-json": {},
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		name, inline, hasInline := strings.Cut(a, "=")
		if _, isBool := boolFlags[name]; isBool {
			if hasInline {
				return nil, nil, fmt.Errorf("flag %s takes no value", name)
			}
			flags = append(flags, name)
			continue
		}
		if hasInline {
			flags = append(flags, name, inline)
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return nil, nil, fmt.Errorf("missing value for %s (check that the variable is set, e.g. S=http://127.0.0.1:8765)", name)
		}
		i++
		flags = append(flags, name, args[i])
	}
	return flags, positional, nil
}
