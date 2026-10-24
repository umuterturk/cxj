package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"example.com/cx/internal/cli"
	"example.com/cx/internal/engine"
	"example.com/cx/internal/scan"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
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
  cx status [--root PATH] [--exclude PATTERN]... [--json]
  cx find QUERY [--root PATH] [--limit 10] [--exclude PATTERN]... [--json]
  cx references SYMBOL [--root PATH] [--exclude PATTERN]... [--json]
  cx endpoints [--method GET] [--path /owners] [--root PATH] [--exclude PATTERN]... [--json]
  cx beans --type SomeType [--root PATH] [--exclude PATTERN]... [--json]
  cx config --key a.b.c [--profile prod] [--root PATH] [--exclude PATTERN]... [--json]
  cx context --symbol Qualified.name [--budget 500] [--root PATH] [--exclude PATTERN]...
`)
}

type common struct {
	root     *string
	asJSON   *bool
	excludes multiFlag
}

func addCommon(fs *flag.FlagSet) *common {
	c := &common{}
	c.root = fs.String("root", "", "workspace root (default: current directory)")
	c.asJSON = fs.Bool("json", false, "emit JSON")
	fs.Var(&c.excludes, "exclude", "repeatable path/glob exclusion")
	return c
}

func openEngine(c *common) (*engine.Engine, error) {
	root := strings.TrimSpace(*c.root)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = wd
	}
	if err := scan.ValidateRoot(root); err != nil {
		return nil, err
	}
	return engine.Open(root, engine.Options{Excludes: c.excludes})
}

func failOpen(err error) int {
	fmt.Fprintln(os.Stderr, err.Error())
	return 1
}

func runFind(args []string) int {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	limit := fs.Int("limit", 10, "max results (1-50)")
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
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Find(eng.Search(strings.Join(positional, " "), *limit), *c.asJSON)
}

func runReferences(args []string) int {
	fs := flag.NewFlagSet("references", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
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
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().References(eng.References(strings.Join(positional, " ")), *c.asJSON)
}

func runEndpoints(args []string) int {
	fs := flag.NewFlagSet("endpoints", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	method := fs.String("method", "", "HTTP method filter")
	path := fs.String("path", "", "route filter")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Generic(eng.Endpoints(*method, *path), *c.asJSON)
}

func runBeans(args []string) int {
	fs := flag.NewFlagSet("beans", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	typ := fs.String("type", "", "bean/interface type")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*typ) == "" {
		fmt.Fprintln(os.Stderr, "--type is required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Generic(eng.Beans(*typ), *c.asJSON)
}

func runConfig(args []string) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	key := fs.String("key", "", "config key")
	profile := fs.String("profile", "", "Spring profile")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*key) == "" {
		fmt.Fprintln(os.Stderr, "--key is required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Generic(eng.Config(*key, *profile), *c.asJSON)
}

func runContext(args []string) int {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	symbol := fs.String("symbol", "", "qualified symbol")
	budget := fs.Int("budget", 500, "text budget")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*symbol) == "" {
		fmt.Fprintln(os.Stderr, "--symbol is required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().JSON(eng.Context(*symbol, *budget))
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Status(eng.Status(), *c.asJSON)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
