// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
