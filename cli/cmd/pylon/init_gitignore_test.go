package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSeedsGitkeepInEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	if code := cmdInit(dir); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	for _, sub := range []string{"plans/archive", "quirks/archive", "packages/archive", "conventions/proposals"} {
		if _, err := os.Stat(filepath.Join(dir, sub, ".gitkeep")); err != nil {
			t.Errorf("%s/.gitkeep missing: %v", sub, err)
		}
	}
	// Seed-file dirs carry content and must not gain a redundant keeper.
	if _, err := os.Stat(filepath.Join(dir, "conventions", ".gitkeep")); !os.IsNotExist(err) {
		t.Error("conventions/ got .gitkeep despite seed content")
	}
}

func TestInitSeedsGitignore(t *testing.T) {
	dir := t.TempDir()
	if code := cmdInit(dir); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore not seeded: %v", err)
	}
	if !strings.Contains(string(b), "._*") {
		t.Errorf(".gitignore missing ._ pattern: %q", b)
	}

	existing := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(existing, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdInit(dir); code != 0 {
		t.Fatalf("second cmdInit exit = %d", code)
	}
	b, _ = os.ReadFile(existing)
	if string(b) != "custom\n" {
		t.Errorf("existing .gitignore overwritten: %q", b)
	}
}
