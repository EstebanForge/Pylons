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

func TestSchemesValidation(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{"valid", "version: 1\nschemes: [legacy, atlas]\n", false},
		{"empty ok", "version: 1\nschemes: []\n", false},
		{"reserved repo", "version: 1\nschemes: [repo]\n", true},
		{"reserved pylon", "version: 1\nschemes: [pylon]\n", true},
		{"reserved http", "version: 1\nschemes: [http]\n", true},
		{"bad grammar", "version: 1\nschemes: [1bad]\n", true},
		{"upper", "version: 1\nschemes: [Legacy]\n", true},
	}
	for _, tc := range cases {
		kb := t.TempDir()
		writeToggleManifest(t, kb, tc.yaml)
		_, err := Load(kb)
		if tc.wantErr && err == nil {
			t.Errorf("%s: want error, got none", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: want ok, got %v", tc.name, err)
		}
	}
}

func TestExcludeValidation(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{"valid", "version: 1\nexclude: [legacy/**, notes/*.md]\n", false},
		{"empty pattern", "version: 1\nexclude: ['']\n", true},
		{"absolute", "version: 1\nexclude: [/etc]\n", true},
		{"traversal", "version: 1\nexclude: [../out]\n", true},
	}
	for _, tc := range cases {
		kb := t.TempDir()
		writeToggleManifest(t, kb, tc.yaml)
		_, err := Load(kb)
		if tc.wantErr && err == nil {
			t.Errorf("%s: want error, got none", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: want ok, got %v", tc.name, err)
		}
	}
}

func TestDecisionPrefixValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"version: 1\n":                         true, // absent means D
		"version: 1\ndecision_prefix: A\n":     true,
		"version: 1\ndecision_prefix: OK\n":    true,
		"version: 1\ndecision_prefix: \"\"\n":  true,
		"version: 1\ndecision_prefix: abc\n":   false, // lowercase
		"version: 1\ndecision_prefix: ABCDE\n": false,
		"version: 1\ndecision_prefix: A1\n":    false,
	}
	for manifest, want := range cases {
		if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(dir)
		if want && err != nil {
			t.Errorf("%q: unexpected error %v", manifest, err)
		}
		if !want && err == nil {
			t.Errorf("%q: expected a validation error", manifest)
		}
	}
}
