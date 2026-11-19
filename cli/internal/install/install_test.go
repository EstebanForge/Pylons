package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func stubBinary(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin", "pylon")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func hookFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, name := range []string{"pre-commit", "pre-push"} {
		b, err := os.ReadFile(filepath.Join(root, ".githooks", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		m[name] = string(b)
	}
	return m
}

func hooksPathConfig(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "config", "core.hooksPath").Output()
	if err != nil {
		t.Fatalf("read core.hooksPath: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestEnsureHooksFreshRepo(t *testing.T) {
	root := gitInit(t)
	stubBinary(t, root)

	res, err := EnsureHooks(root)
	if err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	if len(res.WroteHooks) != 2 {
		t.Errorf("WroteHooks = %v, want both hook files", res.WroteHooks)
	}
	if len(res.SkippedHooks) != 0 {
		t.Errorf("SkippedHooks = %v, want empty", res.SkippedHooks)
	}
	if !res.HooksPathSet {
		t.Error("HooksPathSet = false, want true")
	}
	if !res.BinaryOK {
		t.Error("BinaryOK = false, want true")
	}

	files := hookFiles(t, root)
	for name, body := range files {
		if len(body) == 0 || body[:2] != "#!" {
			t.Errorf("%s: missing shebang or empty", name)
		}
	}
	if got := hooksPathConfig(t, root); got != ".githooks" {
		t.Errorf("core.hooksPath = %q, want .githooks", got)
	}
}

func TestEnsureHooksNoOverwrite(t *testing.T) {
	root := gitInit(t)
	stubBinary(t, root)
	hooksDir := filepath.Join(root, ".githooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(hooksDir, "pre-commit")
	if err := os.WriteFile(keep, []byte("KEEP"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := EnsureHooks(root)
	if err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	if res.WroteHooks == nil || len(res.WroteHooks) != 1 || res.WroteHooks[0] != "pre-push" {
		t.Errorf("WroteHooks = %v, want [pre-push]", res.WroteHooks)
	}
	if len(res.SkippedHooks) != 1 || res.SkippedHooks[0] != "pre-commit" {
		t.Errorf("SkippedHooks = %v, want [pre-commit]", res.SkippedHooks)
	}
	b, err := os.ReadFile(keep)
	if err != nil || string(b) != "KEEP" {
		t.Errorf("existing hook overwritten: %v %q", err, b)
	}
}

func TestEnsureHooksHooksPathAlreadySet(t *testing.T) {
	root := gitInit(t)
	stubBinary(t, root)
	if err := exec.Command("git", "-C", root, "config", "core.hooksPath", ".githooks").Run(); err != nil {
		t.Fatal(err)
	}

	res, err := EnsureHooks(root)
	if err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	if res.HooksPathSet {
		t.Error("HooksPathSet = true, want false for already-configured repo")
	}
}

func TestEnsureHooksNoRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureHooks(dir); err == nil {
		t.Fatal("EnsureHooks outside a git repo: want error, got nil")
	}
}

func TestEnsureHooksBinaryMissing(t *testing.T) {
	root := gitInit(t)
	res, err := EnsureHooks(root)
	if err != nil {
		t.Fatalf("EnsureHooks: %v", err)
	}
	if res.BinaryOK {
		t.Error("BinaryOK = true, want false when bin/pylon absent")
	}
}

func TestEmbeddedAssetsMatchRepoHooks(t *testing.T) {
	for _, name := range []string{"pre-commit", "pre-push"} {
		repo, err := os.ReadFile(filepath.Join("..", "..", "..", ".githooks", name))
		if err != nil {
			t.Skipf("repo .githooks/%s not present from this checkout: %v", name, err)
		}
		emb, err := assets.ReadFile("assets/" + name)
		if err != nil {
			t.Fatalf("embedded %s: %v", name, err)
		}
		if string(repo) != string(emb) {
			t.Errorf(".githooks/%s drifted from embedded assets; sync both copies", name)
		}
	}
}
