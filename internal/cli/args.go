package cli

import (
	"fmt"
	"strings"
)

// SplitArgs separates GNU-style flags from positional args so flags may follow
// the query (e.g. `cx find ToolSmoke --root PATH --json`).
// Value-taking flags must have a value; a bare `--root` returns an error
// instead of letting flag.Parse steal the next flag.
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
			return nil, nil, fmt.Errorf("missing value for %s", name)
		}
		i++
		flags = append(flags, name, args[i])
	}
	return flags, positional, nil
}
