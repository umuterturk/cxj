// Package spring extracts Spring MVC/DI metadata from Java sources.
package spring

import (
	"strings"
	"sync"

	"example.com/cx/internal/ir"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

var (
	langOnce sync.Once
	lang     *tree_sitter.Language
)

func language() *tree_sitter.Language {
	langOnce.Do(func() { lang = tree_sitter.NewLanguage(tsjava.Language()) })
	return lang
}

// Endpoint is one HTTP mapping.
type Endpoint struct {
	Path       string
	HTTPMethod string
	Symbol     string
	File       string
	Line       int
	Certainty  ir.Certainty
}

// Bean is a Spring stereotype or @Bean definition.
type Bean struct {
	Name       string
	Symbol     string
	Types      []string // implemented interfaces + self simple name
	File       string
	Line       int
	Primary    bool
	Qualifiers []string
}

// Injection is a constructor/field dependency edge.
type Injection struct {
	Consumer  string
	Type      string
	Qualifier string
	File      string
	Line      int
}

// TransactionDecl records a declared @Transactional (not runtime-effective).
type TransactionDecl struct {
	Symbol string
	File   string
	Line   int
}

// RepositoryRel records Spring Data repository extension.
type RepositoryRel struct {
	Symbol  string
	Extends string
	File    string
	Line    int
}

// Facts is Spring metadata for one file.
type Facts struct {
	Endpoints    []Endpoint
	Beans        []Bean
	Injections   []Injection
	Transactions []TransactionDecl
	Repositories []RepositoryRel
}

// Extractor parses Spring annotations via Tree-sitter.
type Extractor struct {
	parser *tree_sitter.Parser
}

// NewExtractor constructs a reusable extractor.
func NewExtractor() *Extractor {
	p := tree_sitter.NewParser()
	p.SetLanguage(language())
	return &Extractor{parser: p}
}

// Close releases C resources.
func (e *Extractor) Close() {
	if e.parser != nil {
		e.parser.Close()
		e.parser = nil
	}
}

// Extract parses src for Spring facts.
func (e *Extractor) Extract(path string, src []byte) Facts {
	tree := e.parser.Parse(src, nil)
	if tree == nil {
		return Facts{}
	}
	defer tree.Close()
	root := tree.RootNode()
	pkg := findPackage(root, src)
	var out Facts
	walk(root, src, path, pkg, nil, &out)
	return out
}

func findPackage(root *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c != nil && c.Kind() == "package_declaration" {
			return strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(nodeText(c, src), "package")), ";")
		}
	}
	return ""
}

type classInfo struct {
	name        string
	qual        string
	basePath    string
	stereotypes []string
	beanName    string
	implements  []string
}

func walk(n *tree_sitter.Node, src []byte, path, pkg string, class *classInfo, out *Facts) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "class_declaration", "interface_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			break
		}
		name := nodeText(nameNode, src)
		qual := name
		if pkg != "" {
			qual = pkg + "." + name
		}
		anns := annotationsOf(n, src)
		ci := &classInfo{
			name: name,
			qual: qual,
		}
		for _, a := range anns {
			switch a.Name {
			case "RequestMapping":
				ci.basePath = firstPath(a)
			case "RestController", "Controller", "Service", "Component", "Repository":
				ci.stereotypes = append(ci.stereotypes, a.Name)
				if a.Name == "Service" || a.Name == "Component" || a.Name == "Repository" || a.Name == "Controller" || a.Name == "RestController" {
					if v := firstStringArg(a); v != "" {
						ci.beanName = v
					}
				}
			case "Transactional":
				out.Transactions = append(out.Transactions, TransactionDecl{
					Symbol: qual, File: path, Line: lineAt(src, n.StartByte()),
				})
			}
		}
		ci.implements = typeNamesFrom(n, src, "interfaces", "superclass", "super_interfaces", "extends_interfaces")

		if isBeanStereo(ci.stereotypes) {
			beanName := ci.beanName
			if beanName == "" {
				beanName = decapitalize(name)
			}
			types := append([]string{name}, ci.implements...)
			out.Beans = append(out.Beans, Bean{
				Name: beanName, Symbol: qual, Types: types,
				File: path, Line: lineAt(src, nameNode.StartByte()),
			})
		}

		// Repository extends JpaRepository etc.
		if n.Kind() == "interface_declaration" {
			for _, ext := range ci.implements {
				if isSpringDataRepo(ext) {
					out.Repositories = append(out.Repositories, RepositoryRel{
						Symbol: qual, Extends: ext, File: path, Line: lineAt(src, nameNode.StartByte()),
					})
				}
			}
		}

		body := n.ChildByFieldName("body")
		if body != nil {
			for i := uint(0); i < body.NamedChildCount(); i++ {
				walkMember(body.NamedChild(i), src, path, ci, out)
			}
		}
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walk(n.NamedChild(i), src, path, pkg, class, out)
	}
}

