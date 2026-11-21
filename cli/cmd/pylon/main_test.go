// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInitLintExplain(t *testing.T) {
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifests", "pylons.yaml")); err != nil {
		t.Fatalf("expected manifests/pylons.yaml after init: %v", err)
	}
	if code := run([]string{"lint", "--pylons", dir}); code != 0 {
		t.Fatalf("lint on clean dir returned %d, want 0", code)
	}
	if code := run([]string{"explain", "PYLON001"}); code != 0 {
		t.Fatalf("explain PYLON001 returned %d, want 0", code)
	}
	if code := run([]string{"brief", "--pylons", dir}); code != 0 {
		t.Fatalf("brief returned %d, want 0", code)
	}
	if code := run([]string{"manifest", "rules", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest rules returned %d, want 0", code)
	}
	if code := run([]string{"manifest", "rules", "--check", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest rules --check returned %d, want 0", code)
	}
	if code := run([]string{"--aiur"}); code != 0 {
		t.Fatalf("--aiur returned %d, want 0", code)
	}
}

func TestMergeDangerFlagPath(t *testing.T) {
	// The gate accepts --body and --base through the CLI flag path,
	// not only through the Go API.
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	body := filepath.Join(dir, "body.md")
	if err := os.WriteFile(body, []byte("Merge danger: two-way | blast radius: docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"review", "merge-danger", "--body", body, "--base", "origin/main", "--pylons", dir}); code != 0 {
		t.Fatalf("merge-danger with --base returned %d, want 0", code)
	}
	// The value-flags parse even when --base names a nonexistent
	// ref: the gate fails open loudly with the advisory.
	if code := run([]string{"review", "merge-danger", "--body", body, "--base", "no-such-ref", "--pylons", dir}); code != 0 {
		t.Fatalf("merge-danger with a broken --base returned %d, want 0 (advisory)", code)
	}
}

func TestGraphFlagPath(t *testing.T) {
	// The graph flows through the CLI flag path: --graph is a value
	// flag, --depth parses, and a missing package is a usage error.
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	if code := run([]string{"context", "--graph", "ghost", "--pylons", dir}); code != 2 {
		t.Fatalf("graph for a package no doc declares must exit 2, got %d", code)
	}
	if code := run([]string{"context", "--graph", "ghost", "--depth", "notanint", "--pylons", dir}); code != 2 {
		t.Fatalf("a non-integer --depth must exit 2, got %d", code)
	}
}

func TestAnnotationsFlagPath(t *testing.T) {
	// The annotation converter flows through the CLI flag path; stdin
	// input works and unreadable input fails closed.
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	if code := run([]string{"review", "annotations", "--pylons", dir}); code != 2 {
		t.Fatalf("annotations without --from must exit 2, got %d", code)
	}
	sarif := filepath.Join(dir, "lint.sarif")
	payload := `{"runs": [{"results": [{"ruleId": "PYLON001", "level": "error",
		"message": {"text": "broken"},
		"locations": [{"physicalLocation": {"artifactLocation": {"uri": "packages/a.md"}, "region": {"startLine": 3}}}]}]}]}`
	if err := os.WriteFile(sarif, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"review", "annotations", "--from", sarif, "--pylons", dir}); code != 0 {
		t.Fatalf("annotations from a SARIF file must exit 0, got %d", code)
	}
	bad := filepath.Join(dir, "bad.sarif")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"review", "annotations", "--from", bad, "--pylons", dir}); code != 2 {
		t.Fatalf("unparsable input must fail closed with exit 2, got %d", code)
	}
}
