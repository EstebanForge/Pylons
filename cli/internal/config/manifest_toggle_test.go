// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeToggleManifest(t *testing.T, kb string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateDefaultEnablesBoth(t *testing.T) {
	kb := t.TempDir()
	writeToggleManifest(t, kb, "version: 1\n")
	cfg, err := Load(kb)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.GeneratesManifest("rules") || !cfg.GeneratesManifest("packages") {
		t.Error("absent generate must enable both manifests")
	}
}

func TestGenerateEmptyListDisablesBoth(t *testing.T) {
	kb := t.TempDir()
	writeToggleManifest(t, kb, "version: 1\ngenerate: []\n")
	cfg, err := Load(kb)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GeneratesManifest("rules") || cfg.GeneratesManifest("packages") {
		t.Error("generate: [] must disable both manifests")
	}
}

func TestGenerateListIsExact(t *testing.T) {
	kb := t.TempDir()
	writeToggleManifest(t, kb, "version: 1\ngenerate: [rules]\n")
	cfg, err := Load(kb)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.GeneratesManifest("rules") {
		t.Error("generate: [rules] must enable rules")
	}
	if cfg.GeneratesManifest("packages") {
		t.Error("generate: [rules] must disable packages")
	}
}

func TestGenerateRejectsUnknownManifest(t *testing.T) {
	kb := t.TempDir()
	writeToggleManifest(t, kb, "version: 1\ngenerate: [bogus]\n")
	if _, err := Load(kb); err == nil {
		t.Fatal("generate must reject manifest names outside the closed set")
	}
}