func walkMember(n *tree_sitter.Node, src []byte, path string, class *classInfo, out *Facts) {
	if n == nil || class == nil {
		return
	}
	switch n.Kind() {
	case "method_declaration", "constructor_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			return
		}
		name := nodeText(nameNode, src)
		symbol := class.qual + "." + name
		anns := annotationsOf(n, src)
		for _, a := range anns {
			switch a.Name {
			case "GetMapping", "PostMapping", "PutMapping", "DeleteMapping", "PatchMapping", "RequestMapping":
				method := httpMethodOf(a.Name, a)
				route := joinRoute(class.basePath, firstPath(a))
				out.Endpoints = append(out.Endpoints, Endpoint{
					Path: route, HTTPMethod: method, Symbol: symbol,
					File: path, Line: lineAt(src, nameNode.StartByte()), Certainty: ir.CertaintyExact,
				})
			case "Transactional":
				out.Transactions = append(out.Transactions, TransactionDecl{
					Symbol: symbol, File: path, Line: lineAt(src, nameNode.StartByte()),
				})
			}
		}
		// Constructor / method parameter injections
		params := n.ChildByFieldName("parameters")
		if params != nil && n.Kind() == "constructor_declaration" {
			for i := uint(0); i < params.NamedChildCount(); i++ {
				p := params.NamedChild(i)
				if p == nil || p.Kind() != "formal_parameter" {
					continue
				}
				typ := paramType(p, src)
				qual := ""
				for _, a := range annotationsOf(p, src) {
					if a.Name == "Qualifier" {
						qual = firstStringArg(a)
					}
				}
				if typ != "" {
					out.Injections = append(out.Injections, Injection{
						Consumer: class.qual, Type: typ, Qualifier: qual,
						File: path, Line: lineAt(src, p.StartByte()),
					})
				}
			}
		}
	case "class_declaration", "interface_declaration":
		// nested types ignored for MVP
	}
}

type annotation struct {
	Name string
	Raw  string
	Args string
}

func annotationsOf(n *tree_sitter.Node, src []byte) []annotation {
	var mods *tree_sitter.Node
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Kind() == "modifiers" {
			mods = c
			break
		}
	}
	// parameters may have marker_annotation directly as children
	var out []annotation
	collectAnns := func(node *tree_sitter.Node) {
		if node == nil {
			return
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			c := node.NamedChild(i)
			if c == nil {
				continue
			}
			switch c.Kind() {
			case "marker_annotation", "annotation":
				name := ""
				if id := firstIdent(c); id != nil {
					name = nodeText(id, src)
				}
				out = append(out, annotation{Name: name, Raw: nodeText(c, src), Args: argsOf(c, src)})
			}
		}
	}
	collectAnns(mods)
	if n.Kind() == "formal_parameter" {
		collectAnns(n)
	}
	return out
}

func firstIdent(n *tree_sitter.Node) *tree_sitter.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Kind() == "identifier" {
			return c
		}
	}
	return nil
}

func argsOf(n *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Kind() == "annotation_argument_list" {
			return nodeText(c, src)
		}
	}
	return ""
}

