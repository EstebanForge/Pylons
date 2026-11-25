// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

func TestBuildVocabularyManifestMirrorsCompiledTables(t *testing.T) {
	m := BuildVocabularyManifest()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	status, _ := got["status"].(map[string]any)
	if len(status) != len(allowedStatuses) {
		t.Fatalf("status zones: got %d, want %d", len(status), len(allowedStatuses))
	}
	for zone, want := range allowedStatuses {
		raw, ok := status[zone].([]any)
		if !ok {
			t.Fatalf("status[%s] missing", zone)
		}
		gotSet := make([]string, 0, len(raw))
		for _, v := range raw {
			gotSet = append(gotSet, v.(string))
		}
		if !reflect.DeepEqual(gotSet, want) {
			t.Errorf("status[%s] = %v, want %v", zone, gotSet, want)
		}
	}

	kinds, _ := got["contract_kinds"].(map[string]any)
	wantDocs := resolve.CoreKindDocs()
	if len(kinds) != len(wantDocs) {
		t.Fatalf("contract_kinds: got %d, want %d", len(kinds), len(wantDocs))
	}
	for _, kd := range wantDocs {
		if kinds[kd.Name] != kd.IDForm {
			t.Errorf("contract_kinds[%s] = %v, want %q", kd.Name, kinds[kd.Name], kd.IDForm)
		}
	}

	fm, _ := got["frontmatter_keys"].(map[string]any)
	if len(fm) != len(resolve.TemplateKeyRegistry()) {
		t.Fatalf("frontmatter_keys types: got %d, want %d", len(fm), len(resolve.TemplateKeyRegistry()))
	}
	for typ, want := range resolve.TemplateKeyRegistry() {
		raw, ok := fm[typ].([]any)
		if !ok {
			t.Fatalf("frontmatter_keys[%s] missing", typ)
		}
		gotKeys := make([]string, 0, len(raw))
		for _, v := range raw {
			gotKeys = append(gotKeys, v.(string))
		}
		if !slices.Equal(gotKeys, want) {
			t.Errorf("frontmatter_keys[%s] = %v, want %v", typ, gotKeys, want)
		}
	}

	owners, _ := got["protected_owners"].(map[string]any)
	if len(owners) != len(protectedOwnerSeverities) {
		t.Fatalf("protected_owners: got %d, want %d", len(owners), len(protectedOwnerSeverities))
	}
	for _, os := range protectedOwnerSeverities {
		if owners[os.Owner] != os.Severity {
			t.Errorf("protected_owners[%s] = %v, want %q", os.Owner, owners[os.Owner], os.Severity)
		}
	}

	sets, _ := got["closed_sets"].(map[string]any)
	if len(sets) != len(closedValueSets) {
		t.Fatalf("closed_sets: got %d, want %d", len(sets), len(closedValueSets))
	}
	// A set claiming enforcement must declare exactly the values its
	// compiled token map holds, or the mirror lies in either direction.
	// The loop derives the pairs from closedValueSets itself, so a
	// future entry with EnforcedBy cannot skip the check.
	enforcedTokens := map[string]map[string]bool{
		"ai_authored_contributed": aiAuthoredTokens,
		"volume_pressure":         volumePressureTokens,
		"guard":                   quirkGuards,
	}
	for _, set := range closedValueSets {
		raw, ok := sets[set.Name].(map[string]any)
		if !ok {
			t.Fatalf("closed_sets[%s] missing", set.Name)
		}
		gotVals := map[string]bool{}
		for _, v := range raw["values"].([]any) {
			gotVals[v.(string)] = true
		}
		if set.EnforcedBy == "" {
			if raw["enforced_by"] != nil {
				t.Errorf("closed_sets[%s] declares enforcement but no token map backs it", set.Name)
			}
			continue
		}
		tokens, known := enforcedTokens[set.Name]
		if !known {
			t.Fatalf("closed_sets[%s] claims enforcement by %s but no token map backs it; add the map here", set.Name, set.EnforcedBy)
		}
		if _, inCatalog := Catalog[set.EnforcedBy]; !inCatalog {
			t.Errorf("closed_sets[%s] names rule %s which is not in the catalog", set.Name, set.EnforcedBy)
		}
		if raw["enforced_by"] == nil {
			t.Errorf("closed_sets[%s] claims no enforcement but the engine enforces it", set.Name)
		}
		if len(gotVals) != len(tokens) {
			t.Errorf("closed_sets[%s] values %v != token map %v", set.Name, gotVals, tokens)
		}
		for v := range tokens {
			if !gotVals[v] {
				t.Errorf("closed_sets[%s] missing token %q", set.Name, v)
			}
		}
	}

	again, err := json.MarshalIndent(BuildVocabularyManifest(), "", "  ")
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if string(data) != string(again) {
		t.Error("BuildVocabularyManifest is not byte-stable across calls")
	}
}

func TestCoreKindsMatchRegistry(t *testing.T) {
	// The mirror's kind list and the resolver's enforcement map must name
	// the same set, or the text lies about what the engine accepts.
	r := resolve.NewRegistryWithOptions(resolve.RegistryOptions{})
	for _, k := range resolve.CoreKinds() {
		if !r.SupportsKind(resolve.Kind(k)) {
			t.Errorf("CoreKinds lists %q but the registry does not resolve it", k)
		}
	}
}
