// Package java extracts parser-neutral FileFacts via Tree-sitter.
package java

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"example.com/cx/internal/ir"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

var (
	langOnce sync.Once
	lang     *tree_sitter.Language
)

func language() *tree_sitter.Language {
	langOnce.Do(func() {
		lang = tree_sitter.NewLanguage(tsjava.Language())
	})
	return lang
}

// Extractor parses Java source into FileFacts. Not safe for concurrent use of one instance.
type Extractor struct {
	parser *tree_sitter.Parser
}

// NewExtractor constructs a reusable extractor. Call Close when done.
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

// Extract parses src for path (forward-slash relative) into FileFacts.
func (e *Extractor) Extract(path string, src []byte) (ir.FileFacts, error) {
	if e.parser == nil {
		return ir.FileFacts{}, fmt.Errorf("extractor closed")
	}
	tree := e.parser.Parse(src, nil)
	if tree == nil {
		return ir.FileFacts{}, fmt.Errorf("parse failed: %s", path)
	}
	defer tree.Close()

	facts := ir.FileFacts{Path: path}
	root := tree.RootNode()
	if root == nil {
		return facts, nil
	}
	facts.Package = findPackage(root, src)
	walk(root, src, path, facts.Package, nil, &facts)
	return facts, nil
}

type typeScope struct {
	name      string
	qualified string
	kind      ir.SymbolKind
	parent    *typeScope
}

func walk(n *tree_sitter.Node, src []byte, path, pkg string, scope *typeScope, facts *ir.FileFacts) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "class_declaration", "interface_declaration", "enum_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = firstChildOfKind(n, "identifier")
		}
		if nameNode == nil {
			break
		}
		name := nodeText(nameNode, src)
		kind := ir.SymbolClass
		switch n.Kind() {
		case "interface_declaration":
			kind = ir.SymbolInterface
		case "enum_declaration":
			kind = ir.SymbolEnum
		}
		qual := qualify(pkg, scope, name)
		def := ir.Definition{
			Name:      name,
			Qualified: qual,
			Kind:      kind,
			Owner:     ownerQual(scope),
			Path:      path,
			Span:      spanOf(n, src),
			Certainty: ir.CertaintyExact,
		}
		facts.Definitions = append(facts.Definitions, def)

		// implements / extends as candidate edges only
		collectTypeHeritage(n, src, path, qual, facts)

		body := n.ChildByFieldName("body")
		childScope := &typeScope{name: name, qualified: qual, kind: kind, parent: scope}
		if body != nil {
			for i := uint(0); i < body.NamedChildCount(); i++ {
				walk(body.NamedChild(i), src, path, pkg, childScope, facts)
			}
		}
		return
	case "method_declaration", "constructor_declaration":
		if scope == nil {
			break
		}
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = firstChildOfKind(n, "identifier")
		}
		if nameNode == nil {
			break
		}
		name := nodeText(nameNode, src)
		params := n.ChildByFieldName("parameters")
		sig := paramSignature(params, src)
		kind := ir.SymbolMethod
		if n.Kind() == "constructor_declaration" {
			kind = ir.SymbolConstructor
		}
		qual := scope.qualified + "." + name
		facts.Definitions = append(facts.Definitions, ir.Definition{
			Name:      name,
			Qualified: qual,
			Kind:      kind,
			Owner:     scope.qualified,
			Signature: sig,
			Path:      path,
			Span:      spanOf(nameNode, src),
			Certainty: ir.CertaintyExact,
		})
		// Walk body for invocations
		body := n.ChildByFieldName("body")
		if body != nil {
			walkInvocations(body, src, path, scope, facts)
		}
		return
	case "field_declaration":
		if scope == nil {
			break
		}
		collectFields(n, src, path, scope, facts)
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walk(n.NamedChild(i), src, path, pkg, scope, facts)
	}
}

func collectTypeHeritage(n *tree_sitter.Node, src []byte, path, qual string, facts *ir.FileFacts) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "super_interfaces", "interfaces", "superclass":
			for _, t := range typeNamesIn(c, src) {
				kind := "implements"
				if c.Kind() == "superclass" {
					kind = "extends"
				}
				facts.Edges = append(facts.Edges, ir.Edge{
					Kind:      kind,
					From:      qual,
					To:        t,
					Path:      path,
					Span:      spanOf(c, src),
					Certainty: ir.CertaintyCandidate,
				})
			}
		}
	}
}

