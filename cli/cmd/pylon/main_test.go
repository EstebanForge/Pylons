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
