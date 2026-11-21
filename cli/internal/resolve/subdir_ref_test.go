// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitFixture builds a real commit inside dir and returns a helper
// running git with a pinned identity.
func gitFixture(t *testing.T, dir string) func(args ...string) string {
	t.Helper()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_COUNT=3",
			"GIT_CONFIG_KEY_0=commit.gpgsign", "GIT_CONFIG_VALUE_0=false",
			"GIT_CONFIG_KEY_1=core.hooksPath", "GIT_CONFIG_VALUE_1=",
			"GIT_CONFIG_KEY_2=core.excludesFile", "GIT_CONFIG_VALUE_2=",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	return git
}

// TestSubdirectoryProjectResolvesFromRef registers a project at a
// crate subdirectory of a workspace repo. The ref must keep working:
// git resolves upward, so the project is still a git project.
func TestSubdirectoryProjectResolvesFromRef(t *testing.T) {
	root := t.TempDir()
	git := gitFixture(t, root)
	git("init", "-b", "main")
	crate := filepath.Join(root, "crates", "core")
	if err := os.MkdirAll(crate, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(crate, "lib.rs")
	if err := os.WriteFile(src, []byte("pub struct Vault;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-f", ".")
	git("commit", "-m", "init")

	// The worktree copy is gone; only the ref still carries the file.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}

	ref := "main"
	if data, ok := ReadProjectFile(crate, ref, "lib.rs"); !ok || len(data) == 0 {
		t.Fatal("ReadProjectFile must resolve a subdirectory project file from the ref")
	}
	if !StatProjectFile(crate, ref, "lib.rs") {
		t.Fatal("StatProjectFile must see a subdirectory project file in the ref")
	}
	v := resolveFile(crate, ref, KindFile, "repo://x/lib.rs")
	if !v.Resolved {
		t.Fatalf("file contract = %+v, want resolved from the ref", v)
	}
	v = resolveGlob(crate, ref, KindGlob, "repo://x/*.rs")
	if !v.Resolved {
		t.Fatalf("glob contract = %+v, want resolved from the ref", v)
	}
}
