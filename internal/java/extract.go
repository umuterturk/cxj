// Package java extracts parser-neutral FileFacts via Tree-sitter.
package java

import (
	"fmt"
	"strings"
	"sync"

	"github.com/umuterturk/cxj/internal/ir"
	"github.com/umuterturk/cxj/internal/tshelp"
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
	// fieldTypes maps declared field names to their simple type names, for
	// receiver typing (A3). Fields declared after a method still see it: it
	// is built from the whole class body before any method is walked.
	fieldTypes map[string]string
}

// fieldType walks the type-scope chain (inner classes see outer fields) for a
// declared field's simple type name.
func (s *typeScope) fieldType(name string) string {
	for ; s != nil; s = s.parent {
		if t, ok := s.fieldTypes[name]; ok {
			return t
		}
	}
	return ""
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
			Package:   pkg,
			Path:      path,
			Span:      spanOf(n),
			Certainty: ir.CertaintyExact,
		}
		facts.Definitions = append(facts.Definitions, def)

		// implements / extends as candidate edges only
		collectTypeHeritage(n, src, path, qual, facts)

		body := n.ChildByFieldName("body")
		childScope := &typeScope{name: name, qualified: qual, kind: kind, parent: scope}
		if body != nil {
			childScope.fieldTypes = classFieldTypes(body, src)
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
		full := spanOf(n)
		full.Line = spanOf(nameNode).Line
		facts.Definitions = append(facts.Definitions, ir.Definition{
			Name:      name,
			Qualified: qual,
			Kind:      kind,
			Owner:     scope.qualified,
			Package:   pkg,
			Signature: sig,
			Path:      path,
			Span:      full,
			Certainty: ir.CertaintyExact,
		})
		// Walk body for invocations. The method's declared variable types
		// travel with the walk (A3): parameters bind up front, locals bind
		// as the walk reaches them — one traversal, and a declaration in
		// scope for everything after it, which is where Java puts them.
		vars := map[string]string{}
		if params != nil {
			collectFormalTypes(params, src, vars)
		}
		body := n.ChildByFieldName("body")
		if body != nil {
			walkInvocations(body, src, path, qual, scope, vars, facts)
		}
		return
	case "field_declaration":
		if scope == nil {
			break
		}
		collectFields(n, src, path, facts.Package, scope, facts)
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
					Span:      spanOf(c),
					Certainty: ir.CertaintyCandidate,
				})
			}
		}
	}
}

func collectFields(n *tree_sitter.Node, src []byte, path, pkg string, scope *typeScope, facts *ir.FileFacts) {
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
			Package:   pkg,
			Path:      path,
			Span:      spanOf(nameNode),
			Certainty: ir.CertaintyExact,
		})
	}
}

// walkInvocations walks a method body for calls, binding declared types as
// it passes them (A3). One traversal for both jobs: a call sees the
// parameters and every local declared before it — the scope Java gives it,
// approximated flat (see collectFormalTypes).
func walkInvocations(n *tree_sitter.Node, src []byte, path, from string, scope *typeScope, vars map[string]string, facts *ir.FileFacts) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "method_invocation":
		extractInvocation(n, src, path, from, scope, vars, facts)
	case "local_variable_declaration":
		// Bind before descending, so a call in the initializer resolves the
		// same names the two-pass version saw.
		declVarType(n, src, vars)
	case "enhanced_for_statement":
		// for (Tag tag : tags) — a typed binding like a declaration, with
		// its own node shape rather than a formal_parameter one.
		tn, nn := n.ChildByFieldName("type"), n.ChildByFieldName("name")
		if tn != nil && nn != nil {
			if t := simplifyType(nodeText(tn, src)); t != "" {
				if name := nodeText(nn, src); vars[name] == "" {
					vars[name] = t
				}
			}
		}
	case "formal_parameters":
		// Typed lambda parameters (method parameters were bound before the
		// walk started).
		collectFormalTypes(n, src, vars)
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walkInvocations(n.NamedChild(i), src, path, from, scope, vars, facts)
	}
}

func extractInvocation(n *tree_sitter.Node, src []byte, path, from string, scope *typeScope, vars map[string]string, facts *ir.FileFacts) {
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
		Span:     spanOf(nameNode),
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
		From:      from,
		To:        to,
		RecvType:  resolveReceiver(recv, scope, vars),
		Path:      path,
		Span:      spanOf(nameNode),
		Certainty: ir.CertaintyCandidate,
	})
}

