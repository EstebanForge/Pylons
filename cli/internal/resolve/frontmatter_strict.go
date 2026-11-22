// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// TemplateKeys maps each ledger doc type to the top-level
// frontmatter keys its parser knows. The shipped templates are the
// declared schema: a key is known exactly when a template declares it.
var TemplateKeys = map[string][]string{
	"audit":         {"title", "type", "slug", "status", "remediation", "reaudit", "created", "updated", "owners", "ticket", "scope", "criteria", "artifact", "components", "description", "tags"},
	"convention":    {"title", "type", "slug", "level", "status", "created", "updated", "owners", "ticket", "applies_to", "supersedes", "artifact", "description", "tags"},
	"decision":      {"title", "type", "status", "created", "updated", "owners", "ticket", "supersedes", "superseded_by", "artifact", "links", "description", "tags"},
	"domain-entity": {"title", "type", "status", "created", "updated", "owners", "components", "source_of_truth", "relationships", "states", "states_symbol", "machines", "avoid", "description", "tags"},
	"investigation": {"title", "type", "status", "severity", "reproducible", "created", "updated", "owners", "ticket", "components", "links", "description", "tags"},
	"package":       {"title", "type", "status", "created", "updated", "owners", "ticket", "package", "repo", "ref", "tier", "rollout", "depends_on", "exposes", "protected", "description", "tags"},
	"plan":          {"title", "type", "status", "created", "updated", "owners", "ticket", "artifact", "target_release", "components", "links", "description", "slice", "scenarios", "tags", "contract"},
	"quirk":         {"title", "type", "status", "created", "updated", "owners", "package", "sources", "contracts", "guard", "guardrail", "ai_authored_contributed", "volume_pressure", "tags"},
	"reference":     {"title", "type", "status", "created", "updated", "owners", "sources", "description", "tags"},
	"ticket":        {"title", "type", "kind", "status", "priority", "created", "updated", "owners", "components", "plan", "description", "tags"},
}

// Closed payload shapes: the checks join on these keys, so an unknown
// key inside one fails closed instead of silently dropping a check.
var closedMachinesKeys = []string{"machine", "states", "initial", "terminal", "transitions", "states_symbol", "transitions_symbol", "on_unevaluated", "persisted_key", "verified_by"}
var closedTransitionKeys = []string{"from", "event", "to"}
var closedContractEntryKeys = []string{"kind", "id", "wrong_if", "verified_by", "falsified_by", "external"}
var closedScenarioKeys = []string{"given", "when", "expect", "covers"}

// UnionKeys returns every known frontmatter key across all doc types.
// Documents with an unrecognized type fall back to it so a new type
// does not mass-fire unknown-key advisories.
func UnionKeys() map[string]bool {
	out := map[string]bool{}
	for _, keys := range TemplateKeys {
		for _, k := range keys {
			out[k] = true
		}
	}
	return out
}

// engineReadKeys are read on every doc by the graph rules regardless
// of type, so no type may report them unknown: the PYLON064 fix hint
// would push authors into removing keys that cure PYLON004 orphans.
var engineReadKeys = []string{"links", "supersedes"}

func knownKeySet(docType string) map[string]bool {
	out := make(map[string]bool, len(engineReadKeys)+8)
	if typeKeys, ok := TemplateKeys[docType]; ok {
		for _, k := range typeKeys {
			out[k] = true
		}
	} else {
		// A custom or legacy doc type falls back to the union of
		// known keys: only keys unknown everywhere are advisory, so
		// an unrecognized type cannot mass-fire on title or type.
		for k := range UnionKeys() {
			out[k] = true
		}
	}
	for _, k := range engineReadKeys {
		out[k] = true
	}
	return out
}

// strictFrontMatter parses frontmatter node-first. Anchors, aliases,
// merge keys, and custom tags are rejected with line numbers;
// type-aware unknown top-level keys are collected into out.Unknown for
// the advisory rule; unknown keys inside the closed payloads
// (machines, transitions, contract entries, scenarios) fail closed.
func strictFrontMatter(content string, out *DocMeta) bool {
	front := ExtractFrontMatter(content)
	if front == "" {
		return false
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(front), &root); err != nil {
		out.ParseErrors = append(out.ParseErrors, err.Error())
		return false
	}
	if len(root.Content) == 0 {
		out.ParseErrors = append(out.ParseErrors, "frontmatter is empty")
		return false
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		out.ParseErrors = append(out.ParseErrors, fmt.Sprintf("frontmatter must be a mapping (line %d)", fmLine(doc)))
		return false
	}
	var structural []string
	walkStructural(doc, "frontmatter", &structural)
	if len(structural) > 0 {
		out.ParseErrors = append(out.ParseErrors, structural...)
		return false
	}
	docType := scalarValue(doc, "type")
	known := knownKeySet(docType)
	for _, k := range mappingKeys(doc) {
		if !known[k] {
			out.Unknown = append(out.Unknown, k)
		}
	}
	if errs := closedPayloadErrors(doc, docType); len(errs) > 0 {
		out.ParseErrors = append(out.ParseErrors, errs...)
		return false
	}
	if err := doc.Decode(out); err != nil {
		out.ParseErrors = append(out.ParseErrors, err.Error())
		return false
	}
	return true
}

