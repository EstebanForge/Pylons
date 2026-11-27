// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The ledger's templates/ declare the frontmatter schema their docs
// are validated against: a key lives exactly while its template does.
func TestLintHonorsLedgerTemplateKeys(t *testing.T) {
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
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\n")
	write(filepath.Join("templates", "reference-custom.md"), "---\ntitle: Reference custom\ntype: reference\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\nexperiment_key: true\n---\n\n# Reference custom\n\nBody.\n")
	write(filepath.Join("packages", "kept.md"), "---\ntitle: Kept\ntype: reference\nstatus: active\npackage: kept\nexperiment_key: v\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# Kept\n\nBody.\n")

	code, out, _ := runPylon(t, kb, "lint")
	if code != 0 {
		t.Fatalf("template-declared key must lint clean, got %d:\n%s", code, out)
	}
	if strings.Contains(out, "PYLON064") {
		t.Fatalf("template-declared key must not fire PYLON064, output:\n%s", out)
	}

	// The schema follows the template: gone means unknown again.
	if err := os.Remove(filepath.Join(kb, "templates", "reference-custom.md")); err != nil {
		t.Fatal(err)
	}
	_, out, _ = runPylon(t, kb, "lint")
	if !strings.Contains(out, "PYLON064") {
		t.Fatalf("dropping the template must restore PYLON064, output:\n%s", out)
	}
}
