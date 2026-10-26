package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"example.com/cx/internal/ask"
	"example.com/cx/internal/cli"
	"example.com/cx/internal/engine"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/needle"
	"example.com/cx/internal/scan"
	"example.com/cx/internal/skill"
	"example.com/cx/internal/update"
	"example.com/cx/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "brief":
		os.Exit(runBrief(os.Args[2:]))
	case "find":
		os.Exit(runFind(os.Args[2:]))
	case "show":
		os.Exit(runShow(os.Args[2:]))
	case "outline":
		os.Exit(runOutline(os.Args[2:]))
	case "map":
		os.Exit(runMap(os.Args[2:]))
	case "trail":
		os.Exit(runTrail(os.Args[2:]))
	case "references":
		os.Exit(runReferences(os.Args[2:]))
	case "callers":
		os.Exit(runCallers(os.Args[2:]))
	case "callees":
		os.Exit(runCallees(os.Args[2:]))
	case "keys":
		os.Exit(runKeys(os.Args[2:]))
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
	case "ask":
		os.Exit(runAsk(os.Args[2:]))
	case "install":
		os.Exit(runInstall(os.Args[2:]))
	case "version":
		os.Exit(runVersion(os.Args[2:]))
	case "update":
		os.Exit(runUpdate(os.Args[2:]))
	case "--version", "-version":
		os.Exit(runVersion(os.Args[2:]))
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
  cxj brief [--root PATH] [--json]
  cxj find QUERY [--root PATH] [--limit 5] [--verbose] [--compact] [--json]
  cxj show SYMBOL [--root PATH] [--max-lines 80] [--json]
  cxj outline PATH|SYMBOL [--root PATH] [--json]
  cxj map [--root PATH] [--json]
  cxj trail SYMBOL [--root PATH] [--json]
  cxj callers SYMBOL [--root PATH] [--verbose] [--compact] [--json]
  cxj callees SYMBOL [--root PATH] [--verbose] [--compact] [--json]
  cxj references SYMBOL [--root PATH] [--verbose] [--compact] [--json]
  cxj keys [--root PATH] [--json]
  cxj endpoints [--method GET] [--path /owners] [--root PATH] [--json]
  cxj beans --type SomeType [--root PATH] [--json]
  cxj config --key a.b.c [--profile prod] [--root PATH] [--json]
  cxj context --symbol Qualified.name [--budget 500] [--root PATH]
  cxj status [--root PATH] [--json]
  cxj ask QUESTION [--root PATH] [--download] [--show-command] [--verbose] [--json]
  cxj install               # project: .agents/skills + .claude/skills symlink
  cxj install -g            # global: ~/.agents/skills + symlink; /usr/local/bin/cxj
  cxj version [--json]
  cxj update [--check] [--json]
  cxj COMMAND --example     # 2-3 workspace examples
