// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// StringOrList accepts a YAML scalar or sequence of scalars.
type StringOrList []string

// UnmarshalYAML implements scalar-or-sequence decoding.
func (s *StringOrList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			*s = nil
			return nil
		}
		*s = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := node.Decode(&out); err != nil {
			return err
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("expected a string or a list of strings")
	}
}

// NonEmpty reports whether any entry contains non-blank text.
func (s StringOrList) NonEmpty() bool {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// DocMeta holds common frontmatter fields shared across Pylons document types.
type DocMeta struct {
	Title      string       `yaml:"title"`
	Type       string       `yaml:"type"`
	Slug       string       `yaml:"slug"`
	Status     string       `yaml:"status"`
	Created    string       `yaml:"created"`
	Updated    string       `yaml:"updated"`
	Owners     []string     `yaml:"owners"`
	Components []string     `yaml:"components"`
	Package    string       `yaml:"package"`
	Links      []string     `yaml:"links"`
	Tags       []string     `yaml:"tags"`
	Supersedes []string     `yaml:"supersedes"`
	Artifact   StringOrList `yaml:"artifact"`
	Reaudit    string       `yaml:"reaudit"`
	Guard      string       `yaml:"guard"`
	Sources    []string     `yaml:"sources"`
	Ticket     []any        `yaml:"ticket"`
}

// PlanFrontMatter holds plan-specific frontmatter fields.
type PlanFrontMatter struct {
	DocMeta  `yaml:",inline"`
	Contract struct {
		Stable   []map[string]any `yaml:"stable"`
		Unstable []any            `yaml:"unstable"`
		Editing  []any            `yaml:"editing"`
	} `yaml:"contract"`
}

// PackageFrontMatter holds package-specific frontmatter fields.
type PackageFrontMatter struct {
	DocMeta   `yaml:",inline"`
	Ref       string           `yaml:"ref"`
	DependsOn []string         `yaml:"depends_on"`
	Exposes   []map[string]any `yaml:"exposes"`
	Protected []any            `yaml:"protected"`
}

// normalizeDoc normalizes a BOM prefix and CRLF line endings so fence
// detection works on Windows checkouts (autocrlf) too.
func normalizeDoc(content string) string {
	content = strings.TrimPrefix(content, "\uFEFF")
	return strings.ReplaceAll(content, "\r\n", "\n")
}

// ExtractFrontMatter returns the YAML text between leading `---` fences.
func ExtractFrontMatter(content string) string {
	content = normalizeDoc(content)
	const fence = "---"
	if !strings.HasPrefix(content, fence+"\n") && content != fence {
		return ""
	}
	rest := strings.TrimPrefix(content, fence+"\n")
	if content == fence {
		return ""
	}
	if front, _, ok := strings.Cut(rest, "\n"+fence+"\n"); ok {
		return front
	}
	if strings.HasSuffix(rest, "\n"+fence) {
		return rest[:len(rest)-len("\n"+fence)]
	}
	return ""
}

// WithoutFrontMatter returns content with its leading YAML frontmatter removed.
// The body is returned with normalized line endings.
func WithoutFrontMatter(content string) string {
	content = normalizeDoc(content)
	const fence = "---"
	if !strings.HasPrefix(content, fence+"\n") && content != fence {
		return content
	}
	if content == fence {
		return ""
	}
	rest := strings.TrimPrefix(content, fence+"\n")
	if _, body, ok := strings.Cut(rest, "\n"+fence+"\n"); ok {
		return body
	}
	if strings.HasSuffix(rest, "\n"+fence) {
		return ""
	}
	return content
}

// ParseDocMeta extracts common frontmatter fields into out.
func ParseDocMeta(content string, out *DocMeta) bool {
	front := ExtractFrontMatter(content)
	if front == "" {
		return false
	}
	return yaml.Unmarshal([]byte(front), out) == nil
}

// ParsePlanFrontMatter extracts plan frontmatter into out.
func ParsePlanFrontMatter(content string, out *PlanFrontMatter) bool {
	front := ExtractFrontMatter(content)
	if front == "" {
		return false
	}
	return yaml.Unmarshal([]byte(front), out) == nil
}

// ParsePackageFrontMatter extracts package frontmatter into out.
func ParsePackageFrontMatter(content string, out *PackageFrontMatter) bool {
	front := ExtractFrontMatter(content)
	if front == "" {
		return false
	}
	return yaml.Unmarshal([]byte(front), out) == nil
}
