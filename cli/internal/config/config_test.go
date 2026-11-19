// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAndProjectExtensions(t *testing.T) {
	root := t.TempDir()
	cfgYAML := `version: 1
extensions:
  - go
projects:
  core-api:
    path: ../core-api
    ref: main
    tier: core
    extensions:
      - typescript
  web-ui:
    path: /abs/web-ui
    ref: develop
`
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifests", "pylons.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("Version = %d, want 1", cfg.Version)
	}
	if !cfg.HasExtension("", "go") {
		t.Fatal("expected global extension go to be enabled")
	}
	if cfg.HasExtension("web-ui", "typescript") {
		t.Fatal("did not expect typescript on web-ui")
	}
	if !cfg.HasExtension("core-api", "typescript") {
		t.Fatal("expected per-project extension typescript on core-api")
	}
	if !cfg.HasExtension("core-api", "go") {
		t.Fatal("expected global extension go on core-api")
	}

	wantCorePath := filepath.Clean(filepath.Join(root, "../core-api"))
	if got := cfg.ProjectPath(root, "core-api"); got != wantCorePath {
		t.Fatalf("ProjectPath(core-api) = %q, want %q", got, wantCorePath)
	}
	if got := cfg.ProjectPath(root, "web-ui"); got != "/abs/web-ui" {
		t.Fatalf("ProjectPath(web-ui) = %q, want /abs/web-ui", got)
	}
	if got := cfg.ProjectRef("web-ui"); got != "develop" {
		t.Fatalf("ProjectRef(web-ui) = %q, want develop", got)
	}
}

func TestLoadMissingConfigReturnsDefault(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("expected nil error for missing pylons.yaml, got %v", err)
	}
	if cfg.Version != 1 || len(cfg.Projects) != 0 {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}
