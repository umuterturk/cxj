package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"example.com/cx/internal/engine"
	"example.com/cx/internal/mcp"
	"example.com/cx/internal/scan"
	"example.com/cx/internal/server"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "mcp":
		os.Exit(runMCP(os.Args[2:]))
	case "find":
		os.Exit(runFind(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  cx serve --root /abs/workspace --http 127.0.0.1:8765 [--exclude PATTERN]...
           [--embed-url URL --embed-model NAME]
  cx mcp --root /abs/workspace [--exclude PATTERN]... [--embed-url URL --embed-model NAME]
  cx find QUERY --server http://127.0.0.1:8765 [--mode lexical|semantic|hybrid] [--json]
`)
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "absolute workspace root (required)")
	httpAddr := fs.String("http", "127.0.0.1:8765", "loopback bind address")
	embedURL := fs.String("embed-url", "", "Ollama base URL (optional; enables semantic)")
	embedModel := fs.String("embed-model", "", "embedding model name")
	var excludes multiFlag
	fs.Var(&excludes, "exclude", "repeatable path/glob exclusion")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*root) == "" {
		fmt.Fprintln(os.Stderr, "--root is required")
		return 2
	}
	if err := scan.ValidateRoot(*root); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	eng, err := engine.Open(*root, engine.Options{
		Excludes:   excludes,
		EmbedURL:   strings.TrimSpace(*embedURL),
		EmbedModel: strings.TrimSpace(*embedModel),
		Logger:     log.Default(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	srv := server.New(eng, log.Default())
	if err := srv.ListenAndServe(*httpAddr); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "absolute workspace root (required)")
	embedURL := fs.String("embed-url", "", "Ollama base URL (optional)")
	embedModel := fs.String("embed-model", "", "embedding model name")
	var excludes multiFlag
	fs.Var(&excludes, "exclude", "repeatable path/glob exclusion")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*root) == "" {
		fmt.Fprintln(os.Stderr, "--root is required")
		return 2
	}
	if err := scan.ValidateRoot(*root); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	eng, err := engine.Open(*root, engine.Options{
		Excludes:   excludes,
		EmbedURL:   strings.TrimSpace(*embedURL),
		EmbedModel: strings.TrimSpace(*embedModel),
		Logger:     log.Default(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	srv := mcp.New(eng, log.Default())
	if err := srv.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runFind(args []string) int {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := fs.String("server", "http://127.0.0.1:8765", "cx serve base URL")
	mode := fs.String("mode", "hybrid", "lexical|semantic|hybrid")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "query required")
		return 2
	}
	q := strings.Join(fs.Args(), " ")
	u := strings.TrimRight(*serverURL, "/") + "/v1/search?q=" + url.QueryEscape(q) + "&mode=" + url.QueryEscape(*mode)
	resp, err := http.Get(u)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "search failed: HTTP %d: %s\n", resp.StatusCode, string(body))
		return 1
	}
	if *asJSON {
		os.Stdout.Write(body)
		return 0
	}
	var parsed struct {
		Results []struct {
			Path   string  `json:"path"`
			Line   int     `json:"line"`
			Symbol string  `json:"symbol"`
			Score  float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	for _, r := range parsed.Results {
		fmt.Printf("%s:%d\t%s\n", r.Path, r.Line, r.Symbol)
	}
	return 0
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
