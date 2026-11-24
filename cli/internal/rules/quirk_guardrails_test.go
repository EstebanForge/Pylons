// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// guardrailFixture builds a temp ledger with one tracked project
// (alias alpha, path proj) and a set of quirk documents.
func guardrailFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nextensions: []\nprojects:\n  alpha:\n    path: proj\n"
	if err := os.WriteFile(filepath.Join(root, "manifests", "pylons.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "quirks", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "proj", "src", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	quirk := func(name, sources string) {
		t.Helper()
		body := "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\nsources: " + sources + "\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
		if err := os.WriteFile(filepath.Join(root, "quirks", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	quirk("file-trap.md", "[repo://alpha/src/x.go]")
	quirk("dir-trap.md", "[repo://alpha/src/lib]")
	quirk("dir-slash.md", "[repo://alpha/src/lib/]")
	quirk("other-alias.md", "[repo://beta/src/x.go]")
	quirk("multi.md", "[repo://alpha/src/other.go, repo://alpha/src/lib/deep.go]")
	archived := "---\ntitle: \"Old\"\ntype: quirk\nstatus: active\nsources: [repo://alpha/src/x.go]\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "archive", "old.md"), []byte(archived), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := "Not a quirk: no frontmatter, sources repo://alpha/src/x.go as prose.\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := "---\ntitle: \"P\"\ntype: plan\nstatus: accepted\nsources: [repo://alpha/src/x.go]\ncreated: 2026-11-24\nupdated: 2026-11-24\ncontracts: []\ntags: []\n---\n\n# P\n"
	if err := os.WriteFile(filepath.Join(root, "plans", "p.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestQuirkGuardrailsMatchesTargets(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/x.go"})
	want := []string{"quirks/file-trap.md"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsDirPrefixBoundary(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/lib/deep.go"})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md", "quirks/multi.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
	// A sibling whose name merely extends the source path must not
	// fire: the prefix boundary is a path boundary, not a string one.
	got = QuirkGuardrails(root, []string{"repo://alpha/src/libx"})
	if len(got) != 0 {
		t.Fatalf("boundary leak: %v", got)
	}
}

func TestQuirkGuardrailsTrailingSlashSourceAndTarget(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/lib/"})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsAbsoluteTargetAttribution(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{filepath.Join(root, "proj", "src", "x.go")})
	want := []string{"quirks/file-trap.md"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsIgnoresLedgerTargetsAndArchive(t *testing.T) {
	root := guardrailFixture(t)
	for _, targets := range [][]string{
		{"plans/p.md"},
		{"/tmp/unregistered/src/x.go"},
	} {
		if got := QuirkGuardrails(root, targets); len(got) != 0 {
			t.Fatalf("guardrails for %v = %v, want none", targets, got)
		}
	}
	// Alias mismatch: an alpha-quirk never fires on a beta target and
	// vice versa.
	got := QuirkGuardrails(root, []string{"repo://beta/src/x.go"})
	if len(got) != 1 || got[0] != "quirks/other-alias.md" {
		t.Fatalf("guardrails for beta target = %v, want [quirks/other-alias.md]", got)
	}
}

func TestQuirkGuardrailsSortedDistinctAcrossTargets(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{
		"repo://alpha/src/lib/deep.go",
		"repo://alpha/src/x.go",
		"repo://alpha/src/lib/helper.go",
	})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md", "quirks/file-trap.md", "quirks/multi.md"}
	if len(got) != len(want) {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("guardrails = %v, want %v", got, want)
		}
	}
}
