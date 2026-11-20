// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A KB nested inside a larger repo must refuse: hooks would gate the
// outer repo, not the KB.
func TestEnsureHooksRefusesNestedKB(t *testing.T) {
	outer := gitInit(t)
	inner := filepath.Join(outer, "kb")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureHooks(inner, false)
	if err == nil || errors.Is(err, ErrNotGit) {
		t.Fatalf("nested KB must be refused, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outer, ".githooks")); !os.IsNotExist(err) {
		t.Fatalf("outer repo must stay untouched: %v", err)
	}
}

// An existing core.hooksPath owned by another tool is a conflict unless
// force is set.
func TestEnsureHooksHooksPathConflict(t *testing.T) {
	root := gitInit(t)
	if err := exec.Command("git", "-C", root, "config", "core.hooksPath", ".husky").Run(); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureHooks(root, false)
	if !errors.Is(err, ErrHooksPathConflict) {
		t.Fatalf("want ErrHooksPathConflict, got %v", err)
	}
	cfg, err := exec.Command("git", "-C", root, "config", "--get", "core.hooksPath").Output()
	if err != nil || string(cfg) != ".husky\n" {
		t.Fatalf("existing hooksPath must stay: %v %q", err, cfg)
	}
	res, err := EnsureHooks(root, true)
	if err != nil {
		t.Fatalf("force install: %v", err)
	}
	if len(res.WroteHooks) != len(hookNames) {
		t.Fatalf("force should write hooks, got %v", res.WroteHooks)
	}
	cfg, err = exec.Command("git", "-C", root, "config", "--get", "core.hooksPath").Output()
	if err != nil || string(cfg) != ".githooks\n" {
		t.Fatalf("force should claim hooksPath, got %v %q", err, cfg)
	}
}
