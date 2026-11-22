// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"
)

func decisionKB(t *testing.T, manifestExtra, name string) string {
	t.Helper()
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n"+manifestExtra)
	mustWrite(t, filepath.Join(kb, "decisions", name),
		"---\ntitle: D\ntype: decision\nstatus: accepted\ncreated: 2026-11-01\nupdated: 2026-11-20\nowners: [o]\nsupersedes: []\nartifact: x\nlinks: []\ndescription: d\ntags: [d]\n---\n\n# D\n")
	return kb
}

func TestDecisionPrefixKnobAcceptsConfiguredShape(t *testing.T) {
	fs := Lint(decisionKB(t, "decision_prefix: A\n", "A0001-keep-foreign-names.md"), LintOptions{})
	for _, f := range fs {
		if f.RuleID == "PYLON010" {
			t.Fatalf("the configured prefix must accept A0001: %v", f.Message)
		}
	}
}

func TestDecisionPrefixDefaultStillDemandsD(t *testing.T) {
	fs := Lint(decisionKB(t, "", "A0001-keep-foreign-names.md"), LintOptions{})
	found := false
	for _, f := range fs {
		if f.RuleID == "PYLON010" {
			found = true
			if !strings.Contains(f.Message, "DNNNN") {
				t.Fatalf("the message must name the default prefix: %s", f.Message)
			}
		}
	}
	if !found {
		t.Fatal("a foreign-named accepted decision must fail under the default prefix")
	}
}

func TestDecisionPrefixConfiguredRejectsOtherShapes(t *testing.T) {
	fs := Lint(decisionKB(t, "decision_prefix: A\n", "D0001-legacy.md"), LintOptions{})
	found := false
	for _, f := range fs {
		if f.RuleID == "PYLON010" {
			found = true
			if !strings.Contains(f.Message, "ANNNN") {
				t.Fatalf("the message must name the configured prefix: %s", f.Message)
			}
		}
	}
	if !found {
		t.Fatal("D0001 must fail when the ledger configures prefix A")
	}
}
