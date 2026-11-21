// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"testing"
)

func TestGraphAndDependencyRules(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "README.md"), "# KB\n\n- [Svc](packages/svc.md)\n")
	mustWrite(t, filepath.Join(kb, "packages/AGENTS.md"), "# Packages\n")
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), `---
title: "Svc"
type: package
status: active
package: svc
updated: 2026-11-19
depends_on: [missing-pkg]
---

# Svc

Mentions SessionToken without linking.
`)
	mustWrite(t, filepath.Join(kb, "domain/entities/session-token.md"), `---
title: "SessionToken"
type: domain-entity
status: accepted
---

# SessionToken

Auth token.
`)
	mustWrite(t, filepath.Join(kb, "decisions/D0001-new.md"), `---
title: "New"
type: decision
status: accepted
supersedes: [D9999-missing.md]
---

# New

Supersedes missing.
`)
	// Orphan doc with no incoming links
	mustWrite(t, filepath.Join(kb, "references/orphan.md"), `---
title: "Orphan"
type: reference
status: active
---

# Orphan

Unlinked doc.
`)

	findings := Lint(kb, LintOptions{HasCode: false})
	got := map[string]bool{}
	for _, f := range findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"PYLON004", "PYLON005", "PYLON013", "PYLON019", "PYLON034"} {
		if !got[want] {
			t.Errorf("expected %s to fire; got %+v", want, findings)
		}
	}
}

func TestPlanQuirkAndContractExtendedRules(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "svc.go"), "package svc\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(proj, "svc_test.go"), "package svc\nfunc TestPing() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n")

	// Plan: accepted, missing ticket:, contract.stable missing wrong_if, bad verified_by, inert glob, in-progress no trace
	mustWrite(t, filepath.Join(kb, "plans/feature-one.md"), `---
title: "Feature one"
type: plan
status: accepted
components: [svc]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/svc.go::Ping
      verified_by: svc_test.go::TestMissing
  editing:
    - nonexistent/**
---

# Feature one

Body.
`)
	mustWrite(t, filepath.Join(kb, "plans/feature-two.md"), `---
title: "Feature two"
type: plan
status: in-progress
components: [svc]
---

# Feature two

In progress.
`)
	// Package with pending: true on a symbol that already exists -> PYLON024
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), `---
title: "Svc"
type: package
status: active
package: svc
updated: 2026-11-19
exposes:
  - kind: go-symbol
    id: repo://svc/svc.go::Ping
    pending: true
---

# Svc

Svc.
`)
	// Quirk: old active with no guard (PYLON028) + bad contract entry (PYLON036)
	mustWrite(t, filepath.Join(kb, "quirks/old-quirk.md"), `---
title: "Old quirk"
type: quirk
status: active
created: 2020-01-01
contracts:
  - kind: ""
    id: ""
---

# Old quirk

## Symptom

S.

## Cause

C.

## Resolution

R.
`)

	findings := Lint(kb, LintOptions{HasCode: true})
	got := map[string]bool{}
	for _, f := range findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"PYLON014", "PYLON016", "PYLON024", "PYLON027", "PYLON028", "PYLON032", "PYLON035", "PYLON036"} {
		if !got[want] {
			t.Errorf("expected %s to fire; got %+v", want, findings)
		}
	}
}

func TestDocLifecycleAndFilenameRules(t *testing.T) {
	kb := t.TempDir()
	// Move target collision: plans/archive/feature-dup.md already exists
	mustWrite(t, filepath.Join(kb, "plans/feature-dup.md"), "---\ntitle: \"Dup\"\ntype: plan\nstatus: shipped\nartifact: \"PR 1\"\n---\n\n# Dup\n")
	mustWrite(t, filepath.Join(kb, "plans/archive/feature-dup.md"), "---\ntitle: \"Dup\"\ntype: plan\nstatus: shipped\nartifact: \"PR 1\"\n---\n\n# Dup\n")

	// Bad plan filename (PYLON038)
	mustWrite(t, filepath.Join(kb, "plans/badname.md"), "---\ntitle: \"Bad\"\ntype: plan\nstatus: accepted\nticket: [T-1]\n---\n\n# Bad\n")

	// Overdue audit (PYLON026) + invalid row status (PYLON018)
	mustWrite(t, filepath.Join(kb, "investigations/sec-audit.md"), `---
title: "Sec Audit"
type: audit
status: active
reaudit: 2020-01-01
---

# Sec Audit

| ID | Severity | Finding | Status |
|---|---|---|---|
| F1 | high | Issue | invalid_state |
`)

	// Bad investigation filename (PYLON030) + invalid closure (PYLON031)
	mustWrite(t, filepath.Join(kb, "investigations/archive/bad-inv.md"), "---\ntitle: \"Inv\"\ntype: investigation\nstatus: resolved\n---\n\n# Inv\n\nResolved without sections.\n")

	// Ticket with terminal status outside archive (PYLON007)
	mustWrite(t, filepath.Join(kb, "tickets/T001-done.md"), "---\ntitle: \"Done ticket\"\ntype: ticket\nstatus: done\n---\n\n# Done ticket\n")

	findings := Lint(kb, LintOptions{HasCode: false})
	got := map[string]bool{}
	for _, f := range findings {
		got[f.RuleID] = true
	}
	for _, want := range []string{"PYLON007", "PYLON011", "PYLON018", "PYLON026", "PYLON030", "PYLON031", "PYLON038"} {
		if !got[want] {
			t.Errorf("expected %s to fire; got %+v", want, findings)
		}
	}
}
