// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitInitAt(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func TestCheckHooksReportsUnwiredRepo(t *testing.T) {
	dir := t.TempDir()
	gitInitAt(t, dir)
	st := CheckHooks(dir)
	if !st.InGitRepo {
		t.Fatal("a git repository must report InGitRepo")
	}
	if len(st.Missing) != len(hookNames) {
		t.Fatalf("Missing = %v; an unwired repo misses every hook", st.Missing)
	}
	if st.HooksPath != "" {
		t.Fatalf("HooksPath = %q; want unset", st.HooksPath)
	}
}

func TestCheckHooksReportsWiredRepo(t *testing.T) {
	dir := t.TempDir()
	gitInitAt(t, dir)
	if _, err := EnsureHooks(dir, false); err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	st := CheckHooks(dir)
	if len(st.Missing) != 0 || len(st.Drifted) != 0 {
		t.Fatalf("Missing = %v, Drifted = %v; a wired repo is clean", st.Missing, st.Drifted)
	}
	if st.HooksPath != ".githooks" {
		t.Fatalf("HooksPath = %q; want .githooks", st.HooksPath)
	}
}

func TestCheckHooksReportsDrift(t *testing.T) {
	dir := t.TempDir()
	gitInitAt(t, dir)
	if _, err := EnsureHooks(dir, false); err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	hook := filepath.Join(dir, ".githooks", hookNames[0])
	if err := os.WriteFile(hook, []byte("# locally edited\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := CheckHooks(dir)
	if len(st.Drifted) != 1 || st.Drifted[0] != hookNames[0] {
		t.Fatalf("Drifted = %v; the edited hook must report as drift", st.Drifted)
	}
}

func TestCheckHooksOutsideGit(t *testing.T) {
	st := CheckHooks(t.TempDir())
	if st.InGitRepo {
		t.Fatal("a plain directory must report InGitRepo false")
	}
}
