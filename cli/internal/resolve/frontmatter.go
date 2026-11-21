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
	Avoid      StringOrList `yaml:"avoid"`
	// State parity: the declared closed set and where the backed
	// enum lives in the mapped project.
	States       StringOrList `yaml:"states"`
	StatesSymbol string       `yaml:"states_symbol"`
	// Formal machines: one entry per lifecycle on the entity. Each
	// carries its own declaration and code pointers, so one entity can
	// hold several machines.
	Machines []MachineDecl `yaml:"machines"`
	// Post-mortem signals, human-declared; the engine never infers them.
	AIAuthoredContributed string `yaml:"ai_authored_contributed"`
	VolumePressure        string `yaml:"volume_pressure"`
}

// MachineDecl is one declared state machine on a domain entity.
// Machine ids are identities shared with the code-side marker; they
// must be unique across the whole ledger.
type MachineDecl struct {
	Machine           string           `yaml:"machine"`
	States            StringOrList     `yaml:"states"`
	Initial           StringOrList     `yaml:"initial"`
	Terminal          StringOrList     `yaml:"terminal"`
	Transitions       []TransitionDecl `yaml:"transitions"`
	StatesSymbol      string           `yaml:"states_symbol"`
	TransitionsSymbol string           `yaml:"transitions_symbol"`
	OnUnevaluated     string           `yaml:"on_unevaluated"`
	// PersistedKey names the persisted state field the machine owns.
	// When set, direct writes outside the machine surface are
	// advisory findings.
	PersistedKey string `yaml:"persisted_key"`
	// VerifiedBy points at the table tests proving purity and
	// throw-on-invalid in the mapped repo's QA suite.
	VerifiedBy StringOrList `yaml:"verified_by"`
}

// TransitionDecl is one declared (from, event, to) triple.
type TransitionDecl struct {
	From  string `yaml:"from"`
	Event string `yaml:"event"`
	To    string `yaml:"to"`
}

// PlanFrontMatter holds plan-specific frontmatter fields.
type PlanFrontMatter struct {
	DocMeta `yaml:",inline"`
	// Slice marks the opt-in vertical-slice flavor: one capability
	// built end to end as the task boundary.
	Slice bool `yaml:"slice"`
	// Scenarios are declared given/when/expect examples; covers names
	// the contract ids each example proves.
	Scenarios []map[string]any `yaml:"scenarios"`
	Contract  struct {
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
