// Package ask routes a free-text question through Needle onto a cx tool.
package ask

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"example.com/cx/internal/engine"
	"example.com/cx/internal/ir"
	"example.com/cx/internal/needle"
)

// Call is one Needle tool invocation.
type Call struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Route is Needle's tool choice.
type Route struct {
	FunctionCalls []Call   `json:"function_calls"`
	Confidence    *float64 `json:"confidence"`
	Type          string   `json:"type"`
}

// Reply is the cx ask payload.
type Reply struct {
	Command    string `json:"command,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Arguments  any    `json:"arguments,omitempty"`
	Answer     string `json:"answer"`
	Data       any    `json:"data,omitempty"`
	Confidence any    `json:"confidence,omitempty"`
}

var (
	whoCalls   = regexp.MustCompile(`(?i)\bwho\s+(?:calls?|uses?|references?)\s+['"]?([A-Za-z_][\w.]*)`)
	whoCallees = regexp.MustCompile(`(?i)\bwhat\s+does\s+['"]?([A-Za-z_][\w.]*)['"]?\s+call`)
	showOf     = regexp.MustCompile(`(?i)\b(?:show|body|definition of|source of)\s+['"]?([A-Za-z_][\w.]*)`)
	outlineOf  = regexp.MustCompile(`(?i)\boutline\s+['"]?([A-Za-z_./][\w./]*)`)
	trailOf    = regexp.MustCompile(`(?i)\b(?:how does|trail|around|neighborhood of)\s+['"]?([A-Za-z_][\w.]*)`)
	briefOf    = regexp.MustCompile(`(?i)\b(?:brief|what is this (?:project|repo|codebase)|entrypoints?|overview)\b`)
	keysOf     = regexp.MustCompile(`(?i)\b(?:config keys|list keys|what keys)\b`)
	mapOf      = regexp.MustCompile(`(?i)\b(?:map|packages)\b`)
)

// RouteQuery asks the bundled Needle engine which cx tool to run.
func RouteQuery(query string) (Route, error) {
	resp, err := needle.Complete(query)
	if err != nil {
		return Route{}, err
	}
	return Route{
		FunctionCalls: toCalls(resp.FunctionCalls),
		Confidence:    resp.Confidence,
		Type:          resp.Type,
	}, nil
}

func toCalls(in []needle.Call) []Call {
	out := make([]Call, 0, len(in))
	for _, c := range in {
		out = append(out, Call{Name: c.Name, Arguments: c.Arguments})
	}
	return out
}

// CorrectIntent overrides Needle when the question is a known cheap pattern.
func CorrectIntent(query string, route Route) Route {
	if briefOf.MatchString(query) {
		return Route{
			FunctionCalls: []Call{{Name: "brief", Arguments: map[string]any{}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if keysOf.MatchString(query) {
		return Route{
			FunctionCalls: []Call{{Name: "keys", Arguments: map[string]any{}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if m := whoCallees.FindStringSubmatch(query); m != nil {
		return Route{
			FunctionCalls: []Call{{Name: "callees", Arguments: map[string]any{"symbol": m[1]}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if m := whoCalls.FindStringSubmatch(query); m != nil {
		return Route{
			FunctionCalls: []Call{{Name: "callers", Arguments: map[string]any{"symbol": m[1]}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if m := trailOf.FindStringSubmatch(query); m != nil {
		return Route{
			FunctionCalls: []Call{{Name: "trail", Arguments: map[string]any{"symbol": m[1]}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if m := showOf.FindStringSubmatch(query); m != nil {
		return Route{
			FunctionCalls: []Call{{Name: "show", Arguments: map[string]any{"symbol": m[1]}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if m := outlineOf.FindStringSubmatch(query); m != nil {
		return Route{
			FunctionCalls: []Call{{Name: "outline", Arguments: map[string]any{"query": m[1]}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	if mapOf.MatchString(query) {
		return Route{
			FunctionCalls: []Call{{Name: "map", Arguments: map[string]any{}}},
			Confidence:    route.Confidence,
			Type:          "call",
		}
	}
	return route
}

// CommandLine reconstructs the cxj invocation from a tool call. Only set params are included.
func CommandLine(call Call, root string) string {
	parts := []string{"cxj", call.Name}
	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}
	switch call.Name {
	case "find":
		if q := strArg(args, "query"); q != "" {
			parts = append(parts, quote(q))
		}
		if n, ok := intArg(args, "limit"); ok {
			parts = append(parts, "--limit", strconv.Itoa(n))
		}
	case "show":
		if s := strArg(args, "symbol"); s != "" {
			parts = append(parts, quote(s))
		}
	case "outline":
		if q := strArg(args, "query"); q != "" {
			parts = append(parts, quote(q))
		}
	case "brief", "map", "keys":
	case "trail", "callers", "callees", "references":
		if s := strArg(args, "symbol"); s != "" {
			parts = append(parts, quote(s))
		}
	case "endpoints":
		if m := strArg(args, "method"); m != "" {
			parts = append(parts, "--method", m)
		}
		if p := strArg(args, "path"); p != "" {
			parts = append(parts, "--path", p)
		}
	case "beans":
		if t := strArg(args, "type"); t != "" {
			parts = append(parts, "--type", t)
		}
	case "config":
		if k := strArg(args, "key"); k != "" {
			parts = append(parts, "--key", k)
		}
		if p := strArg(args, "profile"); p != "" {
			parts = append(parts, "--profile", p)
		}
	case "context":
		if s := strArg(args, "symbol"); s != "" {
			parts = append(parts, "--symbol", s)
		}
		if n, ok := intArg(args, "budget"); ok {
			parts = append(parts, "--budget", strconv.Itoa(n))
		}
	}
	if root != "" {
		parts = append(parts, "--root", quote(root))
	}
	return strings.Join(parts, " ")
}

// ShowCommand is the CLI flag that prefixes the reconstructed tool call.
const ShowCommandFlag = "show-command"

// Run executes the routed tool against eng and formats a compact answer.
func Run(eng *engine.Engine, query string, showCommand, verbose bool) (Reply, error) {
	route, err := RouteQuery(query)
	if err != nil {
		return Reply{}, err
	}
	route = CorrectIntent(query, route)
	if len(route.FunctionCalls) == 0 {
		return Reply{Answer: "I could not map that question to a cxj command."}, nil
	}
	call := route.FunctionCalls[0]
	data, answer, err := execTool(eng, call)
	if err != nil {
		return Reply{}, err
	}
	cmd := ""
	if showCommand && hasSetParam(call) {
		cmd = CommandLine(call, relativeRoot(eng.Root()))
		answer = cmd + "\n" + answer
	}
	reply := Reply{
		Command:    cmd,
		Tool:       call.Name,
		Arguments:  call.Arguments,
		Answer:     answer,
		Confidence: route.Confidence,
	}
	if verbose {
		reply.Data = data
	}
	return reply, nil
}

func execTool(eng *engine.Engine, call Call) (any, string, error) {
	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}
	switch call.Name {
	case "find":
		q := strArg(args, "query")
		limit := ir.DefaultFindLimit
		if n, ok := intArg(args, "limit"); ok {
			limit = n
		}
		data := eng.Search(q, limit)
		return data, formatFind(data), nil
	case "show":
		sym := strArg(args, "symbol")
		data := eng.Show(sym, ir.DefaultShowLines)
		if data.Resolved != "" {
			args["symbol"] = data.Resolved
		}
		return data, formatShow(data), nil
	case "outline":
		q := strArg(args, "query")
		data := eng.Outline(q)
		return data, formatOutline(data), nil
	case "brief":
		data := eng.Brief()
		return data, formatBrief(data), nil
	case "map":
		data := eng.Map()
		return data, formatMap(data), nil
	case "trail":
		sym := strArg(args, "symbol")
		data := eng.Trail(sym)
		if data.Resolved != "" {
			args["symbol"] = data.Resolved
		}
		return data, formatTrail(data), nil
	case "keys":
		data := eng.Keys()
		return data, formatKeys(data), nil
	case "callees":
		sym := strArg(args, "symbol")
		data := eng.Callees(sym)
		if data.Resolved != "" {
			args["symbol"] = data.Resolved
		}
		return data, formatRefs(data), nil
	case "callers":
		sym := strArg(args, "symbol")
		data := eng.Callers(sym)
		if data.Resolved != "" {
			args["symbol"] = data.Resolved
		}
		return data, formatRefs(data), nil
	case "references":
		sym := strArg(args, "symbol")
		data := eng.References(sym)
		if data.Resolved != "" {
			args["symbol"] = data.Resolved
		}
		return data, formatRefs(data), nil
	case "endpoints":
		method, path := strArg(args, "method"), strArg(args, "path")
		data := eng.Endpoints(method, path)
		return data, formatEndpoints(data), nil
	case "beans":
		typ := strArg(args, "type")
		data := eng.Beans(typ)
		return data, formatBeans(data), nil
	case "config":
		key, profile := strArg(args, "key"), strArg(args, "profile")
		data := eng.Config(key, profile)
		return data, formatConfig(data), nil
	case "context":
		sym := strArg(args, "symbol")
		budget := 500
		if n, ok := intArg(args, "budget"); ok {
			budget = n
		}
		data := eng.Context(sym, budget)
		return data, formatContext(data), nil
	default:
		return nil, "", fmt.Errorf("unknown tool %q", call.Name)
	}
}

func formatBrief(reply ir.BriefReply) string {
	return ir.FormatBrief(reply)
}

func formatTrail(reply ir.TrailReply) string {
	if reply.Path == "" {
		return "none"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s:%d\n", reply.Kind, reply.Symbol, reply.Path, reply.Line)
	write := func(label string, hits []ir.TrailHit, total int) {
		for _, h := range hits {
			fmt.Fprintf(&b, "%s %s:%d %s %s\n", label, h.Path, h.Line, h.Detail, h.Symbol)
		}
		if more := total - len(hits); more > 0 {
			fmt.Fprintf(&b, "%s … (+%d more)\n", label, more)
		}
	}
	t := reply.Totals
	write("http", reply.HTTP, t.HTTP)
	write("injects", reply.Injects, t.Injects)
	write("callers", reply.Callers, t.Callers)
	write("callees", reply.Callees, t.Callees)
	write("tests", reply.Tests, t.Tests)
	return strings.TrimRight(b.String(), "\n")
}

func formatKeys(reply ir.KeysReply) string {
	if len(reply.Keys) == 0 {
		return "none"
	}
	var b strings.Builder
	if len(reply.Profiles) > 0 {
		fmt.Fprintf(&b, "profiles %s\n", strings.Join(reply.Profiles, ","))
	}
	for _, k := range reply.Keys {
		if k.Secret {
			fmt.Fprintf(&b, "%s [REDACTED]\n", k.Key)
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", k.Key, k.Resolution)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatFind(reply ir.SearchReply) string {
	if len(reply.Results) == 0 {
		return "none"
	}
	var b strings.Builder
	for _, r := range reply.Results {
		fmt.Fprintf(&b, "%s:%d %s\n", r.Path, r.Line, r.Symbol)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatShow(reply ir.ShowReply) string {
	if reply.Path == "" {
		return "none"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s:%d %s %s\n", reply.Path, reply.Line, reply.Kind, reply.Symbol)
	if reply.Body != "" {
		b.WriteString(reply.Body)
		b.WriteByte('\n')
	}
	if reply.Truncated {
		b.WriteString("... [truncated]")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatOutline(reply ir.OutlineReply) string {
	if reply.Path == "" {
		return "none"
	}
	var b strings.Builder
	pkg := reply.Package
	if pkg == "" {
		pkg = "(default)"
	}
	fmt.Fprintf(&b, "pkg %s  %s\n", pkg, reply.Path)
	for _, t := range reply.Types {
		end := t.EndLine
		if end < t.Line {
			end = t.Line
		}
		fmt.Fprintf(&b, "  %s %s %d-%d\n", t.Kind, t.Name, t.Line, end)
		for _, mem := range t.Members {
			fmt.Fprintf(&b, "    %s %s %d\n", mem.Kind, mem.Name, mem.Line)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatMap(reply ir.MapReply) string {
	if len(reply.Packages) == 0 {
		return "none"
	}
	var b strings.Builder
	for _, pkg := range reply.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default)"
		}
		fmt.Fprintln(&b, name)
		for _, t := range pkg.Types {
			fmt.Fprintf(&b, "  %s  %s:%d\n", t.Name, t.Path, t.Line)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatRefs(reply ir.RefReply) string {
	if len(reply.Results) == 0 {
		return "none"
	}
	var b strings.Builder
	if reply.Resolved != "" {
		fmt.Fprintf(&b, "resolved %s\n", reply.Resolved)
	}
	for _, r := range reply.Results {
		kind := string(r.Kind)
		if kind == "" {
			kind = "ref"
		}
		fmt.Fprintf(&b, "%s:%d %s %s\n", r.Path, r.Line, kind, r.Target)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatEndpoints(data any) string {
	raw, _ := json.Marshal(data)
	var parsed struct {
		Results []struct {
			HTTPMethod string `json:"http_method"`
			Route      string `json:"route"`
			Path       string `json:"path"`
			Line       int    `json:"line"`
			Symbol     string `json:"symbol"`
		} `json:"results"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if len(parsed.Results) == 0 {
		return "none"
	}
	var b strings.Builder
	for _, r := range parsed.Results {
		fmt.Fprintf(&b, "%s %s %s:%d %s\n", r.HTTPMethod, r.Route, r.Path, r.Line, r.Symbol)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatBeans(data any) string {
	raw, _ := json.Marshal(data)
	var parsed struct {
		Results []struct {
			Bean         string   `json:"bean"`
			Symbol       string   `json:"symbol"`
			InjectedInto []string `json:"injected_into"`
		} `json:"results"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if len(parsed.Results) == 0 {
		return "none"
	}
	var b strings.Builder
	for _, r := range parsed.Results {
		into := strings.Join(r.InjectedInto, ",")
		fmt.Fprintf(&b, "%s %s %s\n", r.Bean, r.Symbol, into)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatConfig(data any) string {
	raw, _ := json.Marshal(data)
	var parsed struct {
		RepositoryValue string `json:"repository_value"`
		Resolution      string `json:"resolution"`
		Definitions     []struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Value   string `json:"value"`
			Profile string `json:"profile"`
		} `json:"definitions"`
	}
	_ = json.Unmarshal(raw, &parsed)
	var b strings.Builder
	fmt.Fprintf(&b, "%s", parsed.Resolution)
	if parsed.RepositoryValue != "" {
		fmt.Fprintf(&b, " %s", parsed.RepositoryValue)
	}
	b.WriteByte('\n')
	for _, d := range parsed.Definitions {
		fmt.Fprintf(&b, "%s:%d %s\n", d.Path, d.Line, d.Value)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatContext(data any) string {
	raw, _ := json.Marshal(data)
	return string(raw)
}

func hasSetParam(call Call) bool {
	if call.Name == "map" || call.Name == "brief" || call.Name == "keys" {
		return true
	}
	for _, v := range call.Arguments {
		if str, ok := v.(string); ok && strings.TrimSpace(str) != "" {
			return true
		}
		if v != nil {
			return true
		}
	}
	return false
}

func strArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func intArg(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case json.Number:
		n, err := t.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(t)
		return n, err == nil
	default:
		return 0, false
	}
}

func quote(s string) string {
	if s == "" || !strings.ContainsAny(s, " \t\"'") {
		return s
	}
	return strconv.Quote(s)
}

func relativeRoot(root string) string {
	wd, err := os.Getwd()
	if err != nil {
		return root
	}
	rel, err := filepath.Rel(wd, root)
	if err != nil {
		return root
	}
	return rel
}
