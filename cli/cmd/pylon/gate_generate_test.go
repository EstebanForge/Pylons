// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// With generate: [] the pre-commit drift check must not run at all: the
// manifests are foreign-owned, so sweeping them into unrelated commits
// (or dying on git fatal when the files are absent) breaks handovers.
func TestGateGenerateOffSkipsDriftStage(t *testing.T) {
	kb := t.TempDir()
	for _, d := range []string{"manifests", "docs"} {
		if err := os.MkdirAll(filepath.Join(kb, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"),
		[]byte("version: 1\ngenerate: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := "---\ntitle: \"Note\"\ntype: reference\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# Note\n"
	if err := os.WriteFile(filepath.Join(kb, "docs", "note.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = kb
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+filepath.Join(t.TempDir(), "gitconfig"),
			"GIT_CONFIG_SYSTEM="+filepath.Join(t.TempDir(), "gitconfig2"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q")
	git("config", "user.email", "t@example.invalid")
	git("config", "user.name", "t")
	git("add", "-A")
	git("commit", "-qm", "seed")
	// A foreign in-progress edit sits staged; the gate must leave it alone.
	if err := os.WriteFile(filepath.Join(kb, "docs", "note.md"),
		[]byte(strings.Replace(note, "# Note\n", "# Note v2\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "docs/note.md")

	code, _, stderr := runPylon(t, kb, "gate", "pre-commit")
	if code != 0 {
		t.Fatalf("generate: [] gate must pass, got %d", code)
	}
	if strings.Contains(stderr, "stage manifests") {
		t.Fatalf("drift stage must be skipped under generate: []: %s", stderr)
	}
	staged := strings.TrimSpace(git("diff", "--cached", "--name-only"))
	if staged != "docs/note.md" {
		t.Fatalf("only the foreign edit may stay staged, got %q", staged)
	}
}
