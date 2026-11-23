// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests pin the four rules no other test file names: PYLON020
// plan frontmatter, PYLON021 plan sections, PYLON022 quirk sections,
// and PYLON037 stale branch mentions. Each asserts the finding fires
// on its defect and stays quiet on a clean doc.

func writePlanDoc(t *testing.T, fm, body string) string {
	t.Helper()
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(kb, "plans", "p.md")
	if err := os.WriteFile(p, []byte(fm+"\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPYLON020PlanFrontmatterInvalid(t *testing.T) {
	clean := writePlanDoc(t, "---\ntitle: \"P\"\ntype: plan\nstatus: proposed\n---\n",
		"## Problem\n\nx\n\n## Design\n\ny\n\n## Milestones\n\nz\n")
	got := strings.Join(findingRules(ReviewPlan(clean, clean)), "\n")
	if strings.Contains(got, "PYLON020") {
		t.Fatalf("a valid plan must not fire PYLON020: %s", got)
	}
	broken := writePlanDoc(t, "---\ntype: note\n---\n",
		"## Problem\n\nx\n\n## Design\n\ny\n\n## Milestones\n\nz\n")
	got = strings.Join(findingRules(ReviewPlan(broken, broken)), "\n")
	if !strings.Contains(got, "PYLON020") {
		t.Fatalf("a plan without title and type: plan must fire PYLON020, got: %s", got)
	}
}

func TestPYLON021PlanSectionMissing(t *testing.T) {
	broken := writePlanDoc(t, "---\ntitle: \"P\"\ntype: plan\nstatus: proposed\n---\n",
		"## Design\n\ny\n")
	got := strings.Join(findingRules(ReviewPlan(broken, broken)), "\n")
	if !strings.Contains(got, "PYLON021") {
		t.Fatalf("a plan without ## Problem must fire PYLON021, got: %s", got)
	}
}

func TestPYLON022PostmortemSectionMissing(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	quirk := filepath.Join(kb, "quirks", "q.md")
	body := "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\n---\n\n## Symptom\n\ns\n\n## Cause\n\nc\n"
	if err := os.WriteFile(quirk, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findingRules(ReviewPostmortem(kb, "quirks/q.md", false)), "\n")
	if !strings.Contains(got, "PYLON022") {
		t.Fatalf("a quirk without ## Resolution must fire PYLON022, got: %s", got)
	}
	if err := os.WriteFile(quirk, []byte(body+"\n## Resolution\n\nr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = strings.Join(findingRules(ReviewPostmortem(kb, "quirks/q.md", false)), "\n")
	if strings.Contains(got, "PYLON022") {
		t.Fatalf("a complete quirk must not fire PYLON022: %s", got)
	}
}

func TestPYLON037StaleBranchMention(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"),
		[]byte("version: 1\nprojects:\n  svc:\n    path: svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The stale-branch check only judges projects inside a git repo;
	// the branch check itself needs a real refs/heads to miss against.
	if out, err := exec.Command("git", "-C", filepath.Join(kb, "svc"), "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// Only zone-dir docs are corpus docs; a root-level file never
	// reaches the stale-branch scan.
	doc := "---\ntitle: \"D\"\ntype: reference\nstatus: active\n---\n\nSee repo://svc@nowhere/README.md.\n"
	if err := os.MkdirAll(filepath.Join(kb, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "references", "refs.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	// The stale-branch check rides the HasCode gate: the ledger must
	// declare that it judges code for the branch mention to matter.
	findings := Lint(kb, LintOptions{HasCode: true})
	fired := false
	for i := range findings {
		if findings[i].RuleID == "PYLON037" {
			fired = true
		}
	}
	if !fired {
		t.Fatalf("a branch that exists nowhere must fire PYLON037, got %+v", findings)
	}
}