`)
}

func runVersion(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--example", "-example":
			return printStaticExamples("version")
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "usage: cxj version [--json]\n")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown version flag %q\n", a)
			return 2
		}
	}
	if asJSON {
		return cli.NewPrinter().JSON(map[string]string{"name": "cxj", "version": version.Current()})
	}
	fmt.Println(version.Line())
	return 0
}

func runUpdate(args []string) int {
	check := false
	asJSON := false
	for _, a := range args {
		switch a {
		case "--example", "-example":
			return printStaticExamples("update")
		case "--check":
			check = true
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "usage: cxj update [--check] [--json]\n")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown update flag %q\n", a)
			return 2
		}
	}
	c := &update.Client{}
	if check {
		res, err := c.Check()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		return printUpdate(res, asJSON)
	}
	dest, err := update.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	res, err := c.Apply(dest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if !res.UpToDate {
		cmd := exec.Command(dest, "install", "-g")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "note: binary updated; skill refresh failed: %s\n%s", err, out)
		}
	}
	return printUpdate(res, asJSON)
}

func printUpdate(res update.Result, asJSON bool) int {
	if asJSON {
		return cli.NewPrinter().JSON(res)
	}
	if res.UpToDate {
		fmt.Printf("%s (latest)\n", version.Line())
		return 0
	}
	if res.Path != "" {
		fmt.Printf("%s -> %s\n", res.Current, res.Latest)
		fmt.Println(res.Path)
		return 0
	}
	fmt.Printf("%s latest=%s\n", version.Line(), res.Latest)
	return 0
}

func runInstall(args []string) int {
	global := false
	for _, a := range args {
		switch a {
		case "--example", "-example":
			return printStaticExamples("install")
		case "-g", "--global":
			global = true
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "usage: cxj install [-g|--global]\n")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown install flag %q\n", a)
			return 2
		}
	}
	if global {
		return runInstallGlobal()
	}
	return runInstallLocal()
}

func runInstallLocal() int {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	written, err := skill.InstallLocal(root)
	for _, p := range written {
		fmt.Println(p)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runInstallGlobal() int {
	home, err := skill.HomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	code := 0
	if bin, err := skill.InstallBinary(home); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		code = 1
	} else if bin != "" {
		fmt.Println(bin)
		if !skill.BinOnPATH(filepath.Dir(bin), os.Getenv("PATH")) {
			fmt.Fprintf(os.Stderr, "note: %s is not on PATH\n", filepath.Dir(bin))
		}
	}
	written, err := skill.InstallGlobal(home)
	for _, p := range written {
		fmt.Println(p)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return code
}

type common struct {
	root     *string
	asJSON   *bool
	compact  *bool
	verbose  *bool
	example  *bool
	excludes multiFlag
}

func addCommon(fs *flag.FlagSet) *common {
	c := &common{}
	c.root = fs.String("root", "", "workspace root (default: current directory)")
	c.asJSON = fs.Bool("json", false, "emit JSON")
	c.compact = fs.Bool("compact", false, "strip JSON to path,line,symbol")
	c.verbose = fs.Bool("verbose", false, "extra columns (text) or full ask data")
	c.example = fs.Bool("example", false, "print workspace examples and exit")
	fs.Var(&c.excludes, "exclude", "repeatable path/glob exclusion")
	return c
}

func printExamples(c *common, cmd string) int {
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return writeExamples(eng.Examples(cmd))
}

func printStaticExamples(cmd string) int {
	return writeExamples(engine.StaticExamples(cmd))
}

func writeExamples(ex []engine.Example) int {
	for _, x := range ex {
		fmt.Printf("%s  # %s\n", x.Cmd, x.Why)
	}
	return 0
}

func (c *common) mode() cli.Mode {
	return cli.Mode{JSON: *c.asJSON, Compact: *c.compact, Verbose: *c.verbose}
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

func runBrief(args []string) int {
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "brief")
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Brief(eng.Brief(), c.mode())
}

func runTrail(args []string) int {
	fs := flag.NewFlagSet("trail", flag.ContinueOnError)
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
	if *c.example {
		return printExamples(c, "trail")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Trail(eng.Trail(strings.Join(positional, " ")), c.mode())
}

func runCallees(args []string) int {
	fs := flag.NewFlagSet("callees", flag.ContinueOnError)
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
	if *c.example {
		return printExamples(c, "callees")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().References(eng.Callees(strings.Join(positional, " ")), c.mode())
}

func runKeys(args []string) int {
	fs := flag.NewFlagSet("keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "keys")
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Keys(eng.Keys(), c.mode())
}

func runFind(args []string) int {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	limit := fs.Int("limit", ir.DefaultFindLimit, "max results (1-50)")
	flagArgs, positional, err := cli.SplitArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "find")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "query required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Find(eng.Search(strings.Join(positional, " "), *limit), c.mode())
}

func runShow(args []string) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	maxLines := fs.Int("max-lines", ir.DefaultShowLines, "hard cap on printed lines")
	flagArgs, positional, err := cli.SplitArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "show")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Show(eng.Show(strings.Join(positional, " "), *maxLines), c.mode())
}

func runOutline(args []string) int {
	fs := flag.NewFlagSet("outline", flag.ContinueOnError)
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
	if *c.example {
		return printExamples(c, "outline")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "path or symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Outline(eng.Outline(strings.Join(positional, " ")), c.mode())
}

func runMap(args []string) int {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "map")
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().Map(eng.Map(), c.mode())
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
	if *c.example {
		return printExamples(c, "references")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().References(eng.References(strings.Join(positional, " ")), c.mode())
}

func runCallers(args []string) int {
	fs := flag.NewFlagSet("callers", flag.ContinueOnError)
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
	if *c.example {
		return printExamples(c, "callers")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "symbol required")
		return 2
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	return cli.NewPrinter().References(eng.Callers(strings.Join(positional, " ")), c.mode())
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
	if *c.example {
		return printExamples(c, "endpoints")
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
	if *c.example {
		return printExamples(c, "beans")
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
	if *c.example {
		return printExamples(c, "config")
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
	if *c.example {
		return printExamples(c, "context")
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

func runAsk(args []string) int {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	showCommand := fs.Bool(ask.ShowCommandFlag, false, "prefix the reconstructed cxj command when a tool param is set")
	download := fs.Bool("download", false, "download Needle 3 without prompting")
	flagArgs, positional, err := cli.SplitArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "ask")
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "question required")
		return 2
	}
	if err := needle.Ensure(os.Stdin, os.Stderr, *download); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	eng, err := openEngine(c)
	if err != nil {
		return failOpen(err)
	}
	reply, err := ask.Run(eng, strings.Join(positional, " "), *showCommand, *c.verbose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	p := cli.NewPrinter()
	if *c.asJSON {
		return p.JSON(reply)
	}
	fmt.Fprintln(p.Out, reply.Answer)
	return 0
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	c := addCommon(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *c.example {
		return printExamples(c, "status")
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
