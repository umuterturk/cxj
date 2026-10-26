// Package needle loads the Needle native engine from the app folder.
package needle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

const (
	libDarwin   = "libneedle.dylib"
	libDarwin3  = "libneedle3.dylib"
	libWindows  = "libneedle.dll"
	libWindows3 = "libneedle3.dll"
	libUnix     = "libneedle.so"
	libUnix3    = "libneedle3.so"
	weightsName = "needle3.cact"
	bufferSize  = 65536
	maxTokens   = 512
	needle3Ver  = "3.3.3"
	needle2Ver  = "2.0.4"
)

// Tools is the cx tool catalogue Needle routes against.
var Tools = []map[string]any{
	{
		"name":        "brief",
		"description": "Workspace orientation: kind, packages, all ingress (HTTP, scheduled, kafka, runners), profiles.",
		"triggers":    []string{"brief", "what is this project", "entrypoints", "overview"},
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		"name":        "find",
		"description": "Search the workspace for a Java symbol or source text by name.",
		"triggers":    []string{"find", "search", "where is", "locate"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "symbol or text to search"},
				"limit": map[string]any{"type": "integer", "description": "max results"},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "show",
		"description": "Print the source definition of a Java class or method, capped at 80 lines.",
		"triggers":    []string{"show", "body", "definition of", "source of"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "class or method name"},
			},
			"required": []string{"symbol"},
		},
	},
	{
		"name":        "outline",
		"description": "Print package, types, and methods of a Java file or symbol without bodies.",
		"triggers":    []string{"outline", "structure of", "methods in"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "file path or symbol"},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "map",
		"description": "Print a compact package to type to file index of the workspace.",
		"triggers":    []string{"map", "packages"},
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		"name":        "trail",
		"description": "One-hop neighborhood: HTTP, injects, callers, callees, tests. No bodies.",
		"triggers":    []string{"trail", "how does", "around"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "class or method name"},
			},
			"required": []string{"symbol"},
		},
	},
	{
		"name":        "keys",
		"description": "List Spring config key names and profiles. Values omitted; secrets flagged.",
		"triggers":    []string{"config keys", "list keys"},
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		"name":        "callees",
		"description": "Find methods that a Java class or method calls.",
		"triggers":    []string{"what does it call", "callees of"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "the calling class or method"},
			},
			"required": []string{"symbol"},
		},
	},
	{
		"name":        "callers",
		"description": "Find call sites of a Java class or method. Excludes the declaration.",
		"triggers":    []string{"who calls", "callers of", "who uses"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "the class or method name being called"},
			},
			"required": []string{"symbol"},
		},
	},
	{
		"name":        "references",
		"description": "Find declarations, implementations, and uses of a Java class or method.",
		"triggers":    []string{"references", "usages of"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "the class or method name"},
			},
			"required": []string{"symbol"},
		},
	},
	{
		"name":        "endpoints",
		"description": "List Spring HTTP routes, optionally filtered by method and path.",
		"triggers":    []string{"endpoint", "route", "http mapping"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method": map[string]any{"type": "string", "description": "HTTP method such as GET"},
				"path":   map[string]any{"type": "string", "description": "route path such as /owners"},
			},
		},
	},
	{
		"name":        "beans",
		"description": "Find Spring beans injected for a type.",
		"triggers":    []string{"bean", "who injects", "wired"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{"type": "string", "description": "bean or interface type"},
			},
			"required": []string{"type"},
		},
	},
	{
		"name":        "config",
		"description": "Resolve a Spring configuration key.",
		"triggers":    []string{"config", "property", "application.yml"},
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":     map[string]any{"type": "string", "description": "dotted config key"},
				"profile": map[string]any{"type": "string", "description": "Spring profile"},
			},
			"required": []string{"key"},
		},
	},
}

// Call is one tool invocation from the engine.
type Call struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Response is the Needle complete envelope.
type Response struct {
	Type          string   `json:"type"`
	FunctionCalls []Call   `json:"function_calls"`
	Confidence    *float64 `json:"confidence"`
}

type native struct {
	gen       int
	init      func(system, tools, index *byte) int32
	complete2 func(text *byte, maxTokens int32, buf *byte, n int32) int32
	complete3 func(text *byte, pcm *float32, samples int32, maxTokens int32, buf *byte, n int32) int32
	reset     func()
	load      func(data *byte, n uint64) int32
	system    []byte
	tools     []byte
}

var (
	once    sync.Once
	eng     *native
	loadErr error
)

// LibNames returns candidate shared-library filenames, generation-3 first.
func LibNames() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{libDarwin3, libDarwin}
	case "windows":
		return []string{libWindows3, libWindows}
	default:
		return []string{libUnix3, libUnix}
	}
}

