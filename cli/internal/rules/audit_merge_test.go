// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"testing"
)

// Audits merged into investigations: the folder is gone, the document
// type stays. Audit docs live in investigations/ and keep their own
// rules (PYLON018 row statuses, PYLON026 reaudit dates) while the
// investigation checks stop applying to them.
func TestAuditDocsLiveInInvestigations(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	mustWrite(t, filepath.Join(kb, "investigations", "2026-01-01-plain.md"), "---\ntitle: \"Inc\"\ntype: investigation\nstatus: active\n---\n\n# Inc\n")
	mustWrite(t, filepath.Join(kb, "investigations", "sec-audit.md"), `---
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
	findings := Lint(kb, LintOptions{})
	for _, f := range findings {
		if f.RuleID == "PYLON030" {
			t.Errorf("audit doc must not be judged by the investigation filename rule: %+v", f)
		}
	}
	var sawRow, sawReaudit bool
	for _, f := range findings {
		switch f.RuleID {
		case "PYLON018":
			sawRow = true
		case "PYLON026":
			sawReaudit = true
		}
	}
	if !sawRow || !sawReaudit {
		t.Errorf("audit rules must fire from investigations/: row=%v reaudit=%v", sawRow, sawReaudit)
	}
}

func TestAuditTerminalStatusesArchiveInInvestigations(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	for _, tc := range []struct {
		rel    string
		status string
		wantPY string
	}{
		{"investigations/done-audit.md", "withdrawn", "PYLON007"},
		{"investigations/archive/done-audit.md", "withdrawn", ""},
	} {
		mustWrite(t, filepath.Join(kb, filepath.FromSlash(tc.rel)), "---\ntitle: \"A\"\ntype: audit\nstatus: "+tc.status+"\n---\n\n# A\n")
		findings := Lint(kb, LintOptions{})
		var fired bool
		for _, f := range findings {
			if f.RuleID == tc.wantPY {
				fired = true
			}
		}
		if tc.wantPY != "" && !fired {
			t.Errorf("%s: expected %s, got %d findings", tc.rel, tc.wantPY, len(findings))
		}
		if tc.wantPY == "" && fired {
			t.Errorf("%s: unexpected PYLON007", tc.rel)
		}
	}
}
