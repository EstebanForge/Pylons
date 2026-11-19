// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortedCLICommands(t *testing.T) {
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	plan := `---
title: "Auth Plan"
type: plan
status: accepted
components: [svc]
contract:
  editing:
    - auth/**
---

# Auth Plan

## Problem

Auth problem.

## Design

Auth design.

## Milestones

- M1
`
	if err := os.WriteFile(filepath.Join(dir, "plans/feat-auth.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := `---
title: "Svc"
type: package
status: active
package: svc
updated: 2026-11-19
---

# Svc

Core service.
`
	if err := os.WriteFile(filepath.Join(dir, "packages/svc.md"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"contract", "plans/feat-auth.md", "--pylons", dir}); code != 0 {
		t.Fatalf("contract returned %d, want 0", code)
	}
	if code := run([]string{"staleness", "--pylons", dir}); code != 0 {
		t.Fatalf("staleness returned %d, want 0", code)
	}
	if code := run([]string{"review", "plan", "plans/feat-auth.md", "--pylons", dir}); code != 0 {
		t.Fatalf("review plan returned %d, want 0", code)
	}
	if code := run([]string{"manifest", "packages", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest packages returned %d, want 0", code)
	}
	if code := run([]string{"manifest", "packages", "--check", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest packages --check returned %d, want 0", code)
	}
	if code := run([]string{"map", "--query", "Auth", "--limit", "5", "--pylons", dir}); code != 0 {
		t.Fatalf("map --query returned %d, want 0", code)
	}
	if code := run([]string{"search", "--limit", "5", "Auth", "--pylons", dir}); code != 0 {
		t.Fatalf("search returned %d, want 0", code)
	}
	if code := run([]string{"check", "--stop-block", "auth/login.go", "--pylons", dir}); code != 1 {
		t.Fatalf("check --stop-block returned %d, want 1", code)
	}
	if code := run([]string{"context", "--preamble", "auth/login.go", "--pylons", dir}); code != 0 {
		t.Fatalf("context --preamble returned %d, want 0", code)
	}

	// review-brief generate + validate
	if code := run([]string{"review-brief", "plans/feat-auth.md", "--pylons", dir}); code != 0 {
		t.Fatalf("review-brief generate returned %d, want 0", code)
	}
	briefFile := filepath.Join(dir, ".pylon-review-brief.md")
	if _, err := os.Stat(briefFile); err != nil {
		t.Fatalf("expected %s to exist: %v", briefFile, err)
	}
	if code := run([]string{"review-brief", "--validate", briefFile, "--pylons", dir}); code != 0 {
		t.Fatalf("review-brief --validate without plan arg returned %d, want 0", code)
	}
	if code := run([]string{"lint", "--format", "bogus", "--pylons", dir}); code != 2 {
		t.Fatalf("expected exit 2 on unknown format, got %d", code)
	}
	if code := run([]string{"protocol", "list", "--pylons", dir}); code != 0 {
		t.Fatalf("protocol list returned %d, want 0", code)
	}
	if code := run([]string{"reflect", "--pylons", dir}); code != 0 {
		t.Fatalf("reflect returned %d, want 0", code)
	}
	if code := run([]string{"extensions", "list", "--pylons", dir}); code != 0 {
		t.Fatalf("extensions list returned %d, want 0", code)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "extensions.json"), []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"extensions", "schema", "--pylons", dir}); code != 0 {
		t.Fatalf("extensions schema returned %d, want 0", code)
	}

	// Archived docs must be excluded from check/context/manifest/map/search by default.
	archPlan := "---\ntitle: \"Archived Plan\"\ntype: plan\nstatus: shipped\nartifact: \"PR 1\"\ncontract:\n  editing:\n    - archived/**\n---\n\n# Archived Plan\n"
	if err := os.WriteFile(filepath.Join(dir, "plans/archive/feature-old.md"), []byte(archPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"check", "archived/file.go", "--pylons", dir}); code != 0 {
		t.Fatalf("expected archived plan editing glob to be excluded from check, got %d", code)
	}
	docs := scanDocs(dir, false)
	for _, d := range docs {
		if d.Title == "Archived Plan" {
			t.Fatal("expected archive/ doc to be excluded when includeArchive=false")
		}
	}
}
