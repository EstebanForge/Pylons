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

// The review passes the gate runs on staged files (plan structure,
// quirk sources) must honor the accepted-debt baseline exactly like
// the lint pass does. Otherwise a ledger that accepted debt in
// manifests/baseline.json sails through `pylon lint` and then dies at
// commit time on findings it already accepted.
func TestGateAppliesBaselineToPlanReview(t *testing.T) {
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
	mustWrite(filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	mustWrite(filepath.Join(kb, "proposals", "docs-short.md"), "---\ntitle: Short\ntype: plan\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Problem\n\nNothing works yet.\n")

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

	// Control: without a baseline the debt blocks, and lint names the
	// same defect before the gate does (lint parity for the plan
	// review path).
	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 1 {
		t.Fatalf("unbaselined plan debt must block pre-commit, got %d", code)
	}
	code, out, _ := runPylon(t, kb, "lint")
	if code != 1 || !strings.Contains(out, "PYLON021") {
		t.Fatalf("lint must surface PYLON021 itself, got exit %d, output:\n%s", code, out)
	}

	// Accepted debt passes the gate.
	if code, _, _ := runPylon(t, kb, "lint", "--update-baseline"); code != 0 {
		t.Fatalf("update-baseline: %d", code)
	}
	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 0 {
		t.Fatalf("baselined plan debt must pass pre-commit, got %d", code)
	}
	if code, _, _ := runPylon(t, kb, "lint"); code != 0 {
		t.Fatalf("baselined lint must stay clean, got %d", code)
	}
}

func TestGateAppliesBaselineToQuirkReview(t *testing.T) {
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
	mustWrite(filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	mustWrite(filepath.Join(kb, "quirks", "q.md"), "---\ntitle: q\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\npackage: \"\"\nsources: [nope/missing.go]\ncontracts: []\nguard: test\nguardrail: keep the guard valid\n---\n\n## Symptom\n\ns\n\n## Cause\n\nc\n\n## Resolution\n\nr\n")

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

	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 1 {
		t.Fatalf("unbaselined quirk source debt must block pre-commit, got %d", code)
	}

	if code, _, _ := runPylon(t, kb, "lint", "--update-baseline"); code != 0 {
		t.Fatalf("update-baseline: %d", code)
	}
	if code, _, _ := runPylon(t, kb, "gate", "pre-commit"); code != 0 {
		t.Fatalf("baselined quirk source debt must pass pre-commit, got %d", code)
	}
}
