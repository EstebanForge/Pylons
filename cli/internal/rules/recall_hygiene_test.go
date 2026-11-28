// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// Recall hygiene: a doc that declares superseded_by still answers
// search and map queries while it lives outside an archive folder,
// so agents keep citing dead conclusions. The ledger treats archives
// as the only recall surface for superseded knowledge.
func TestLintFlagsSupersededOutsideArchive(t *testing.T) {
	kb := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	decision := func(title, supersededBy string) string {
		fm := "---\ntitle: " + title + "\ntype: decision\nstatus: accepted\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\ntags: []\n"
		if supersededBy != "" {
			fm += "superseded_by: [" + supersededBy + "]\n"
		}
		return fm + "---\n\nDecision body.\n"
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\nschemes: []\n")
	write(filepath.Join("decisions", "a0001-old-way.md"), decision("Old way", "decisions/a0002-new-way.md"))
	write(filepath.Join("decisions", "a0002-new-way.md"), decision("New way", ""))
	write(filepath.Join("decisions", "archive", "a0000-oldest.md"), decision("Oldest", "decisions/a0002-new-way.md"))

	findings := Lint(kb, LintOptions{HasCode: false})
	var hits []report.Finding
	for _, f := range findings {
		if f.RuleID == "PYLON066" {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("PYLON066 findings = %d (%v), want 1", len(hits), hits)
	}
	if hits[0].File != "decisions/a0001-old-way.md" {
		t.Fatalf("PYLON066 landed on %q, want the live superseded doc", hits[0].File)
	}
	if hits[0].Grade != report.GradeAdvisory {
		t.Fatalf("PYLON066 grade = %q, want advisory", hits[0].Grade)
	}
}

// Contract claims: one contract id must have exactly one claiming
// package doc, or the map is ambiguous about who owns the surface.
// Pending entries are exempt: they are candidate claims, not owners.
func TestLintFlagsContractClaimCollisions(t *testing.T) {
	kb := t.TempDir()
	write := func(rel, exposes string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\ntitle: " + rel + "\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\npackage: " + filepath.Base(rel) + "\ntags: []\n" + exposes + "---\n\nPackage body.\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\nschemes: []\n")
	// Twice in one doc, then again from a second doc: both extras flag.
	write(filepath.Join("packages", "alpha.md"), "exposes:\n  - kind: func\n    id: alpha.DoThing\n  - kind: func\n    id: alpha.DoThing\n")
	write(filepath.Join("packages", "beta.md"), "exposes:\n  - kind: func\n    id: alpha.DoThing\n")
	// Pending re-declaration is a candidate, not a claim.
	write(filepath.Join("packages", "gamma.md"), "exposes:\n  - kind: func\n    id: alpha.DoThing\n    pending: true\n")
	// A unique claim never flags.
	write(filepath.Join("packages", "delta.md"), "exposes:\n  - kind: func\n    id: delta.Other\n")

	findings := Lint(kb, LintOptions{HasCode: false})
	byFile := map[string]int{}
	for _, f := range findings {
		if f.RuleID != "PYLON067" {
			continue
		}
		byFile[f.File]++
		if f.Grade != report.GradeForced {
			t.Fatalf("PYLON067 on %s is not FORCED: %+v", f.File, f)
		}
	}
	if byFile["packages/alpha.md"] != 1 {
		t.Fatalf("alpha within-doc duplicate findings = %d, want 1", byFile["packages/alpha.md"])
	}
	if byFile["packages/beta.md"] != 1 {
		t.Fatalf("beta cross-doc duplicate findings = %d, want 1", byFile["packages/beta.md"])
	}
	for _, quiet := range []string{"packages/gamma.md", "packages/delta.md"} {
		if byFile[quiet] != 0 {
			t.Fatalf("PYLON067 fired on %s, want silent", quiet)
		}
	}
}

// Archived claimants are history: a live doc re-claiming a contract
// id an archived package doc once exposed is the normal end of a
// version transition, not a collision. The map's ownership surface
// is live docs only.
func TestLintContractClaimsSkipArchive(t *testing.T) {
	kb := t.TempDir()
	write := func(rel, exposes string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\ntitle: " + rel + "\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\npackage: " + filepath.Base(rel) + "\ntags: []\n" + exposes + "---\n\nPackage body.\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\nschemes: []\n")
	write(filepath.Join("packages", "archive", "legacy.md"), "exposes:\n  - kind: func\n    id: old.Thing\n")
	write(filepath.Join("packages", "live.md"), "exposes:\n  - kind: func\n    id: old.Thing\n")

	for _, f := range Lint(kb, LintOptions{HasCode: false}) {
		if f.RuleID == "PYLON067" {
			t.Fatalf("PYLON067 fired across an archive boundary: %+v", f)
		}
	}
}
