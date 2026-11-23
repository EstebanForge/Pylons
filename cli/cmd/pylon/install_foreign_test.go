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

// writeStubBinary plants an executable bin/pylon so the missing-binary
// refusal does not mask the behavior under test.
func writeStubBinary(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func foreignRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeForeignHooks(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pre-commit", "pre-push"} {
		p := filepath.Join(root, ".githooks", name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho foreign\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// A KB whose every gate hook is foreign runs none of the pylon
// gates. Install must refuse loudly instead of quietly succeeding;
// --force is the explicit takeover.
func TestInstallBlocksWhenAllGateHooksAreForeign(t *testing.T) {
	root := foreignRepo(t)
	writeForeignHooks(t, root)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if code := cmdInstall(root, false); code == 0 {
		t.Fatal("cmdInstall exit 0 with every gate hook foreign; want refusal")
	}
	if code := cmdInstall(root, true); code != 0 {
		t.Fatalf("cmdInstall --force exit %d; want 0 (explicit takeover)", code)
	}
}

// One foreign hook among our own gates is a warning, not a block:
// the pylon gates still run for the hooks we own.
func TestInstallMixedHooksStillSucceed(t *testing.T) {
	root := foreignRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, ".githooks", "pre-commit")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho foreign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if code := cmdInstall(root, false); code != 0 {
		t.Fatalf("cmdInstall exit %d with one foreign hook; want 0 (warn only)", code)
	}
}

func captureInstallOutput(t *testing.T, fn func()) string {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	eR, eW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, eW
	fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	w.Close()
	eW.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	for {
		n, err := eR.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	r.Close()
	return string(buf)
}

func TestInstallForceRecordsPreviousHooksPath(t *testing.T) {
	root := foreignRepo(t)
	writeForeignHooks(t, root)
	set := exec.Command("git", "-C", root, "config", "core.hooksPath", "custom")
	if out, err := set.CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureInstallOutput(t, func() { code = cmdInstall(root, true) })
	if code != 0 {
		t.Fatalf("cmdInstall --force exit %d; want 0", code)
	}
	if !strings.Contains(out, "custom") {
		t.Fatalf("the replaced core.hooksPath value must be recorded, got: %s", out)
	}
}

func TestInstallMixedHooksNamesTheChainLine(t *testing.T) {
	root := foreignRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".githooks", "pre-commit"), []byte("#!/bin/sh\necho foreign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureInstallOutput(t, func() { code = cmdInstall(root, false) })
	if code != 0 {
		t.Fatalf("cmdInstall exit %d; want 0", code)
	}
	if !strings.Contains(out, "bin/pylon gate pre-commit || exit 1") {
		t.Fatalf("the mixed case must name the chain line, got: %s", out)
	}
}

// Wiring core.hooksPath disables any legacy .git/hooks scripts; the
// handover must say so instead of silently benching them.
func TestInstallWarnsOnLegacyGitHooks(t *testing.T) {
	top := foreignRepo(t)
	writeForeignHooks(t, top)
	writeStubBinary(t, top)
	if err := os.MkdirAll(filepath.Join(top, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nlegacy\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStubBinary(t, top)
	code, _, stderr := runPylon(t, top, "install", "--pylons", top, "--force")
	if code != 0 {
		t.Fatalf("install must succeed with --force, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, ".git/hooks/pre-commit") {
		t.Fatalf("a legacy .git/hooks script must be named in a warning: %s", stderr)
	}
}

// Fail closed before wiring: gates for an engine that cannot run are
// the confusing success the write order caused.
func TestInstallRefusesMissingBinaryBeforeWiring(t *testing.T) {
	top := foreignRepo(t)
	writeForeignHooks(t, top)
	code, _, stderr := runPylon(t, top, "install", "--pylons", top, "--force")
	if code != 1 {
		t.Fatalf("a missing binary must refuse install, got %d", code)
	}
	body, err := os.ReadFile(filepath.Join(top, ".githooks", "pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "#!/bin/sh\necho foreign\n" {
		t.Fatal("the foreign hook must stay untouched when the binary is missing")
	}
	if !strings.Contains(stderr, "bin/pylon missing") {
		t.Fatalf("the refusal must name the build step: %s", stderr)
	}
}

// A --force takeover must not write through a symlinked hook file: the
// link is replaced by a real file, and the link target stays untouched.
func TestInstallForceReplacesSymlinkedHook(t *testing.T) {
	top := foreignRepo(t)
	writeForeignHooks(t, top)
	writeStubBinary(t, top)
	target := filepath.Join(t.TempDir(), "elsewhere-hook")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nforeign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(top, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(top, ".githooks", "pre-commit")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	writeStubBinary(t, top)
	code, _, stderr := runPylon(t, top, "install", "--pylons", top, "--force")
	if code != 0 {
		t.Fatalf("--force must take over, got %d: %s", code, stderr)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the hook must be a real file after takeover, not a symlink")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "#!/bin/sh\nforeign\n" {
		t.Fatal("the link target must stay untouched")
	}
}
