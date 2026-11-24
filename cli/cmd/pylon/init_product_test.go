package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The product layer travels with the binary: init copies templates,
// machine manifests, protocols, and the reference pages into the fresh
// ledger so pylon new and the lint rules work from the first command.
func TestInitSeedsWorkingLedgerFromProductLayer(t *testing.T) {
	src := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		path := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("manifests/pylons.yaml", "version: 1\n")
	mustWrite("templates/quirk.md", "---\ntitle: quirk\n---\n")
	mustWrite("manifests/vocabulary.json", "{}\n")
	mustWrite("manifests/parity.json", "{}\n")
	mustWrite("manifests/rules.json", "{}\n")
	mustWrite("protocols/agent.md", "# loop\n")
	mustWrite("pylons-docs/adopting.md", "# adopting\n")
	mustWrite("extensions/_examples/README.md", "# examples\n")

	restore := productLayerForInit
	productLayerForInit = func() string { return src }
	defer func() { productLayerForInit = restore }()

	dest := t.TempDir()
	if code := cmdInit(dest); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	for _, rel := range []string{
		"templates/quirk.md", "manifests/vocabulary.json", "manifests/parity.json",
		"manifests/rules.json", "protocols/agent.md", "pylons-docs/adopting.md",
		"extensions/_examples/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s missing after init: %v", rel, err)
		}
	}
	// The dogfood ledger's own pylons.yaml must never ride along.
	if _, err := os.Stat(filepath.Join(dest, "manifests", "pylons.yaml")); err != nil {
		t.Errorf("fresh pylons.yaml missing: %v", err)
	}
}

func TestInitStillScaffoldsWithoutProductLayer(t *testing.T) {
	restore := productLayerForInit
	productLayerForInit = func() string { return "" }
	defer func() { productLayerForInit = restore }()

	dest := t.TempDir()
	if code := cmdInit(dest); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifests", "pylons.yaml")); err != nil {
		t.Errorf("pylons.yaml missing: %v", err)
	}
}

// A dangling symlink in the fresh target is never followed: init
// refuses the ledger instead of writing through the link.
func TestInitRefusesDanglingSymlinkManifest(t *testing.T) {
	dest := t.TempDir()
	targets := t.TempDir()
	link := filepath.Join(dest, "manifests", "pylons.yaml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(targets, "pylons-target.yaml"), link); err != nil {
		t.Fatal(err)
	}
	if code := cmdInit(dest); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(targets, "pylons-target.yaml")); !os.IsNotExist(err) {
		t.Fatal("init wrote through a dangling symlink")
	}
}
