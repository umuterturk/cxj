package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"example.com/cx/internal/engine"
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
	case "find", "mcp":
		fmt.Fprintf(os.Stderr, "command %q is not implemented yet\n", os.Args[1])
		os.Exit(2)
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
	fs.Var(&excludes, "exclude", "repeatable path/glob exclusion (applied in later phases)")
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
	_ = excludes
	_ = embedURL
	_ = embedModel

	eng, err := engine.Open(*root)
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

// multiFlag collects repeated --exclude values.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
