// Package exposure builds dbt exposures YAML (schema version 2).
package exposure

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Exposure is one dbt exposure entry.
type Exposure struct {
	Name        string
	Type        string
	URL         string
	Label       string
	Description string
	OwnerName   string
	OwnerEmail  string
	DependsOn   []string
	Maturity    string   // omit empty
	Tags        []string // omit empty
}

// File is a dbt exposures YAML document (one file, many exposures).
type File struct {
	Name      string // basename without .yml
	Exposures []Exposure
}

type yamlDoc struct {
	Version   int           `yaml:"version"`
	Exposures []yamlExposure `yaml:"exposures"`
}

type yamlExposure struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	URL         string      `yaml:"url"`
	Label       string      `yaml:"label"`
	Description yamlString  `yaml:"description"`
	Owner       yamlOwner   `yaml:"owner"`
	DependsOn   []string    `yaml:"depends_on"`
	Maturity    string      `yaml:"maturity,omitempty"`
	Tags        []string    `yaml:"tags,omitempty"`
}

type yamlOwner struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// yamlString emits multiline values with literal block style (|).
type yamlString string

func (s yamlString) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(s)}
	if strings.Contains(string(s), "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n, nil
}

// Encode returns version-2 exposures YAML with exposures sorted by name.
func (f *File) Encode() ([]byte, error) {
	items := make([]yamlExposure, 0, len(f.Exposures))
	for _, e := range f.Exposures {
		items = append(items, toYAML(e))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	doc := yamlDoc{Version: 2, Exposures: items}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("exposure: encode: %w", err)
	}
	return out, nil
}

// Write writes {dir}/{Name}.yml and returns the path written.
func (f *File) Write(dir string) (string, error) {
	if f.Name == "" {
		return "", fmt.Errorf("exposure: file name is required")
	}
	raw, err := f.Encode()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, f.Name+".yml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("exposure: write %s: %w", path, err)
	}
	return path, nil
}

func toYAML(e Exposure) yamlExposure {
	depends := e.DependsOn
	if depends == nil {
		depends = []string{}
	}
	out := yamlExposure{
		Name:        e.Name,
		Type:        e.Type,
		URL:         e.URL,
		Label:       e.Label,
		Description: yamlString(e.Description),
		Owner:       yamlOwner{Name: e.OwnerName, Email: e.OwnerEmail},
		DependsOn:   depends,
		Maturity:    e.Maturity,
	}
	if len(e.Tags) > 0 {
		out.Tags = e.Tags
	}
	return out
}
