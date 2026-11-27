// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exclude: promises "never linted and never indexed" (pylons-yaml.md).
// The corpus map feeds map/search/status, so it must skip excluded
// docs just like the lint walk does.
func TestScanDocsHonorsExclude(t *testing.T) {
	kb := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\nexclude:\n  - atlas/**\n")
	write(filepath.Join("atlas", "ok.md"), "---\ntitle: Zone doc\ntype: meta\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# Zone doc\n\nBody.\n")
	write(filepath.Join("packages", "kept.md"), "---\ntitle: Kept\ntype: package\nstatus: active\npackage: kept\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# Kept\n\nBody.\n")

	_, out, _ := runPylon(t, kb, "map", "--all", "--json")
	if strings.Contains(out, "atlas/ok.md") {
		t.Fatalf("map must not index excluded docs, output:\n%s", out)
	}
	if !strings.Contains(out, "packages/kept.md") {
		t.Fatalf("map must keep unexcluded docs, output:\n%s", out)
	}

	// A broken doc inside an excluded folder cannot block lint.
	write(filepath.Join("atlas", "broken.md"), "no frontmatter at all\n")
	if code, _, _ := runPylon(t, kb, "lint"); code != 0 {
		t.Fatalf("excluded folder must not block lint, got %d", code)
	}

	// Dropping the exclusion restores both surfaces.
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\n")
	_, out, _ = runPylon(t, kb, "map", "--all", "--json")
	if !strings.Contains(out, "atlas/ok.md") {
		t.Fatalf("without exclude the map must index the doc, output:\n%s", out)
	}
	if code, _, _ := runPylon(t, kb, "lint"); code == 0 {
		t.Fatalf("without exclude the broken doc must block lint")
	}
}
