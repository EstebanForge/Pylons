// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The commit gate must not review documents the ledger excluded
// through pylons.yaml. The lint pass already honors the exclude list;
// a review loop that ignores it turns a deliberately skipped zone
// back into a commit blocker that update-baseline can never record,
// because excluded docs generate no lint findings to baseline.
func TestGateSkipsExcludedReviewPaths(t *testing.T) {
	kb := t.TempDir()
	mustWrite := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\nexclude:\n  - proposals/skipme/**\n  - quirks/skipme/**\n")
	mustWrite(filepath.Join(kb, "proposals", "skipme", "docs-x.md"), "---\ntitle: X\ntype: plan\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Problem\n\np\n")
	mustWrite(filepath.Join(kb, "quirks", "skipme", "q.md"), "---\ntitle: q\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\npackage: \"\"\nsources: [nope/missing.go]\ncontracts: []\nguard: test\nguardrail: keep the guard valid\n---\n\n## Symptom\n\ns\n\n## Cause\n\nc\n\n## Resolution\n\nr\n")

	gitRun := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", kb}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitRun("init", "-q", "-b", "main")
	gitRun("config", "user.email", "t@example.com")
	gitRun("config", "user.name", "t")
	gitRun("add", "-Af", ".")

	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 0 {
		t.Fatalf("excluded review paths must not block pre-commit, got %d", code)
	}

	// Control: without the exclusion the same content blocks on both
	// review loops.
	mustWrite(filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	gitRun("add", "-Af", ".")
	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 1 {
		t.Fatalf("unexcluded review debt must block pre-commit, got %d", code)
	}
}