// walkStructural rejects anchors, aliases, merge keys, and custom
// tags anywhere in the frontmatter tree, each with its file line
// (yaml node lines are frontmatter-relative; the fence sits at line 1).
func walkStructural(n *yaml.Node, path string, errs *[]string) {
	if n == nil {
		return
	}
	if n.Tag != "" && strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") {
		*errs = append(*errs, fmt.Sprintf("%s: custom tag %s at line %d", path, n.Tag, fmLine(n)))
		return
	}
	if n.Kind == yaml.AliasNode {
		*errs = append(*errs, fmt.Sprintf("%s: alias at line %d", path, fmLine(n)))
		return
	}
	if n.Anchor != "" {
		*errs = append(*errs, fmt.Sprintf("%s: anchor at line %d", path, fmLine(n)))
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			walkStructural(n.Content[0], path, errs)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			if key.Tag == "!!merge" {
				*errs = append(*errs, fmt.Sprintf("%s: merge key at line %d", path, fmLine(key)))
				continue
			}
			walkStructural(key, path, errs)
			walkStructural(val, path+"."+key.Value, errs)
		}
	case yaml.SequenceNode:
		for i, item := range n.Content {
			walkStructural(item, fmt.Sprintf("%s[%d]", path, i), errs)
		}
	}
}

// fmLine converts a frontmatter-relative yaml node line to a
// file-relative line: the opening fence occupies file line 1.
func fmLine(n *yaml.Node) int {
	if n == nil {
		return 0
	}
	return n.Line + 1
}

// closedPayloadErrors walks the payloads the checks join on and
// reports unknown keys with their lines.
func closedPayloadErrors(doc *yaml.Node, docType string) []string {
	var errs []string
	machines := sequenceAt(doc, "machines")
	for i, item := range machines {
		entry := mappingEntry(item, closedMachinesKeys, fmt.Sprintf("machines[%d]", i), &errs)
		transitions := sequenceAt(entry, "transitions")
		for j, tr := range transitions {
			mappingEntry(tr, closedTransitionKeys, fmt.Sprintf("machines[%d].transitions[%d]", i, j), &errs)
		}
	}
	if docType != "plan" {
		return errs
	}
	if contract := mappingAt(doc, "contract"); contract != nil {
		stable := sequenceAt(contract, "stable")
		for i, item := range stable {
			mappingEntry(item, closedContractEntryKeys, fmt.Sprintf("contract.stable[%d]", i), &errs)
		}
	}
	scenarios := sequenceAt(doc, "scenarios")
	for i, item := range scenarios {
		mappingEntry(item, closedScenarioKeys, fmt.Sprintf("scenarios[%d]", i), &errs)
	}
	return errs
}

// mappingEntry checks one closed mapping and returns the node for
// further descent; a non-mapping item is reported.
func mappingEntry(item *yaml.Node, closed []string, path string, errs *[]string) *yaml.Node {
	if item == nil || item.Kind != yaml.MappingNode {
		*errs = append(*errs, fmt.Sprintf("%s: must be a mapping (line %d)", path, fmLine(item)))
		return nil
	}
	known := map[string]bool{}
	for _, k := range closed {
		known[k] = true
	}
	for _, k := range mappingKeys(item) {
		if !known[k] {
			*errs = append(*errs, fmt.Sprintf("%s: unknown key %s (line %d)", path, k, fmLine(item)))
		}
	}
	return item
}

func mappingKeys(doc *yaml.Node) []string {
	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(doc.Content); i += 2 {
		out = append(out, doc.Content[i].Value)
	}
	return out
}

func scalarValue(doc *yaml.Node, key string) string {
	if doc == nil || doc.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == key && doc.Content[i+1].Kind == yaml.ScalarNode {
			return doc.Content[i+1].Value
		}
	}
	return ""
}

func sequenceAt(doc *yaml.Node, key string) []*yaml.Node {
	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == key && doc.Content[i+1].Kind == yaml.SequenceNode {
			return doc.Content[i+1].Content
		}
	}
	return nil
}

func mappingAt(doc *yaml.Node, key string) *yaml.Node {
	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == key && doc.Content[i+1].Kind == yaml.MappingNode {
			return doc.Content[i+1]
		}
	}
	return nil
}