func firstStringArg(a annotation) string {
	s := a.Args
	i := strings.Index(s, "\"")
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i+1:], "\"")
	if j < 0 {
		return ""
	}
	return s[i+1 : i+1+j]
}

func firstPath(a annotation) string {
	return firstStringArg(a)
}

func httpMethodOf(name string, a annotation) string {
	switch name {
	case "GetMapping":
		return "GET"
	case "PostMapping":
		return "POST"
	case "PutMapping":
		return "PUT"
	case "DeleteMapping":
		return "DELETE"
	case "PatchMapping":
		return "PATCH"
	case "RequestMapping":
		// look for method=RequestMethod.GET etc.
		args := strings.ToUpper(a.Args)
		for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
			if strings.Contains(args, m) {
				return m
			}
		}
		return "GET"
	}
	return "GET"
}

func joinRoute(base, leaf string) string {
	base = strings.TrimSuffix(base, "/")
	if leaf == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	if !strings.HasPrefix(leaf, "/") {
		leaf = "/" + leaf
	}
	if base == "" {
		return leaf
	}
	return base + leaf
}

func isBeanStereo(s []string) bool {
	for _, x := range s {
		switch x {
		case "Service", "Component", "Repository", "Controller", "RestController":
			return true
		}
	}
	return false
}

func isSpringDataRepo(ext string) bool {
	simple := ext
	if i := strings.Index(ext, "<"); i >= 0 {
		simple = ext[:i]
	}
	if i := strings.LastIndex(simple, "."); i >= 0 {
		simple = simple[i+1:]
	}
	switch simple {
	case "JpaRepository", "CrudRepository", "PagingAndSortingRepository", "Repository", "ListCrudRepository":
		return true
	}
	return strings.Contains(ext, "JpaRepository") || strings.Contains(ext, "CrudRepository")
}

func typeNamesFrom(n *tree_sitter.Node, src []byte, kinds ...string) []string {
	want := map[string]struct{}{}
	for _, k := range kinds {
		want[k] = struct{}{}
	}
	var out []string
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c == nil {
			continue
		}
		if _, ok := want[c.Kind()]; !ok {
			// ChildByFieldName aliases
			fn := ""
			_ = fn
			continue
		}
		out = append(out, collectTypes(c, src)...)
	}
	for _, field := range []string{"interfaces", "superclass"} {
		if f := n.ChildByFieldName(field); f != nil {
			out = append(out, collectTypes(f, src)...)
		}
	}
	return unique(out)
}

func collectTypes(n *tree_sitter.Node, src []byte) []string {
	var out []string
	var walk func(*tree_sitter.Node)
	walk = func(x *tree_sitter.Node) {
		if x == nil {
			return
		}
		switch x.Kind() {
		case "type_identifier":
			out = append(out, nodeText(x, src))
			return
		case "generic_type":
			// Keep "JpaRepository" as the extends name; also store full text simplified.
			out = append(out, simplifyType(nodeText(x, src)))
			return
		case "scoped_type_identifier":
			out = append(out, simplifyType(nodeText(x, src)))
			return
		}
		for i := uint(0); i < x.NamedChildCount(); i++ {
			walk(x.NamedChild(i))
		}
	}
	walk(n)
	return out
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func paramType(p *tree_sitter.Node, src []byte) string {
	if t := p.ChildByFieldName("type"); t != nil {
		return simplifyType(nodeText(t, src))
	}
	for i := uint(0); i < p.NamedChildCount(); i++ {
		c := p.NamedChild(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "type_identifier", "generic_type", "scoped_type_identifier", "integral_type":
			return simplifyType(nodeText(c, src))
		}
	}
	return ""
}

func simplifyType(t string) string {
	t = strings.TrimSpace(t)
	if i := strings.IndexByte(t, '<'); i >= 0 {
		t = t[:i]
	}
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		t = t[i+1:]
	}
	return t
}

func decapitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func nodeText(n *tree_sitter.Node, src []byte) string {
	return string(src[n.StartByte():n.EndByte()])
}

func lineAt(src []byte, off uint) int {
	line := 1
	for i := uint(0); i < off && i < uint(len(src)); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}
