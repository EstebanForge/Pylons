// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"bytes"
	"encoding/json"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// Canonical homes for every word the vocabulary mirror carries. The
// mirror (manifests/vocabulary.json) is generated from these tables by
// `pylon manifest vocabulary`; the binary stays the enforced truth and
// the pre-push gate fails when the mirror drifts from here.

// protectedOwnerSeverities declares the protected: owner vocabulary
// with the severity PYLON029 applies; the check consults this table.
var protectedOwnerSeverities = []ownerSeverity{
	{"release-bot", "forced"},
	{"vendored", "forced"},
	{"generated", "forced"},
	{"human-only", "forced"},
	{"ask-first", "advisory"},
}

type ownerSeverity struct {
	Owner    string
	Severity string
}

// ProtectedOwnerSeverity returns the PYLON029 severity for a protected:
// owner; unknown owners fail forced like the known non-advisory ones.
func ProtectedOwnerSeverity(owner string) string {
	for _, os := range protectedOwnerSeverities {
		if os.Owner == owner {
			return os.Severity
		}
	}
	return "forced"
}

// closedValueSets declares every closed field value set. Enforced sets
// name their rule id and mirror a compiled token map (aiAuthoredTokens,
// volumePressureTokens, quirkGuards); the rest are declared prose
// vocabulary the engine does not yet enforce.
var closedValueSets = []closedValueSet{
	{Name: "level", Documents: []string{"conventions"}, Values: []string{"MUST", "SHOULD"}},
	{Name: "ai_authored_contributed", Documents: []string{"quirks"}, Values: []string{"yes", "no", "unknown"}, EnforcedBy: "PYLON041"},
	{Name: "volume_pressure", Documents: []string{"quirks"}, Values: []string{"review", "test", "rollout", "observability", "none"}, EnforcedBy: "PYLON041"},
	{Name: "remediation", Documents: []string{"investigations"}, Values: []string{"open", "in-progress", "complete", "rejected"}},
	{Name: "severity", Documents: []string{"investigations"}, Values: []string{"critical", "high", "medium", "low"}},
	{Name: "reproducible", Documents: []string{"investigations"}, Values: []string{"always", "intermittent", "environmental"}},
	{Name: "rollout", Documents: []string{"packages"}, Values: []string{"development", "production"}},
	{Name: "guard", Documents: []string{"quirks"}, Values: []string{"lint-rule", "convention", "test", "accepted-risk"}, EnforcedBy: "PYLON028"},
}

type closedValueSet struct {
	Name       string   `json:"-"`
	Documents  []string `json:"documents"`
	Values     []string `json:"values"`
	EnforcedBy string   `json:"-"`
}

// statusZoneOrder fixes the mirror's zone emission order; maps would
// otherwise marshal alphabetically and churn the file on regen.
var statusZoneOrder = []string{"proposals", "plans", "conventions", "decisions", "investigations", "packages", "references", "quirks", "domain", "tickets"}

const (
	vocabularyID          = "https://pylons.local/schemas/vocabulary.json"
	vocabularyVersion     = 1
	vocabularyTitle       = "Pylons closed frontmatter vocabulary"
	vocabularyDescription = "The closed value sets Pylons frontmatter fields accept. Status values are enforced by the lint engine (PYLON008); the other sets are conventions, with PYLON041 validating the post-mortem tokens."
)

type vocabularyManifest struct {
	ID              string              `json:"$id"`
	Version         int                 `json:"version"`
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	Status          zoneStatusMap       `json:"status"`
	ContractKinds   kindIDForms         `json:"contract_kinds"`
	FrontmatterKeys map[string][]string `json:"frontmatter_keys"`
	ProtectedOwners ownerSeverityMap    `json:"protected_owners"`
	ClosedSets      closedSetMap        `json:"closed_sets"`
}

// zoneStatusMap marshals zones in statusZoneOrder instead of map order.
type zoneStatusMap map[string][]string

func (z zoneStatusMap) MarshalJSON() ([]byte, error) {
	return marshalOrdered(map[string][]string(z), statusZoneOrder)
}

// kindIDForms marshals kinds in the resolver's canonical order.
type kindIDForms []resolve.CoreKindDoc

func (k kindIDForms) MarshalJSON() ([]byte, error) {
	names := make([]string, len(k))
	forms := make([]string, len(k))
	for i, kd := range k {
		names[i], forms[i] = kd.Name, kd.IDForm
	}
	return marshalOrderedPairs(names, anySlice(forms))
}

// ownerSeverityMap marshals owners in declaration order.
type ownerSeverityMap []ownerSeverity

func (o ownerSeverityMap) MarshalJSON() ([]byte, error) {
	downers := make([]string, len(o))
	values := make([]string, len(o))
	for i, os := range o {
		downers[i], values[i] = os.Owner, os.Severity
	}
	return marshalOrderedPairs(downers, anySlice(values))
}

// closedSetMap marshals sets in declaration order, with a null
// enforced_by for prose-only sets.
type closedSetMap []closedValueSet

func (c closedSetMap) MarshalJSON() ([]byte, error) {
	names := make([]string, len(c))
	for i, s := range c {
		names[i] = s.Name
	}
	return marshalOrderedPairs(names, anySlice(c))
}

func (s closedValueSet) MarshalJSON() ([]byte, error) {
	enforced := any(nil)
	if s.EnforcedBy != "" {
		enforced = s.EnforcedBy
	}
	return json.Marshal(struct {
		Documents  []string `json:"documents"`
		Values     []string `json:"values"`
		EnforcedBy any      `json:"enforced_by"`
	}{s.Documents, s.Values, enforced})
}

// marshalOrdered emits a JSON object whose keys follow names order.
func marshalOrdered[V any](m map[string]V, names []string) ([]byte, error) {
	values := make([]V, len(names))
	for i, n := range names {
		values[i] = m[n]
	}
	return marshalOrderedPairs(names, anySlice(values))
}

// anySlice widens a typed slice for the pair marshaler.
func anySlice[V any](in []V) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// marshalOrderedPairs emits a JSON object from parallel key/value
// slices, preserving the given order.
func marshalOrderedPairs(names []string, values []any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalNoEscape(name)
		if err != nil {
			return nil, err
		}
		val, err := marshalNoEscape(values[i])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalNoEscape encodes any JSON value without HTML escaping; plain
// json.Marshal turns literal angle brackets in id-form docs into \u003c.
func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// BuildVocabularyManifest compiles the vocabulary mirror payload from
// the canonical tables above.
func BuildVocabularyManifest() vocabularyManifest {
	status := make(zoneStatusMap, len(allowedStatuses))
	for zone, set := range allowedStatuses {
		status[zone] = append([]string(nil), set...)
	}
	return vocabularyManifest{
		ID:              vocabularyID,
		Version:         vocabularyVersion,
		Title:           vocabularyTitle,
		Description:     vocabularyDescription,
		Status:          status,
		ContractKinds:   kindIDForms(resolve.CoreKindDocs()),
		FrontmatterKeys: resolve.TemplateKeyRegistry(),
		ProtectedOwners: ownerSeverityMap(protectedOwnerSeverities),
		ClosedSets:      closedSetMap(closedValueSets),
	}
}
