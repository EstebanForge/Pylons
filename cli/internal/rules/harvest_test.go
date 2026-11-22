// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"
)

func harvestKB(t *testing.T) string {
	t.Helper()
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n")
	mustWrite(t, filepath.Join(kb, "packages", "core.md"), "---\ntitle: Core\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: core\ncomponents: [svc]\nexposes:\n  - kind: go-symbol\n    id: repo://svc/src/api.go::Known\n---\nThe core package.\n")
	mustWrite(t, filepath.Join(kb, "svc", "src", "api.go"), "package src\n\nfunc Known() {}\n")
	return kb
}

func harvestDiff(t *testing.T, kb string) []byte {
	t.Helper()
	diff := strings.Join([]string{
		"diff --git a/svc/src/api.go b/svc/src/api.go",
		"+++ b/svc/src/api.go",
		"@@ -1,2 +1,6 @@",
		" func Known() {}",
		"+func Other() {}",
		"+func Known() {}",
		"+func hidden() {}",
		"+func (s *Store) Load() {}",
		"+type Cache struct {}",
		"diff --git a/svc/src/api_test.go b/svc/src/api_test.go",
		"+++ b/svc/src/api_test.go",
		"@@ -0,0 +1,2 @@",
		"+func TestKnown(t *testing.T) {}",
		"diff --git a/vendor/x.go b/vendor/x.go",
		"+++ b/vendor/x.go",
		"Binary files a/vendor/x.go and b/vendor/x.go differ",
		"diff --git a/svc/src/store.rs b/svc/src/store.rs",
		"+++ b/svc/src/store.rs",
		"@@ -0,0 +1,3 @@",
		"+pub fn new_store() -> Store {",
		"+    Store::default()",
		"+}",
		"diff --git a/svc/src/config.ts b/svc/src/config.ts",
		"+++ b/svc/src/config.ts",
		"@@ -0,0 +1,1 @@",
		"+export const DEFAULT_TIMEOUT = 30;",
		"",
	}, "\n")
	return []byte(diff)
}

func TestHarvestExtractsExportedSymbols(t *testing.T) {
	kb := harvestKB(t)
	t.Chdir(kb)
	rep, err := HarvestPR(kb, harvestDiff(t, kb), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	names := map[string]bool{}
	for _, s := range rep.Symbols {
		names[s.Name] = true
	}
	for _, want := range []string{"Other", "Load", "Cache", "new_store", "DEFAULT_TIMEOUT"} {
		if !names[want] {
			t.Fatalf("missing exported symbol %s in %v", want, rep.Symbols)
		}
	}
	for _, quiet := range []string{"hidden", "TestKnown"} {
		if names[quiet] {
			t.Fatalf("non-exported or test symbol %s must stay out", quiet)
		}
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "vendor/x.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("binary path must produce a warning, got %v", rep.Warnings)
	}
}

func TestHarvestAttributesAndDedups(t *testing.T) {
	kb := harvestKB(t)
	t.Chdir(kb)
	rep, err := HarvestPR(kb, harvestDiff(t, kb), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	var proposed SymbolCandidate
	for _, s := range rep.Symbols {
		if s.Name == "Known" && s.Proposed {
			t.Fatal("already-declared symbol must not be proposed")
		}
		if s.Name == "Other" {
			proposed = s
		}
	}
	if proposed.Name == "" || !proposed.Proposed {
		t.Fatalf("Other must be a live proposal, got %+v", proposed)
	}
	if proposed.Alias != "svc" || proposed.Package != "core" {
		t.Fatalf("attribution wrong: %+v", proposed)
	}
	if proposed.ID != "repo://svc/src/api.go::Other" {
		t.Fatalf("proposal id wrong: %s", proposed.ID)
	}
	// extra.go lives in the project but no package claims a symbol-only
	// proposal target beyond core: still attributed to svc/core.
	for _, s := range rep.Symbols {
		if s.Name == "new_store" && (s.Alias != "svc" || s.Package != "core") {
			t.Fatalf("rust symbol must attribute through its project: %+v", s)
		}
	}
	if len(rep.Unattributed) != 0 {
		t.Fatalf("all diff paths are registered: %v", rep.Unattributed)
	}
}

func TestHarvestUnresolvablePathWarns(t *testing.T) {
	kb := harvestKB(t)
	diff := "+++ b/nowhere/lost.go\n@@ -0,0 +1,1 @@\n+func Stray() {}\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if len(rep.Unattributed) != 1 || rep.Unattributed[0].Name != "Stray" {
		t.Fatalf("stray symbol must land unattributed, got %+v", rep.Unattributed)
	}
}

func TestHarvestFootgunScaffolds(t *testing.T) {
	kb := harvestKB(t)
	meta := []byte(`{"title":"merge fix","commits":[
		{"subject":"fix: replay storm","body":"The queue replayed on retry.\nFootgun: replay dedupe relies on the idempotency key."},
		{"subject":"chore: docs"}],
		"reviews":[{"body":"note\nFootgun: replay dedupe relies on the idempotency key."}]}`)
	rep, err := HarvestPR(kb, harvestDiff(t, kb), meta)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if len(rep.Quirks) != 2 {
		t.Fatalf("one commit plus one review marker expected, got %+v", rep.Quirks)
	}
	if !strings.Contains(rep.Quirks[0].Slug, "replay-dedupe") {
		t.Fatalf("slug must derive from the marker text: %s", rep.Quirks[0].Slug)
	}
	if rep.Quirks[1].Slug == rep.Quirks[0].Slug {
		t.Fatalf("duplicate slug must be disambiguated: %s", rep.Quirks[1].Slug)
	}
	for _, q := range rep.Quirks {
		for _, h := range []string{"## Symptom", "## Cause", "## Resolution"} {
			if !strings.Contains(q.Body, h) {
				t.Fatalf("scaffold must carry %s: %s", h, q.Body)
			}
		}
	}
}

func TestHarvestNoMarkersNoQuirks(t *testing.T) {
	kb := harvestKB(t)
	rep, err := HarvestPR(kb, harvestDiff(t, kb), []byte(`{"commits":[{"subject":"feat: thing"}]}`))
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if len(rep.Quirks) != 0 {
		t.Fatalf("no markers must mean no quirks: %+v", rep.Quirks)
	}
}

func TestHarvestMetadataFailClosed(t *testing.T) {
	kb := harvestKB(t)
	if _, err := HarvestPR(kb, harvestDiff(t, kb), []byte("{not json")); err == nil {
		t.Fatal("malformed metadata must fail closed")
	}
	if _, err := HarvestPR(kb, nil, nil); err != nil {
		t.Fatalf("empty diff is a valid empty harvest: %v", err)
	}
}
