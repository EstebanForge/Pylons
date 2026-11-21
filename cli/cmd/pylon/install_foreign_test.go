// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
