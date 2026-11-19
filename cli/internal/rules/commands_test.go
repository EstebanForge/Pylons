// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPlanContract(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "api.go"), "package api\nfunc Serve() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n")
	planPath := filepath.Join(kb, "plans/feat-serve.md")
	mustWrite(t, planPath, `---
title: "Serve"
type: plan
status: accepted
components: [svc]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/api.go::Serve
    - kind: go-symbol
      id: repo://svc/api.go::Missing
---

# Serve

Plan body.
`)
	findings, err := CheckPlanContract(kb, "plans/feat-serve.md", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].RuleID != "PYLON003" {
		t.Fatalf("expected 1 PYLON003 finding, got %+v", findings)
	}
}

func TestStalenessAndFix(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "packages/old.md"), `---
title: "Old pkg"
type: package
status: active
package: old
updated: 2020-01-01
---

# Old pkg

Old.
`)
	findings := CheckStaleness(kb, "", false)
	if len(findings) != 1 || findings[0].RuleID != "PYLON015" {
		t.Fatalf("expected 1 PYLON015 finding, got %+v", findings)
	}

	// --fix bumps updated: to today, so a second pass is clean.
	_ = CheckStaleness(kb, "", true)
	after := CheckStaleness(kb, "", false)
	if len(after) != 0 {
		t.Fatalf("expected 0 stale findings after --fix, got %+v", after)
	}
}

func TestReviewPlanAndPostmortem(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "auth/login.go"), "package auth\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  svc:\n    path: "+proj+"\n")

	mustWrite(t, filepath.Join(kb, "proposals/feature-x.md"), `---
title: "Feature X"
type: plan
status: proposed
components: [svc]
---

# Feature X

## Problem

Missing sections.
`)
	pf := ReviewPlan(kb, "proposals/feature-x.md")
	if len(pf) < 2 {
		t.Fatalf("expected missing section findings, got %+v", pf)
	}

	mustWrite(t, filepath.Join(kb, "quirks/auth-trap.md"), `---
title: "Troubleshooting: Auth trap"
type: quirk
status: active
created: 2026-11-19
package: svc
sources:
  - repo://svc/auth/login.go
  - repo://svc/auth/missing.go
guard: convention
---

# Auth trap

## Symptom

Fails.
`)
	qf := ReviewPostmortem(kb, "quirks/auth-trap.md", true)
	if len(qf) == 0 {
		t.Fatalf("expected missing section + missing source findings, got 0")
	}
	hasSourceErr := false
	for _, f := range qf {
		if f.RuleID == "PYLON025" {
			hasSourceErr = true
		}
	}
	if !hasSourceErr {
		t.Fatalf("expected PYLON025 for missing source, got %+v", qf)
	}
}

func TestLintFixMovesMismatchedPlan(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "plans/shipped-item.md"), `---
title: "Shipped item"
type: plan
status: shipped
artifact: "PR #1"
---

# Shipped item

Done.
`)
	_ = Lint(kb, LintOptions{Fix: true})
	if _, err := os.Stat(filepath.Join(kb, "plans/archive/shipped-item.md")); err != nil {
		t.Fatalf("expected shipped plan moved to plans/archive/: %v", err)
	}
}