func collectFields(n *tree_sitter.Node, src []byte, path string, scope *typeScope, facts *ir.FileFacts) {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c == nil || c.Kind() != "variable_declarator" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = firstChildOfKind(c, "identifier")
		}
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, src)
		facts.Definitions = append(facts.Definitions, ir.Definition{
			Name:      name,
			Qualified: scope.qualified + "." + name,
			Kind:      ir.SymbolField,
			Owner:     scope.qualified,
			Path:      path,
			Span:      spanOf(nameNode, src),
			Certainty: ir.CertaintyExact,
		})
	}
}

func walkInvocations(n *tree_sitter.Node, src []byte, path string, scope *typeScope, facts *ir.FileFacts) {
	if n == nil {
		return
	}
	if n.Kind() == "method_invocation" {
		extractInvocation(n, src, path, scope, facts)
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walkInvocations(n.NamedChild(i), src, path, scope, facts)
	}
}

func extractInvocation(n *tree_sitter.Node, src []byte, path string, scope *typeScope, facts *ir.FileFacts) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nodeText(nameNode, src)
	recv := ""
	if obj := n.ChildByFieldName("object"); obj != nil {
		recv = nodeText(obj, src)
	}
	occ := ir.Occurrence{
		Name:     name,
		Kind:     ir.SymbolMethod,
		Path:     path,
		Span:     spanOf(nameNode, src),
		Receiver: recv,
	}
	if recv != "" {
		occ.Qualified = recv + "." + name
	} else {
		occ.Qualified = name
	}
	facts.Occurrences = append(facts.Occurrences, occ)

	// Calls stay candidate: interface/virtual dispatch must not claim exact targets.
	to := name
	if recv != "" {
		to = recv + "." + name
	}
	facts.Edges = append(facts.Edges, ir.Edge{
		Kind:      "calls",
		From:      scope.qualified,
		To:        to,
		Path:      path,
		Span:      spanOf(nameNode, src),
		Certainty: ir.CertaintyCandidate,
	})
}

func findPackage(root *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c != nil && c.Kind() == "package_declaration" {
			text := strings.TrimSpace(strings.TrimPrefix(nodeText(c, src), "package"))
			return strings.TrimSuffix(text, ";")
		}
	}
	return ""
}

func qualify(pkg string, scope *typeScope, name string) string {
	if scope != nil {
		return scope.qualified + "." + name
	}
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}

func ownerQual(scope *typeScope) string {
	if scope == nil {
		return ""
	}
	return scope.qualified
}

func paramSignature(params *tree_sitter.Node, src []byte) string {
	if params == nil {
		return "()"
	}
	var parts []string
	for i := uint(0); i < params.NamedChildCount(); i++ {
		p := params.NamedChild(i)
		if p == nil || p.Kind() != "formal_parameter" {
			continue
		}
		parts = append(parts, typeNameOfParam(p, src))
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func typeNameOfParam(p *tree_sitter.Node, src []byte) string {
	for i := uint(0); i < p.NamedChildCount(); i++ {
		c := p.NamedChild(i)
		if c == nil {
			continue
		}
		switch c.Kind() {
		case "identifier":
			continue
		case "integral_type", "floating_point_type", "boolean_type", "void_type",
			"type_identifier", "scoped_type_identifier", "generic_type", "array_type":
			return simplifyType(nodeText(c, src))
		}
	}
	return "?"
}

func typeNamesIn(n *tree_sitter.Node, src []byte) []string {
	var out []string
	var walkTypes func(*tree_sitter.Node)
	walkTypes = func(x *tree_sitter.Node) {
		if x == nil {
			return
		}
		switch x.Kind() {
		case "type_identifier", "scoped_type_identifier", "generic_type":
			out = append(out, simplifyType(nodeText(x, src)))
			return
		}
		for i := uint(0); i < x.NamedChildCount(); i++ {
			walkTypes(x.NamedChild(i))
		}
	}
	walkTypes(n)
	return out
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

func spanOf(n *tree_sitter.Node, src []byte) ir.Span {
	start := n.StartByte()
	return ir.Span{
		Line:      lineAt(src, start),
		StartByte: start,
		EndByte:   n.EndByte(),
	}
}

func lineAt(src []byte, byteOffset uint) int {
	if int(byteOffset) > len(src) {
		byteOffset = uint(len(src))
	}
	line := 1
	for i := uint(0); i < byteOffset; {
		r, size := utf8.DecodeRune(src[i:])
		if size == 0 {
			break
		}
		if r == '\n' {
			line++
		}
		i += uint(size)
	}
	return line
}

func nodeText(n *tree_sitter.Node, src []byte) string {
	return string(src[n.StartByte():n.EndByte()])
}

func firstChildOfKind(n *tree_sitter.Node, kind string) *tree_sitter.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Kind() == kind {
			return c
		}
	}
	return nil
}
