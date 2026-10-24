// Package config indexes Spring Boot YAML/properties and Java bindings.
package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Definition is one key occurrence in a config file.
type Definition struct {
	Key     string
	Value   string
	Path    string
	Line    int
	Profile string // "" = default; or explicit profile name
	Origin  string // file role hint
}

// Binding links a Java symbol to a config key.
type Binding struct {
	Key    string
	Symbol string
	Path   string
	Line   int
	Kind   string // value|configuration_properties|conditional_on_property
}

// FileFacts holds parsed config for one file.
type FileFacts struct {
	Path        string
	Definitions []Definition
	Imports     []string // classpath: relative names
}

// IsConfigFile reports whether rel should be parsed as Spring config.
func IsConfigFile(rel string) bool {
	base := path.Base(rel)
	switch {
	case base == "application.yml" || base == "application.yaml" || base == "application.properties":
		return true
	case strings.HasPrefix(base, "application-") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".properties")):
		return true
	case strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".properties"):
		// Imported or other Spring-ish resources still under resources.
		return true
	}
	return false
}

// ParseIfConfig parses rel when it looks like Spring config; returns nil if not.
func ParseIfConfig(rel string, data []byte) (*FileFacts, error) {
	if !IsConfigFile(rel) {
		return nil, nil
	}
	ff, err := ParseFile(rel, data)
	if err != nil {
		return nil, err
	}
	return &ff, nil
}

func profileFromFilename(base string) string {
	if !strings.HasPrefix(base, "application-") {
		return ""
	}
	rest := strings.TrimPrefix(base, "application-")
	for _, suf := range []string{".properties", ".yaml", ".yml"} {
		if strings.HasSuffix(rest, suf) {
			return strings.TrimSuffix(rest, suf)
		}
	}
	return ""
}

// ParseFile parses YAML or properties into definitions.
func ParseFile(rel string, data []byte) (FileFacts, error) {
	base := path.Base(rel)
	fileProfile := profileFromFilename(base)
	ff := FileFacts{Path: rel}
	switch {
	case strings.HasSuffix(base, ".properties"):
		defs, err := parseProperties(rel, data, fileProfile)
		if err != nil {
			return ff, err
		}
		ff.Definitions = defs
	case strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml"):
		defs, imports, err := parseYAML(rel, data, fileProfile)
		if err != nil {
			return ff, err
		}
		ff.Definitions = defs
		ff.Imports = imports
	}
	return ff, nil
}

func parseProperties(rel string, data []byte, fileProfile string) ([]Definition, error) {
	var out []Definition
	sc := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "!") {
			continue
		}
		i := strings.IndexAny(raw, "=:")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(raw[:i])
		val := strings.TrimSpace(raw[i+1:])
		out = append(out, Definition{
			Key: key, Value: val, Path: rel, Line: line, Profile: fileProfile,
		})
	}
	return out, sc.Err()
}

func parseYAML(rel string, data []byte, fileProfile string) ([]Definition, []string, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var all []Definition
	var imports []string
	docIdx := 0
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return all, imports, fmt.Errorf("%s: %w", rel, err)
		}
		docIdx++
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			doc = *doc.Content[0]
		}
		profile := fileProfile
		if p := activeProfile(&doc); p != "" {
			profile = p
		}
		var defs []Definition
		flattenYAML("", &doc, rel, profile, &defs)
		for _, d := range defs {
			if d.Key == "spring.config.import" {
				for _, part := range splitImport(d.Value) {
					imports = append(imports, part)
				}
			}
		}
		all = append(all, defs...)
	}
	return all, imports, nil
}

func activeProfile(n *yaml.Node) string {
	// spring.config.activate.on-profile
	if n == nil || n.Kind != yaml.MappingNode {
		return ""
	}
	m := mapping(n)
	spring, ok := m["spring"]
	if !ok || spring.Kind != yaml.MappingNode {
		return ""
	}
	sm := mapping(spring)
	cfg, ok := sm["config"]
	if !ok || cfg.Kind != yaml.MappingNode {
		return ""
	}
	cm := mapping(cfg)
	act, ok := cm["activate"]
	if !ok || act.Kind != yaml.MappingNode {
		return ""
	}
	am := mapping(act)
	if p, ok := am["on-profile"]; ok {
		return strings.TrimSpace(p.Value)
	}
	return ""
}

func mapping(n *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	if n == nil || n.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if k != nil {
			out[k.Value] = v
		}
	}
	return out
}

func flattenYAML(prefix string, n *yaml.Node, rel, profile string, out *[]Definition) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			flattenYAML(prefix, c, rel, profile, out)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			v := n.Content[i+1]
			key := k.Value
			if prefix != "" {
				key = prefix + "." + key
			}
			flattenYAML(key, v, rel, profile, out)
		}
	case yaml.SequenceNode:
		*out = append(*out, Definition{
			Key: prefix, Value: sequenceString(n), Path: rel, Line: n.Line, Profile: profile,
		})
	case yaml.ScalarNode:
		*out = append(*out, Definition{
			Key: prefix, Value: n.Value, Path: rel, Line: n.Line, Profile: profile,
		})
	}
}

func sequenceString(n *yaml.Node) string {
	var parts []string
	for _, c := range n.Content {
		parts = append(parts, c.Value)
	}
	return strings.Join(parts, ",")
}

func splitImport(v string) []string {
	v = strings.Trim(v, `"'`)
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		part = strings.TrimPrefix(part, "classpath:")
		part = strings.TrimPrefix(part, "optional:")
		part = strings.TrimPrefix(part, "classpath:")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