// resolveReceiver types a call receiver from what the file itself declares
// (FIX_PLAN A3). Known answers: bare calls, this/super, plain identifiers
// declared as a parameter, local, or field of an enclosing type, and
// `new X()...` constructions. Unknown stays empty — chained calls
// (getSvc().pay()), qualified receivers, and lambda parameters genuinely have
// no cheap declared type, and inventing one would be the guess A3 exists to
// remove. The caller keeps matching those the old, honest-candidate way.
func resolveReceiver(recv string, scope *typeScope, vars map[string]string) string {
	switch {
	case recv == "" || recv == "this" || recv == "super":
		if scope != nil {
			return scope.name // bare call: implicit this, or inherited (subtype rule covers supers)
		}
		return ""
	case strings.HasPrefix(recv, "new "):
		t := strings.TrimSpace(strings.TrimPrefix(recv, "new"))
		if i := strings.IndexByte(t, '('); i >= 0 {
			t = t[:i]
		}
		if t = simplifyType(t); t != "" {
			return t
		}
		return ""
	case strings.HasPrefix(recv, "this."), strings.HasPrefix(recv, "super."):
		// this.field.other() → the field's type; this.method() → the object
		// itself. super.method() reads the same way: typing it to the
		// enclosing class still answers superclass queries, because a
		// subtype of the superclass is exactly what the subtype rule accepts.
		rest := recv[5:] // after "this."
		if recv[0] == 's' {
			rest = recv[6:] // after "super."
		}
		head := rest
		if i := strings.IndexAny(rest, ".("); i >= 0 {
			head = rest[:i]
		}
		if scope == nil || !isIdentifierLike(head) {
			return ""
		}
		if t := scope.fieldType(head); t != "" {
			return t
		}
		if !strings.ContainsRune(rest, '.') {
			return scope.name
		}
		return ""
	case isPlainIdentifier(recv):
		if t, ok := vars[recv]; ok && t != "" {
			return t
		}
		if scope != nil {
			if t := scope.fieldType(recv); t != "" {
				return t
			}
		}
		return "" // could be a static call on a type from another file; index completes that
	}
	return ""
}

// isPlainIdentifier accepts a lowercase-initial Java identifier (camelCase
// names like payService). A capitalized receiver is deliberately excluded:
// the extractor cannot see cross-file types, and a capitalized name might be
// a static type reference — that case is completed by the index, where the
// full type set is known, rather than guessed here.
func isPlainIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 'a' && r <= 'z':
		case i > 0 && (r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'):
		default:
			return false
		}
	}
	return !(s[0] >= 'A' && s[0] <= 'Z')
}

// isIdentifierLike: any-case Java identifier (no dots, parens, spaces).
func isIdentifierLike(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// classFieldTypes reads the declared types of a class body's fields. Done for
// the whole body before methods are walked, so a field declared after the
// method that uses it is still typed.
func classFieldTypes(body *tree_sitter.Node, src []byte) map[string]string {
	var out map[string]string
	for i := uint(0); i < body.NamedChildCount(); i++ {
		c := body.NamedChild(i)
		if c == nil || c.Kind() != "field_declaration" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		declVarType(c, src, out)
	}
	return out
}

// collectFormalTypes records parameter declared types (formal and varargs).
func collectFormalTypes(params *tree_sitter.Node, src []byte, vars map[string]string) {
	for i := uint(0); i < params.NamedChildCount(); i++ {
		p := params.NamedChild(i)
		if p == nil {
			continue
		}
		switch p.Kind() {
		case "formal_parameter", "spread_parameter":
			declVarType(p, src, vars)
		}
	}
}

// declVarType handles the shapes that declare a typed name: a
// field_declaration/local_variable_declaration (type field + declarators) and
// a formal_parameter/spread_parameter (type + name fields).
// Declarations bind first-wins: parameters are collected before the body, and
// an earlier declaration beats a later lambda parameter of the same name, so
// the most method-level reading of a name is the one that sticks.
func declVarType(n *tree_sitter.Node, src []byte, vars map[string]string) {
	typeNode := n.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	typ := simplifyType(nodeText(typeNode, src))
	if typ == "" {
		return
	}
	bind := func(name string) {
		if _, ok := vars[name]; !ok {
			vars[name] = typ
		}
	}
	if n.Kind() == "formal_parameter" || n.Kind() == "spread_parameter" {
		if nameNode := n.ChildByFieldName("name"); nameNode != nil {
			bind(nodeText(nameNode, src))
		}
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c == nil || c.Kind() != "variable_declarator" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = firstChildOfKind(c, "identifier")
		}
		if nameNode != nil {
			bind(nodeText(nameNode, src))
		}
	}
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

func spanOf(n *tree_sitter.Node) ir.Span {
	return ir.Span{
		Line:      tshelp.Line(n),
		EndLine:   tshelp.EndLine(n),
		StartByte: n.StartByte(),
		EndByte:   n.EndByte(),
	}
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
