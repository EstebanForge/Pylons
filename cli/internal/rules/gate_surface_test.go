// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// gateKB builds a committed git repo holding a KB so gate-surface has a
// base ref to diff against. It returns the kb root and the base sha.
func gateKB(t *testing.T) (string, string) {
	t.Helper()
	kb := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", kb}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nexclude:\n  - old/**\n")
	mustWrite(t, filepath.Join(kb, "manifests/rules.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "domain/machine.md"), "# machine\n")
	run("add", "-Af", ".")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")
	return kb, base[:len(base)-1]
}

func findingRules(fs []report.Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.RuleID+" "+f.File)
	}
	return out
}

func TestGateSurfaceFlagsManifests(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, "manifests/ratchet.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "docs/other.md"), "# note\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "touch ratchet")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	fs := GateSurface(kb, base)
	found := findingRules(fs)
	if len(found) != 1 || found[0] != "PYLON062 manifests/ratchet.json" {
		t.Fatalf("expected exactly the ratchet finding, got %v", found)
	}
	for _, f := range fs {
		if f.Severity != report.SeverityError || f.Grade != report.GradeForced {
			t.Fatalf("PYLON062 must be error/FORCED: %+v", f)
		}
	}
}

func TestGateSurfaceFlagsHooksAndKnobs(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, ".githooks/pre-commit"), "#!/usr/bin/env bash\nexit 0\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nexclude:\n  - old/**\n  - everything/**\nschemes: [legacy]\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "widen")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	fs := GateSurface(kb, base)
	found := findingRules(fs)
	if len(found) != 2 {
		t.Fatalf("expected hook and knob findings, got %v", found)
	}
	joined := found[0] + "\n" + found[1]
	if !strings.Contains(joined, "PYLON062 .githooks/pre-commit") || !strings.Contains(joined, "PYLON062 manifests/pylons.yaml") {
		t.Fatalf("missing hook or knob finding: %v", found)
	}
}

func TestGateSurfaceCleanSilent(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, "docs/only.md"), "# note\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "docs only")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	if fs := GateSurface(kb, base); len(fs) != 0 {
		t.Fatalf("doc-only diff must stay silent, got %v", findingRules(fs))
	}
}

func TestGateSurfaceUnresolvableBaseAdvises(t *testing.T) {
	kb, _ := gateKB(t)
	fs := GateSurface(kb, "definitely-not-a-ref")
	if len(fs) != 1 {
		t.Fatalf("expected one advisory, got %v", findingRules(fs))
	}
	if fs[0].RuleID != "PYLON063" || fs[0].Severity != report.SeverityAdvisory {
		t.Fatalf("expected advisory PYLON063, got %+v", fs[0])
	}
}

func TestSecretAndGateNeverBaselined(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	fs := []report.Finding{
		errFinding("PYLON060", "domain/x.md", 3, "possible credential: rotate it; deleting the line does not remove it from git history"),
		errFinding("PYLON062", "manifests/ratchet.json", 1, "gate surface changed"),
		errFinding("PYLON018", "investigations/a.md", 9, "finding row 1 is invalid"),
	}
	if err := UpdateBaseline(kb, fs); err != nil {
		t.Fatal(err)
	}
	kept, accepted, _ := ApplyBaseline(kb, fs)
	if accepted != 1 {
		t.Fatalf("only the ordinary finding may be accepted, got %d", accepted)
	}
	if len(kept) != 2 {
		t.Fatalf("secret and gate findings must survive the baseline, kept %d", len(kept))
	}
}

func TestPackagesManifestEmptyIsArray(t *testing.T) {
	// The generated manifest must stay a JSON array even with zero
	// packages: null breaks every consumer that indexes it.
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	out := BuildPackagesManifest(kb)
	if out == nil {
		t.Fatal("BuildPackagesManifest must return an empty slice, not nil")
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty packages manifest must marshal as [], got %s", data)
	}
}

func TestMergeDangerNotEvaluatedWithoutGit(t *testing.T) {
	// A KB whose project diffs cannot be read degrades to the
	// not-evaluated advisory; it never reads as a clean verdict.
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n")
	plan := "---\ntitle: ship\ntype: plan\nstatus: accepted\ncreated: 2026-11-01\nupdated: 2026-11-01\nowners: [Pylons]\ncomponents: [svc]\nticket: [EX-1]\ncontract:\n  stable:\n    - kind: go-schema\n      id: repo://svc/schema.sql::tables\n---\n\n## Problem\n\nShip it.\n"
	mustWrite(t, filepath.Join(kb, "plans/feature-ship.md"), plan)
	if err := os.MkdirAll(filepath.Join(kb, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(kb, "svc/schema.sql"), "CREATE TABLE ship;\n")
	body := "---\ntitle: body\n---\n\nMerge danger: two-way | blast radius: docs\n"
	mustWrite(t, filepath.Join(kb, "body.md"), body)
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: filepath.Join(kb, "body.md"), Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if strings.Contains(f.Message, "MERGE_DANGER_NOT_EVALUATED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a not-evaluated advisory, got %v", fs)
	}
}
