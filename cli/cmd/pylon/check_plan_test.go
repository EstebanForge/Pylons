// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// --plan names the plan of the caller on check and on context. A name that
// is not an active plan of the ledger is a usage error.
func TestCheckPlanFlag(t *testing.T) {
	dir := t.TempDir()
	plan := `---
title: "Ship"
type: plan
status: accepted
created: 2026-11-23
updated: 2026-11-23
contract:
  stable: []
  unstable: []
  editing:
    - auth/**
---

# Ship
`
	for path, body := range map[string]string{
		"manifests/pylons.yaml": "version: 1\nprojects: {}\n",
		"proposals/ship.md":     plan,
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := []string{"auth/login.go", "--pylons", dir}

	if code := run(append([]string{"check"}, target...)); code != 1 {
		t.Fatalf("check without a plan returned %d, want 1", code)
	}
	if code := run(append([]string{"check", "--plan", "proposals/ship.md"}, target...)); code != 0 {
		t.Fatalf("check --plan returned %d, want 0", code)
	}
	if code := run(append([]string{"context", "--plan", "proposals/ship.md"}, target...)); code != 0 {
		t.Fatalf("context --plan returned %d, want 0", code)
	}
	if code := run(append([]string{"check", "--plan", "proposals/missing.md"}, target...)); code != 2 {
		t.Fatalf("check --plan with a missing plan returned %d, want 2", code)
	}
	if code := run(append([]string{"context", "--plan", "proposals/missing.md"}, target...)); code != 2 {
		t.Fatalf("context --plan with a missing plan returned %d, want 2", code)
	}
}
