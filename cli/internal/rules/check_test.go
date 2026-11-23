// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// A target under two registered roots must always resolve to the deepest
// one, regardless of Go's randomized map iteration order.
func TestNormalizeCheckTargetDeterministic(t *testing.T) {
	tmp := t.TempDir()
	outer := filepath.Join(tmp, "outer")
	inner := filepath.Join(outer, "inner")
	for _, d := range []string{outer, inner} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"outer": {Path: outer},
		"inner": {Path: inner},
	}}
	target := filepath.Join(inner, "f.md")
	for i := 0; i < 200; i++ {
		nt := normalizeCheckTarget(tmp, target, cfg)
		if nt.alias != "inner" || nt.path != "f.md" {
			t.Fatalf("run %d: got alias=%q path=%q, want inner f.md", i, nt.alias, nt.path)
		}
	}
}

func TestNormalizeCheckTargetAbsoluteTargetWithRelativeRoot(t *testing.T) {
	tmp := t.TempDir()
	svc := filepath.Join(tmp, "svc")
	if err := os.MkdirAll(svc, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"svc": {Path: "svc"},
	}}
	target := filepath.Join(svc, "shipments.go")
	// A relative --pylons root must not blind the absolute match.
	restore := chdirTmp(t, tmp)
	defer restore()
	for i := 0; i < 50; i++ {
		if nt := normalizeCheckTarget(".", target, cfg); nt.alias != "svc" || nt.path != "shipments.go" {
			t.Fatalf("run %d: absolute target with relative root: alias=%q path=%q", i, nt.alias, nt.path)
		}
	}
}

func chdirTmp(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}
}

// activePlanDoc is an accepted plan whose editing and unstable sets are
// the declared scope of its author.
func activePlanDoc(editing, unstable string) string {
	return `---
title: "Plan"
type: plan
status: accepted
created: 2026-11-23
updated: 2026-11-23
owners:
  - Pylons
contract:
  stable: []
  unstable:
    - ` + unstable + `
  editing:
    - ` + editing + `
---

# Plan
`
}

// writeTwoPlanLedger maps one project and two accepted plans: ship edits src/a.go and marks src/b.go unstable, other edits src/shared.go.
func writeTwoPlanLedger(t *testing.T) string {
	t.Helper()
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: app\n")
	mustWrite(t, filepath.Join(kb, "proposals/ship.md"), activePlanDoc("repo://app/src/a.go", "repo://app/src/b.go"))
	mustWrite(t, filepath.Join(kb, "proposals/other.md"), activePlanDoc("repo://app/src/shared.go", ""))
	return kb
}

// The author of an active plan must pass on the scope that plan declares.
// Without a named plan the same scope is still contested ground.
func TestCheckForOwnPlanSparesItsOwnScope(t *testing.T) {
	kb := writeTwoPlanLedger(t)

	if !hasRule(Check(kb, []string{"repo://app/src/a.go"}), "PYLON-CHECK-EDITING") {
		t.Fatal("without a named plan the editing glob must still stop the caller")
	}

	for _, arg := range []string{"proposals/ship.md", filepath.Join(kb, "proposals", "ship.md")} {
		findings, err := CheckFor(kb, arg, []string{"repo://app/src/a.go", "repo://app/src/b.go"})
		if err != nil {
			t.Fatalf("CheckFor(%q): %v", arg, err)
		}
		if hasRule(findings, "PYLON-CHECK-EDITING") || hasRule(findings, "PYLON-CHECK-UNSTABLE") {
			t.Fatalf("CheckFor(%q): the scope of the own plan must not block, got %v", arg, ruleIDs(findings))
		}
	}
}

// Naming a plan spares that plan only. Another active plan on the same
// ground still stops the caller.
func TestCheckForOwnPlanSparesNoOtherPlan(t *testing.T) {
	kb := writeTwoPlanLedger(t)

	findings, err := CheckFor(kb, "proposals/ship.md", []string{"repo://app/src/a.go", "repo://app/src/shared.go"})
	if err != nil {
		t.Fatal(err)
	}
	if ids := ruleIDs(findings); ids["PYLON-CHECK-EDITING"] != 1 || len(findings) != 1 {
		t.Fatalf("want exactly one editing finding from the other plan, got %v", ruleIDs(findings))
	}
	if findings[0].File != "proposals/other.md" {
		t.Fatalf("finding anchored to %s, want proposals/other.md", findings[0].File)
	}
}

// A plan argument that is not an active plan of the ledger is an error,
// never a silent exemption.
func TestCheckForRejectsInactiveOrForeignPlan(t *testing.T) {
	kb := writeTwoPlanLedger(t)
	mustWrite(t, filepath.Join(kb, "proposals/draft.md"),
		strings.Replace(activePlanDoc("repo://app/src/d.go", ""), "status: accepted", "status: proposed", 1))
	mustWrite(t, filepath.Join(kb, "tickets/t.md"), "---\ntitle: \"T\"\ntype: ticket\nstatus: open\n---\n\n# T\n")
	outside := filepath.Join(filepath.Dir(kb), "outside.md")
	mustWrite(t, outside, activePlanDoc("repo://app/src/o.go", ""))

	cases := []struct{ name, arg, want string }{
		{"missing document", "proposals/missing.md", "no such document"},
		{"plan not yet accepted", "proposals/draft.md", "not an active plan"},
		{"document that is not a plan", "tickets/t.md", "not a plan document"},
		{"plan outside the ledger", outside, "outside the ledger root"},
	}
	for _, tc := range cases {
		_, err := CheckFor(kb, tc.arg, []string{"repo://app/src/a.go"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: CheckFor(%q) error = %v, want it to contain %q", tc.name, tc.arg, err, tc.want)
		}
	}
}