// SearchDirs returns folders that may contain the engine (app folder first).
func SearchDirs() []string {
	var dirs []string
	add := func(d string) {
		if d == "" {
			return
		}
		d = filepath.Clean(d)
		for _, e := range dirs {
			if e == d {
				return
			}
		}
		dirs = append(dirs, d)
	}
	add(os.Getenv("CX_NEEDLE_DIR"))
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		add(dir)
		add(filepath.Join(dir, "needle"))
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			dir = filepath.Dir(real)
			add(dir)
			add(filepath.Join(dir, "needle"))
		}
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
		add(filepath.Join(wd, "needle"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".cache", "cactus-needle", "v3", needle3Ver))
		add(filepath.Join(home, ".cache", "cactus-needle", "v2", needle2Ver))
	}
	return dirs
}

// FindLibrary locates libneedle next to the binary or under CX_NEEDLE_DIR.
func FindLibrary() (string, error) {
	for _, key := range []string{"NEEDLE3_LIB_PATH", "NEEDLE_LIB_PATH", "NEEDLE2_LIB_PATH"} {
		if p := os.Getenv(key); p != "" {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	var tried []string
	for _, dir := range SearchDirs() {
		for _, name := range LibNames() {
			path := filepath.Join(dir, name)
			tried = append(tried, path)
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("Needle engine not found (looked for %v). Run `cxj ask` to download, or set CX_NEEDLE_DIR. searched: %v", LibNames(), tried)
}

func cptr(b []byte) *byte {
	if len(b) == 0 {
		return nil
	}
	return &b[0]
}

func detectGeneration(libPath string) int {
	if stringsHas3(filepath.Base(libPath)) {
		return 3
	}
	if w := weightsBeside(libPath); w != "" {
		return 3
	}
	return 2
}

func stringsHas3(name string) bool {
	return len(name) >= 10 && (name == libDarwin3 || name == libUnix3 || name == libWindows3)
}

func load() (*native, error) {
	once.Do(func() {
		path, err := FindLibrary()
		if err != nil {
			loadErr = err
			return
		}
		lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			loadErr = fmt.Errorf("load %s: %w", path, err)
			return
		}
		n := &native{gen: detectGeneration(path)}
		purego.RegisterLibFunc(&n.init, lib, "needle_init")
		purego.RegisterLibFunc(&n.reset, lib, "needle_reset")
		purego.RegisterLibFunc(&n.load, lib, "needle_load")
		if n.gen >= 3 {
			purego.RegisterLibFunc(&n.complete3, lib, "needle_complete")
			w := weightsBeside(path)
			if w == "" {
				loadErr = fmt.Errorf("Needle 3 requires %s next to %s. Re-run `cxj ask` to download", weightsName, path)
				return
			}
			data, readErr := os.ReadFile(w)
			if readErr != nil {
				loadErr = fmt.Errorf("read %s: %w", w, readErr)
				return
			}
			if n.load(cptr(data), uint64(len(data))) < 0 {
				loadErr = fmt.Errorf("needle_load failed for %s", w)
				return
			}
		} else {
			purego.RegisterLibFunc(&n.complete2, lib, "needle_complete")
			if w := weightsBeside(path); w != "" {
				data, readErr := os.ReadFile(w)
				if readErr != nil {
					loadErr = fmt.Errorf("read %s: %w", w, readErr)
					return
				}
				if n.load(cptr(data), uint64(len(data))) < 0 {
					loadErr = fmt.Errorf("needle_load failed for %s", w)
					return
				}
			}
		}
		toolsJSON, err := json.Marshal(Tools)
		if err != nil {
			loadErr = err
			return
		}
		n.system = append([]byte{}, 0)
		n.tools = append(append([]byte{}, toolsJSON...), 0)
		if rc := n.init(cptr(n.system), cptr(n.tools), nil); rc < 0 {
			loadErr = fmt.Errorf("needle_init failed (code %d) using %s", rc, path)
			return
		}
		eng = n
	})
	return eng, loadErr
}

func weightsBeside(libPath string) string {
	dir := filepath.Dir(libPath)
	for _, name := range []string{weightsName, "needle2.cact"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// Complete runs one Needle turn and returns tool calls.
func Complete(query string) (Response, error) {
	n, err := load()
	if err != nil {
		return Response{}, err
	}
	text := append([]byte(query), 0)
	buf := make([]byte, bufferSize)
	runtime.KeepAlive(n.system)
	runtime.KeepAlive(n.tools)
	var rc int32
	if n.gen >= 3 {
		rc = n.complete3(cptr(text), nil, 0, maxTokens, cptr(buf), int32(len(buf)))
	} else {
		rc = n.complete2(cptr(text), maxTokens, cptr(buf), int32(len(buf)))
	}
	runtime.KeepAlive(text)
	if rc < 0 {
		detail := cString(buf)
		if detail == "" {
			detail = fmt.Sprintf("code %d", rc)
		}
		return Response{}, fmt.Errorf("needle_complete failed: %s", detail)
	}
	raw := cString(buf)
	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return Response{}, fmt.Errorf("needle JSON: %v; output=%s", err, raw)
	}
	return resp, nil
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
