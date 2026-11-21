// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// Docs matched by the config exclude globs are outside the engine:
// never linted, never graph docs. Everything else lints normally.
func TestConfigExcludeSkipsCorpus(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nexclude: [legacy/**]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBadDoc(t, filepath.Join(kb, "legacy", "old.md"))
	writeBadDoc(t, filepath.Join(kb, "plans", "keep.md"))

	findings := Lint(kb, LintOptions{})
	for _, f := range findings {
		if filepath.ToSlash(filepath.Dir(f.File)) == "legacy" || f.File == "legacy/old.md" {
			t.Errorf("excluded doc produced %s: %s", f.RuleID, f.Message)
		}
	}
	found := false
	for _, f := range findings {
		if f.File == "plans/keep.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("the non-excluded doc must still lint (fixture must produce findings)")
	}
}

// writeBadDoc drops a doc that fails the frontmatter rule.
func writeBadDoc(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("no frontmatter here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
