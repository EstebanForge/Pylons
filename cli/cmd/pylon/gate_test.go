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

func gateKB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "svc", "main.go"), []byte("package svc\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// README links cure the orphan rule for the fixture docs.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# KB\n\nSee [the quirk](quirks/q.md) and [the plan](plans/proposals/feature-p.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quirks", "q.md"), []byte("---\ntitle: q\ntype: quirk\nstatus: active\npackage: p\nguard: test\nsources: []\nai_authored_contributed: no\nvolume_pressure: none\n---\n\n## Symptom\n\ns\n\n## Cause\n\nc\n\n## Resolution\n\nr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "packages"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A package doc gives packages.json real content, so mutating the
	// doc makes the committed manifest stale.
	if err := os.WriteFile(filepath.Join(dir, "packages", "svc.md"), []byte("---\ntitle: Svc\ntype: package\nstatus: active\npackage: svc\nrepo: repo://svc\ntier: core\ncreated: 2026-11-01\nupdated: 2026-11-20\nowners: [o]\ncomponents: [svc]\ndepends_on: []\nexposes:\n  - kind: go-symbol\n    id: repo://svc/main.go::Run\ndescription: d\ntags: [p]\n---\n\n# Svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	gitRun("init", "-q", "-b", "main")
	gitRun("config", "user.email", "t@example.com")
	gitRun("config", "user.name", "t")
	gitRun("add", "-Af", ".")
	gitRun("commit", "-qm", "kb")
	// Regenerate the manifests so the committed ones are fresh.
	if code := run([]string{"manifest", "rules", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest rules: %d", code)
	}
	if code := run([]string{"manifest", "packages", "--pylons", dir}); code != 0 {
		t.Fatalf("manifest packages: %d", code)
	}
	gitRun("add", "-Af", ".")
	gitRun("commit", "-qm", "manifests")
	return dir
}

func TestGateDispatchRequiresStage(t *testing.T) {
	dir := gateKB(t)
	if code := run([]string{"gate", "--pylons", dir}); code != 2 {
		t.Fatalf("gate without a stage must exit 2, got %d", code)
	}
	if code := run([]string{"gate", "nonsense", "--pylons", dir}); code != 2 {
		t.Fatalf("gate with an unknown stage must exit 2, got %d", code)
	}
}

func TestGatePrePushCleanPasses(t *testing.T) {
	dir := gateKB(t)
	if code := run([]string{"gate", "pre-push", "--pylons", dir}); code != 0 {
		t.Fatalf("a clean gate pre-push must exit 0, got %d", code)
	}
}

func TestGatePrePushBlocksOnStaleManifest(t *testing.T) {
	dir := gateKB(t)
	// A package-doc mutation makes the committed packages.json stale.
	if err := os.WriteFile(filepath.Join(dir, "packages", "svc.md"), []byte("---\ntitle: Svc\ntype: package\nstatus: deprecated\npackage: svc\nrepo: repo://svc\ntier: service\ncreated: 2026-11-01\nupdated: 2026-11-21\nowners: [o]\ncomponents: [svc]\ndepends_on: []\nexposes:\n  - kind: go-symbol\n    id: repo://svc/main.go::Run\ndescription: d\ntags: [p]\n---\n\n# Svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"gate", "pre-push", "--pylons", dir}); code != 1 {
		t.Fatalf("a stale manifest must block the push gate, got %d", code)
	}
}

func TestGatePreCommitAutoStagesManifests(t *testing.T) {
	dir := gateKB(t)
	if err := os.WriteFile(filepath.Join(dir, "quirks", "q.md"), []byte("---\ntitle: q2\ntype: quirk\nstatus: active\npackage: p\nguard: test\nsources: []\nai_authored_contributed: no\nvolume_pressure: none\n---\n\n## Symptom\n\ns\n\n## Cause\n\nc\n\n## Resolution\n\nr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Mutating the package doc drifts packages.json.
	if err := os.WriteFile(filepath.Join(dir, "packages", "svc.md"), []byte("---\ntitle: Svc\ntype: package\nstatus: deprecated\npackage: svc\nrepo: repo://svc\ntier: service\ncreated: 2026-11-01\nupdated: 2026-11-21\nowners: [o]\ncomponents: [svc]\ndepends_on: []\nexposes:\n  - kind: go-symbol\n    id: repo://svc/main.go::Run\ndescription: d\ntags: [p]\n---\n\n# Svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "plans", "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plans", "proposals", "feature-p.md"), []byte("---\ntitle: p\ntype: plan\nstatus: proposed\ncreated: 2026-11-01\nupdated: 2026-11-20\nowners: [o]\nticket: [T-1]\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# P\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := exec.Command("git", "-C", dir, "add", "-Af", ".")
	if out, err := g.CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if code := run([]string{"gate", "pre-commit", "--pylons", dir}); code != 0 {
		t.Fatalf("pre-commit must pass and auto-stage, got %d", code)
	}
	out, err := exec.Command("git", "-C", dir, "diff", "--cached", "--name-only").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "manifests/packages.json") {
		t.Fatalf("the drifted manifest must be auto-staged, staged=%s", string(out))
	}
}
