package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// A checkout registered through a symlinked path and a query through the
// realpath are one directory; attribution and protected-glob checks must not
// depend on which name the caller happened to use.
func TestCheckTargetAttributionSurvivesSymlinks(t *testing.T) {
	kb := t.TempDir()
	svc := filepath.Join(kb, "svc")
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(svc, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc-link\n"
	link := filepath.Join(kb, "svc-link")
	if err := os.Symlink(svc, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(svc, "main.go")
	got := normalizeCheckTarget(kb, target, cfg)
	if got.alias != "svc" {
		t.Fatalf("realpath query through a symlinked registration attributed %q, want svc", got.alias)
	}
}
