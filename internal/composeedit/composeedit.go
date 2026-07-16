// Package composeedit surgically edits docker-compose.yml using yaml.Node
// so mcsync never resorts to text/regex replacement on the manifest. Only
// the MODRINTH_PROJECTS / CURSEFORGE_FILES lists under services.mc.environment
// are touched; everything else in the file (comments, ordering, unrelated
// keys) is preserved as-is.
package composeedit

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is a parsed docker-compose.yml held as a yaml.Node tree.
type Document struct {
	root *yaml.Node
	path string
}

// Load reads and parses the docker-compose.yml at path.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &Document{root: &root, path: path}, nil
}

// Save writes the document back to its original path.
func (d *Document) Save() error {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d.root); err != nil {
		return fmt.Errorf("encoding %s: %w", d.path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encoding %s: %w", d.path, err)
	}
	return os.WriteFile(d.path, []byte(buf.String()), 0o644)
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapAppend(m *yaml.Node, key string, value *yaml.Node) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	m.Content = append(m.Content, keyNode, value)
}

// environmentNode navigates services.mc.environment, creating the
// environment mapping if the service exists but has none yet.
func (d *Document) environmentNode() (*yaml.Node, error) {
	if d.root.Kind != yaml.DocumentNode || len(d.root.Content) == 0 {
		return nil, fmt.Errorf("%s is empty or not a valid YAML document", d.path)
	}
	top := d.root.Content[0]
	services := mapGet(top, "services")
	if services == nil {
		return nil, fmt.Errorf("%s has no top-level 'services' key", d.path)
	}
	mc := mapGet(services, "mc")
	if mc == nil {
		return nil, fmt.Errorf("%s has no 'services.mc' entry", d.path)
	}
	env := mapGet(mc, "environment")
	if env == nil {
		env = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mapAppend(mc, "environment", env)
	}
	return env, nil
}

// AddModrinthProject appends slug to MODRINTH_PROJECTS in
// services.mc.environment. added is false (with no error) if slug is
// already listed.
func (d *Document) AddModrinthProject(slug string) (added bool, err error) {
	return d.addToBlockList("MODRINTH_PROJECTS", slug)
}

// AddCurseforgeFile appends slug to CURSEFORGE_FILES in
// services.mc.environment. added is false (with no error) if slug is
// already listed.
func (d *Document) AddCurseforgeFile(slug string) (added bool, err error) {
	return d.addToBlockList("CURSEFORGE_FILES", slug)
}

func (d *Document) addToBlockList(envKey, entry string) (bool, error) {
	env, err := d.environmentNode()
	if err != nil {
		return false, err
	}
	existing := mapGet(env, envKey)
	var lines []string
	if existing != nil {
		for _, line := range strings.Split(existing.Value, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if line == entry {
				return false, nil
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, entry)
	value := strings.Join(lines, "\n") + "\n"
	if existing != nil {
		existing.Value = value
		existing.Style = yaml.LiteralStyle
		existing.Tag = "!!str"
	} else {
		mapAppend(env, envKey, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: value,
			Style: yaml.LiteralStyle,
		})
	}
	return true, nil
}
