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

// Contracts whose components do not map to a registered project must
// surface as PYLON003, not skip silently.
func TestUnregisteredComponentsFlagged(t *testing.T) {
	kb := t.TempDir()
	os.MkdirAll(filepath.Join(kb, "plans"), 0o755)
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects:\n  real:\n    path: ../real\n"), 0o644)
	os.WriteFile(filepath.Join(kb, "plans", "feature-x.md"), []byte("---\ntitle: X\ntype: plan\nstatus: accepted\nticket:\n  - id: T1\ncomponents: [typo-alias]\ncontract:\n  stable:\n    - id: file:src.go\n      wrong_if: never\n---\n\n# X\n"), 0o644)
	findings := Lint(kb, LintOptions{HasCode: true})
	found := false
	for _, f := range findings {
		if f.RuleID == "PYLON003" && strings.Contains(f.Message, "components do not map") {
			found = true
		}
	}
	if !found {
		t.Fatalf("PYLON003 not raised for unregistered components: %v", findings)
	}
}

// staleness --fix must never rewrite updated: lines in body text.
func TestStalenessFixFrontmatterOnly(t *testing.T) {
	content := "---\ntitle: P\ntype: package\nstatus: active\npackage: p\nupdated: 2020-01-01\n---\n\nExample block:\nupdated: 1999-12-31\n"
	patched := patchUpdatedFrontMatter(content, "2026-11-19")
	if !strings.Contains(patched, "updated: 1999-12-31") {
		t.Fatal("body updated: line was rewritten")
	}
	if strings.Contains(patched, "updated: 2020-01-01") {
		t.Fatal("frontmatter updated: was not bumped")
	}
}

// Postmortem headings must be exact lines; ## Problems must not satisfy ## Problem.
func TestPostmortemExactHeadings(t *testing.T) {
	if hasHeading("## Problems\n", "## Problem") {
		t.Fatal("## Problems matched ## Problem")
	}
	if !hasHeading("intro\n## Cause \n", "## Cause") {
		t.Fatal("trimmed heading not matched")
	}
	kb := t.TempDir()
	os.MkdirAll(filepath.Join(kb, "quirks"), 0o755)
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects: {}\n"), 0o644)
	os.WriteFile(filepath.Join(kb, "quirks", "feature-q.md"), []byte("---\ntitle: Q\ntype: quirk\nstatus: active\ncreated: 2026-11-19\nupdated: 2026-11-19\n---\n\n## Problems\n## Causes\n## Resolutions\n"), 0o644)
	findings := ReviewPostmortem(kb, "quirks/feature-q.md", false)
	if len(findings) == 0 {
		t.Fatal("near-miss headings must not satisfy required sections")
	}
}

// Zone entry advisories must come out in deterministic order.
func TestFolderEntryAdvisoriesDeterministic(t *testing.T) {
	kb := t.TempDir()
	for _, zone := range []string{"zz-docs", "aa-docs", "mm-docs"} {
		os.MkdirAll(filepath.Join(kb, zone), 0o755)
		os.WriteFile(filepath.Join(kb, zone, "feature-d.md"), []byte("---\ntitle: D\ntype: reference\nstatus: active\ncreated: 2026-11-19\nupdated: 2026-11-19\n---\n\nBody.\n"), 0o644)
	}
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects: {}\n"), 0o644)
	var first []string
	for i := 0; i < 30; i++ {
		var zones []string
		for _, f := range Lint(kb, LintOptions{}) {
			if f.RuleID == "PYLON034" {
				zones = append(zones, f.File)
			}
		}
		if i == 0 {
			first = zones
			continue
		}
		for j := range zones {
			if zones[j] != first[j] {
				t.Fatalf("run %d order differs: %v vs %v", i, zones, first)
			}
		}
	}
	_ = config.Config{}
}
