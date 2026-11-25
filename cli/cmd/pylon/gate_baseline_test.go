// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The gate stages and the lint command share one suppression surface:
// accepted FORCED debt recorded in manifests/baseline.json. A ledger
// carrying baselined debt must pass gate pre-commit/pre-push, or the
// gates block forever on debt the operator already accepted.
func TestGateAppliesBaseline(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	quirk := "---\ntitle: Bad guard\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\npackage: \"\"\nsources: []\ncontracts: []\nguard: warden-test\nguardrail: fix the guard value\n---\n\n## Symptom\n\nA guard outside the closed set.\n\n## Cause\n\nLegacy vocabulary.\n\n## Resolution\n\nRemap the guard to test.\n"
	if err := os.WriteFile(filepath.Join(kb, "quirks", "bad-guard.md"), []byte(quirk), 0o644); err != nil {
		t.Fatal(err)
	}
	// Bring manifests current so the pre-push manifest checks pass.
	if code, _, _ := runPylon(t, kb, "manifest", "rules"); code != 0 {
		t.Fatalf("manifest rules: %d", code)
	}
	if code, _, _ := runPylon(t, kb, "manifest", "packages"); code != 0 {
		t.Fatalf("manifest packages: %d", code)
	}
	if code, _, _ := runPylon(t, kb, "manifest", "vocabulary"); code != 0 {
		t.Fatalf("manifest vocabulary: %d", code)
	}

	// The unbaselined finding blocks the gate.
	if code, _, _ := runPylon(t, kb, "gate", "pre-push"); code != 1 {
		t.Fatalf("unbaselined corpus debt must block pre-push, got %d", code)
	}

	// Record the debt, then the gate must pass.
	if code, _, _ := runPylon(t, kb, "lint", "--update-baseline"); code != 0 {
		t.Fatalf("update-baseline: %d", code)
	}
	if code, _, _ := runPylon(t, kb, "gate", "pre-push"); code != 0 {
		t.Fatalf("baselined debt must pass pre-push, got %d", code)
	}
	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 0 {
		t.Fatalf("baselined debt must pass pre-commit, got %d", code)
	}

	// The lint command itself stays clean too.
	if code, _, _ := runPylon(t, kb, "lint"); code != 0 {
		t.Fatalf("baselined lint must exit 0, got %d", code)
	}
}
