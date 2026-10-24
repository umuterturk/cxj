package config

import (
	"regexp"
	"strings"
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

var (
	langOnce sync.Once
	lang     *tree_sitter.Language
	valueRE  = regexp.MustCompile(`\$\{([^}:]+)(?::[^}]*)?\}`)
)

func language() *tree_sitter.Language {
	langOnce.Do(func() { lang = tree_sitter.NewLanguage(tsjava.Language()) })
	return lang
}

// BindingExtractor finds @Value / @ConfigurationProperties / @ConditionalOnProperty links.
type BindingExtractor struct {
	parser *tree_sitter.Parser
}

// NewBindingExtractor constructs a reusable extractor.
func NewBindingExtractor() *BindingExtractor {
	p := tree_sitter.NewParser()
	p.SetLanguage(language())
	return &BindingExtractor{parser: p}
}

// Close releases C resources.
func (e *BindingExtractor) Close() {
	if e.parser != nil {
		e.parser.Close()
		e.parser = nil
	}
}

// ExtractBindings scans Java source for config bindings.
func (e *BindingExtractor) ExtractBindings(path string, src []byte) []Binding {
	tree := e.parser.Parse(src, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()
	var out []Binding
	walkBindings(tree.RootNode(), src, path, "", &out)
	return out
}

func walkBindings(n *tree_sitter.Node, src []byte, path, typeName string, out *[]Binding) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "class_declaration", "interface_declaration", "record_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = firstIdent(n)
		}
		name := ""
		if nameNode != nil {
			name = nodeText(nameNode, src)
		}
		anns := annotations(n, src)
		prefix := ""
		for _, a := range anns {
			switch a.name {
			case "ConfigurationProperties":
				prefix = annotationString(a, "prefix")
				if prefix == "" {
					prefix = a.firstString
				}
				*out = append(*out, Binding{
					Key: prefix, Symbol: name, Path: path,
					Line: lineAt(src, nameNode.StartByte()), Kind: "configuration_properties",
				})
			case "ConditionalOnProperty":
				p := annotationString(a, "prefix")
				nm := annotationString(a, "name")
				key := joinKey(p, nm)
				if key == "" {
					key = a.firstString
				}
				*out = append(*out, Binding{
					Key: key, Symbol: name, Path: path,
					Line: lineAt(src, nameNode.StartByte()), Kind: "conditional_on_property",
				})
			}
		}
		// Record components / fields with @Value
		body := n.ChildByFieldName("body")
		if body != nil {
			for i := uint(0); i < body.NamedChildCount(); i++ {
				walkMemberBindings(body.NamedChild(i), src, path, name, prefix, out)
			}
		}
		// record header components are children of record_declaration
		for i := uint(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c != nil && c.Kind() == "formal_parameters" {
				for j := uint(0); j < c.NamedChildCount(); j++ {
					p := c.NamedChild(j)
					if p == nil || p.Kind() != "formal_parameter" {
						continue
					}
					pname := ""
					if pn := p.ChildByFieldName("name"); pn != nil {
						pname = nodeText(pn, src)
					}
					if prefix != "" && pname != "" {
						*out = append(*out, Binding{
							Key: joinKey(prefix, camelToRelaxed(pname)), Symbol: name + "." + pname,
							Path: path, Line: lineAt(src, p.StartByte()), Kind: "configuration_properties",
						})
						*out = append(*out, Binding{
							Key: joinKey(prefix, pname), Symbol: name + "." + pname,
							Path: path, Line: lineAt(src, p.StartByte()), Kind: "configuration_properties",
						})
					}
				}
			}
		}
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		walkBindings(n.NamedChild(i), src, path, typeName, out)
	}
}

func walkMemberBindings(n *tree_sitter.Node, src []byte, path, typeName, prefix string, out *[]Binding) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case "field_declaration", "method_declaration", "constructor_declaration":
		anns := annotations(n, src)
		for _, a := range anns {
			if a.name != "Value" {
				continue
			}
			raw := a.firstString
			for _, key := range placeholderKeys(raw) {
				*out = append(*out, Binding{
					Key: key, Symbol: typeName, Path: path,
					Line: lineAt(src, n.StartByte()), Kind: "value",
				})
			}
		}
		if prefix != "" && n.Kind() == "field_declaration" {
			for i := uint(0); i < n.NamedChildCount(); i++ {
				c := n.NamedChild(i)
				if c == nil || c.Kind() != "variable_declarator" {
					continue
				}
				if name := c.ChildByFieldName("name"); name != nil {
					fname := nodeText(name, src)
					*out = append(*out, Binding{
						Key: joinKey(prefix, camelToRelaxed(fname)), Symbol: typeName + "." + fname,
						Path: path, Line: lineAt(src, name.StartByte()), Kind: "configuration_properties",
					})
				}
			}
		}
	}
}

type ann struct {
	name        string
	raw         string
	firstString string
}

func annotations(n *tree_sitter.Node, src []byte) []ann {
	var mods *tree_sitter.Node
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Kind() == "modifiers" {
			mods = c
			break
		}
	}
	if mods == nil {
		return nil
	}
	var out []ann
	for i := uint(0); i < mods.NamedChildCount(); i++ {
		c := mods.NamedChild(i)
		if c == nil {
			continue
		}
		if c.Kind() != "annotation" && c.Kind() != "marker_annotation" {
			continue
		}
		name := ""
		if id := firstIdent(c); id != nil {
			name = nodeText(id, src)
		}
		raw := nodeText(c, src)
		out = append(out, ann{name: name, raw: raw, firstString: firstQuoted(raw)})
	}
	return out
}

func annotationString(a ann, key string) string {
	// prefix = "billing" or prefix="billing"
	re := regexp.MustCompile(key + `\s*=\s*"([^"]+)"`)
	if m := re.FindStringSubmatch(a.raw); len(m) == 2 {
		return m[1]
	}
	return ""
}

func firstQuoted(s string) string {
	i := strings.Index(s, `"`)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i+1:], `"`)
	if j < 0 {
		return ""
	}
	return s[i+1 : i+1+j]
}

func placeholderKeys(s string) []string {
	ms := valueRE.FindAllStringSubmatch(s, -1)
	var out []string
	for _, m := range ms {
		if len(m) > 1 {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	if len(out) == 0 && s != "" && !strings.Contains(s, "${") {
		out = append(out, s)
	}
	return out
}

func joinKey(prefix, name string) string {
	prefix = strings.TrimSpace(prefix)
	name = strings.TrimSpace(name)
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

// camelToRelaxed converts maxRetries -> max-retries (Spring relaxed form).
func camelToRelaxed(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// RelaxedForms returns equivalent Spring relaxed binding keys.
func RelaxedForms(key string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(k string) {
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	add(key)
	add(strings.ReplaceAll(key, "-", "."))
	add(strings.ReplaceAll(key, "-", ""))
	add(strings.ToLower(key))
	// kebab <-> camel per segment
	parts := strings.Split(key, ".")
	var camelParts []string
	var kebabParts []string
	for _, p := range parts {
		camelParts = append(camelParts, kebabToCamel(p))
		kebabParts = append(kebabParts, camelToRelaxed(p))
	}
	add(strings.Join(camelParts, "."))
	add(strings.Join(kebabParts, "."))
	return out
}

func kebabToCamel(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		return s
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
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
