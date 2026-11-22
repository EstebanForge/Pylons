// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// With generation disabled the manifest verbs must no-op: no write,
// exit 0, so the shipped gate hooks stay byte-identical and simply
// have nothing to do. Adopters with foreign manifests in those files
// depend on this.
func TestManifestToggleDisablesGeneration(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\ngenerate: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin", "pylon"))
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"rules", "packages"} {
		out, err := exec.Command(bin, "manifest", verb, "--pylons", kb).CombinedOutput()
		if err != nil {
			t.Fatalf("manifest %s: %v\n%s", verb, err, out)
		}
		if _, statErr := os.Stat(filepath.Join(kb, "manifests", verb+".json")); !os.IsNotExist(statErr) {
			t.Errorf("manifest %s wrote a file despite generate: []", verb)
		}
		// --check passes too: a disabled manifest is never stale.
		out, err = exec.Command(bin, "manifest", verb, "--check", "--pylons", kb).CombinedOutput()
		if err != nil {
			t.Fatalf("manifest %s --check: %v\n%s", verb, err, out)
		}
	}
}

func TestManifestDefaultStillWrites(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin", "pylon"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "manifest", "rules", "--pylons", kb).CombinedOutput()
	if err != nil {
		t.Fatalf("manifest rules: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(kb, "manifests", "rules.json")); statErr != nil {
		t.Fatal("manifest rules must write rules.json when generation is enabled")
	}
}

func TestUpdateBaselineFlagPath(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "plans", "bad.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin", "pylon"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "lint", "--update-baseline", "--pylons", kb).CombinedOutput()
	if err != nil {
		t.Fatalf("lint --update-baseline: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(kb, "manifests", "baseline.json")); statErr != nil {
		t.Fatal("--update-baseline must write manifests/baseline.json")
	}
	// The seeded debt no longer blocks.
	out, err = exec.Command(bin, "lint", "--pylons", kb).CombinedOutput()
	if err != nil {
		t.Fatalf("lint with a fresh baseline still failed: %v\n%s", err, out)
	}
}

func TestSelfUpdateFlagPath(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	// Installed pylon at 1.0.0 with its manifest.
	seedProductTree(t, kb)
	seedManifest(t, kb, "1.0.0")
	// Staged 1.1.0 with one changed product file.
	seedProductTree(t, staged)
	if err := os.WriteFile(filepath.Join(staged, "AGENTS.md"), []byte("# guidelines v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedManifest(t, staged, "1.1.0")
	if err := os.MkdirAll(filepath.Join(staged, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin", "pylon"))
	if err != nil {
		t.Fatal(err)
	}
	// Dry run first: nothing written.
	out, err := exec.Command(bin, "self-update", "--from", staged, "--dry-run", "--pylons", kb).CombinedOutput()
	if err != nil {
		t.Fatalf("self-update --dry-run: %v\n%s", err, out)
	}
	if body, _ := os.ReadFile(filepath.Join(kb, "AGENTS.md")); string(body) != "# AGENTS.md\n" {
		t.Fatal("dry run must not write")
	}
	// Real run.
	if out, err := exec.Command(bin, "self-update", "--from", staged, "--pylons", kb).CombinedOutput(); err != nil {
		t.Fatalf("self-update: %v\n%s", err, out)
	}
	if body, _ := os.ReadFile(filepath.Join(kb, "AGENTS.md")); string(body) != "# guidelines v2\n" {
		t.Fatal("self-update must replace the unmodified product file")
	}
	// Missing --from is a usage error.
	if out, err := exec.Command(bin, "self-update", "--pylons", kb).CombinedOutput(); err == nil {
		t.Fatalf("self-update without --from must fail, got: %s", out)
	}
	// manifest product renders the release manifest.
	out, err = exec.Command(bin, "manifest", "product", "--release-version", "9.9.9", "--pylons", kb).CombinedOutput()
	if err != nil {
		t.Fatalf("manifest product: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "\"version\": \"9.9.9\"") {
		t.Fatalf("manifest product output = %s", out)
	}
}

func seedProductTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"manifests/pylons.yaml", "AGENTS.md", "LICENSE", "NOTICE", "bin/THIRD_PARTY_NOTICES",
		"manifests/rules.json", "manifests/vocabulary.json", "manifests/parity.json",
		"manifests/extensions.example.json", "manifests/packages.example.json",
		"manifests/pylons.example.yaml", "protocols/agent.md", "protocols/reflect.md",
		"meta/MANIFESTO.md", "templates/plan.md", "extensions/_examples/pylon-ext-js",
		"_examples/ledger/README.md",
		"packages/AGENTS.md", "proposals/AGENTS.md", "plans/AGENTS.md",
		"decisions/AGENTS.md", "conventions/AGENTS.md", "quirks/AGENTS.md",
		"domain/AGENTS.md", "investigations/AGENTS.md",
		"references/AGENTS.md", "tickets/AGENTS.md",
		"prompts/AGENTS.md", "skills/AGENTS.md",
		"packages/archive/.gitkeep", "plans/archive/.gitkeep",
		"decisions/proposals/.gitkeep", "decisions/archive/.gitkeep",
		"conventions/proposals/.gitkeep", "conventions/archive/.gitkeep",
		"quirks/archive/.gitkeep",
		"domain/entities/.gitkeep", "domain/archive/.gitkeep",
		"investigations/archive/.gitkeep",
		"references/archive/.gitkeep", "tickets/archive/.gitkeep"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# "+p+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func seedManifest(t *testing.T, root, version string) {
	t.Helper()
	// Reuse the engine through the CLI so the manifest matches
	// exactly what a release would ship.
	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "bin", "pylon"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "manifest", "product", "--release-version", version, "--pylons", root).Output()
	if err != nil {
		t.Fatalf("manifest product: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, "manifests", "product.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGateSurfaceFlagPath(t *testing.T) {
	// The guard flows through the CLI flag path: a diff touching a
	// gate surface exits 1, a clean diff exits 0, and an unreadable
	// base degrades to the advisory at exit 0.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun := func(args ...string) string {
		t.Helper()
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
	gitRun("commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(dir, "manifests", "ratchet.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "-Af", ".")
	gitRun("commit", "-qm", "touch ratchet")
	if code := run([]string{"review", "gate-surface", "--base", "HEAD~1", "--pylons", dir}); code != 1 {
		t.Fatalf("gate-surface over a touched ratchet must exit 1, got %d", code)
	}
	if code := run([]string{"review", "gate-surface", "--base", "HEAD", "--pylons", dir}); code != 0 {
		t.Fatalf("gate-surface over a clean diff must exit 0, got %d", code)
	}
	if code := run([]string{"review", "gate-surface", "--base", "no-such-ref", "--pylons", dir}); code != 0 {
		t.Fatalf("an unreadable base must stay advisory at exit 0, got %d", code)
	}
	if code := run([]string{"review", "gate-surface", "--base", "no-such-ref", "--ci", "--pylons", dir}); code != 1 {
		t.Fatalf("under --ci an unreadable base must fail the run, got %d", code)
	}
}

func TestLintBaselineBannerNeverPollutesJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "---\ntitle: q\ntype: quirk\nstatus: active\n---\n\npassword: hunter2hunter2\n"
	if err := os.WriteFile(filepath.Join(dir, "quirks", "q.md"), []byte(bad), 0o644); err != nil {
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
	// Absorb the advisory so the baseline actually suppresses something.
	cmd := exec.Command("../../../bin/pylon", "lint", "--update-baseline", "--pylons", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-baseline: %v\n%s", err, out)
	}
	out, err := exec.Command("../../../bin/pylon", "lint", "--format=json", "--pylons", dir).Output()
	if err != nil {
		t.Fatalf("lint with a suppressing baseline must exit 0: %v\n%s", err, out)
	}
	line := strings.TrimSpace(string(out))
	if !json.Valid([]byte(line)) {
		t.Fatalf("stdout must be valid JSON, got %q", line)
	}
}

func TestLintRejectsStrayPositionals(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"lint", "extra", "--pylons", dir}); code != 2 {
		t.Fatalf("a stray positional must be a usage error, got %d", code)
	}
}
