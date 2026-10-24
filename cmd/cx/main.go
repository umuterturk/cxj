package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"example.com/cx/internal/cli"
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
	case "find":
		os.Exit(runFind(os.Args[2:]))
	case "references":
		os.Exit(runReferences(os.Args[2:]))
	case "endpoints":
		os.Exit(runEndpoints(os.Args[2:]))
	case "beans":
		os.Exit(runBeans(os.Args[2:]))
	case "config":
		os.Exit(runConfig(os.Args[2:]))
	case "context":
		os.Exit(runContext(os.Args[2:]))
	case "status":
		os.Exit(runStatus(os.Args[2:]))
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
  cx status --server http://127.0.0.1:8765 [--json]
  cx find QUERY --server http://127.0.0.1:8765 [--mode lexical|semantic|hybrid] [--limit 10] [--json]
  cx references SYMBOL --server URL [--json]
  cx endpoints --server URL [--method GET] [--path /owners] [--json]
  cx beans --type SomeType --server URL [--json]
  cx config --key a.b.c [--profile prod] --server URL [--json]
  cx context --symbol Qualified.name [--budget 500] --server URL
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

func serverFlag(fs *flag.FlagSet) *string {
	return fs.String("server", "http://127.0.0.1:8765", "cx serve base URL")
}

func runFind(args []string) int {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	mode := fs.String("mode", "hybrid", "lexical|semantic|hybrid")
	limit := fs.Int("limit", 10, "max results (1-50)")
	asJSON := fs.Bool("json", false, "emit JSON")
	flagArgs, positional, err := cli.SplitArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "query required")
		return 2
	}
	return cli.New(*serverURL).Find(strings.Join(positional, " "), *mode, *limit, *asJSON)
}

func runReferences(args []string) int {
	fs := flag.NewFlagSet("references", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	asJSON := fs.Bool("json", false, "emit JSON")
	flagArgs, positional, err := cli.SplitArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	return cli.New(*serverURL).References(strings.Join(positional, " "), *asJSON)
}

func runEndpoints(args []string) int {
	fs := flag.NewFlagSet("endpoints", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	method := fs.String("method", "", "HTTP method filter")
	path := fs.String("path", "", "route filter")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.New(*serverURL).Endpoints(*method, *path, *asJSON)
}

func runBeans(args []string) int {
	fs := flag.NewFlagSet("beans", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	typ := fs.String("type", "", "bean/interface type")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*typ) == "" {
		fmt.Fprintln(os.Stderr, "--type is required")
		return 2
	}
	return cli.New(*serverURL).Beans(*typ, *asJSON)
}

func runConfig(args []string) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	key := fs.String("key", "", "config key")
	profile := fs.String("profile", "", "Spring profile")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*key) == "" {
		fmt.Fprintln(os.Stderr, "--key is required")
		return 2
	}
	return cli.New(*serverURL).Config(*key, *profile, *asJSON)
}

func runContext(args []string) int {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	symbol := fs.String("symbol", "", "qualified symbol")
	budget := fs.Int("budget", 500, "text budget")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*symbol) == "" {
		fmt.Fprintln(os.Stderr, "--symbol is required")
		return 2
	}
	return cli.New(*serverURL).Context(*symbol, *budget, true)
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverURL := serverFlag(fs)
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return cli.New(*serverURL).Status(*asJSON)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
