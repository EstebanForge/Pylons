// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "manifests", "pylons.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParitySpecLoads(t *testing.T) {
	root := writeManifest(t, `version: 1
extensions:
  - go
  - kotlin
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
    kotlin:
      use: external:kotlin
      options:
        marker: "pylon:machine"
      extensions:
        - .kt
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Parity == nil {
		t.Fatal("parity spec not decoded")
	}
	if cfg.Parity.Version != 1 {
		t.Fatalf("parity version = %d", cfg.Parity.Version)
	}
	got, ok := cfg.Parity.Profiles["go"]
	if !ok || got.Use != "builtin:go" {
		t.Fatalf("go profile = %+v", got)
	}
	kotlin, ok := cfg.Parity.Profiles["kotlin"]
	if !ok || kotlin.Use != "external:kotlin" {
		t.Fatalf("kotlin profile = %+v", kotlin)
	}
	if kotlin.Options["marker"] != "pylon:machine" {
		t.Fatalf("kotlin options = %+v", kotlin.Options)
	}
}

func TestParityUnknownTopLevelKeyFailsClosed(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  version: 1
  proflies:
    go:
      use: builtin:go
`)
	if _, err := Load(root); err == nil {
		t.Fatal("typo'd parity key must fail closed")
	} else if !strings.Contains(err.Error(), "parity") {
		t.Fatalf("error should name the parity section: %v", err)
	}
}

func TestParityUnknownProfileKeyFailsClosed(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
      marker: sneaky
`)
	if _, err := Load(root); err == nil {
		t.Fatal("unknown profile key must fail closed")
	}
}

func TestParityVersionRequiredAndSingle(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  profiles:
    go:
      use: builtin:go
`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("missing parity.version must error: %v", err)
	}
	root = writeManifest(t, `version: 1
parity:
  version: 2
  profiles:
    go:
      use: builtin:go
`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "supported") {
		t.Fatalf("unsupported parity.version must name the supported set: %v", err)
	}
}

func TestParityUseValidation(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  version: 1
  profiles:
    go:
      use: builtin:brainfuck
`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "builtin") {
		t.Fatalf("unknown builtin language must error: %v", err)
	}
	root = writeManifest(t, `version: 1
extensions:
  - kotlin
parity:
  version: 1
  profiles:
    kotlin:
      use: external:../evil
`)
	if _, err := Load(root); err == nil {
		t.Fatal("external profile name with a path must error")
	}
	root = writeManifest(t, `version: 1
parity:
  version: 1
  profiles:
    kotlin:
      use: external:pylon-ext-kotlin
`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("external profile naming a disabled extension must error: %v", err)
	}
}

func TestParityExtensionOwnershipConflict(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
    rust:
      use: builtin:rust
      extensions:
        - .go
`)
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), ".go") {
		t.Fatalf("two profiles claiming one extension must error: %v", err)
	}
}

func TestParityProjectOverride(t *testing.T) {
	root := writeManifest(t, `version: 1
extensions:
  - go
  - rust
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
projects:
  app:
    path: app
    extensions:
      - gowp
    parity:
      version: 1
      profiles:
        go:
          use: external:gowp
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	proj, ok := cfg.Projects["app"]
	if !ok || proj.Parity == nil {
		t.Fatal("project parity spec not decoded")
	}
	if proj.Parity.Profiles["go"].Use != "external:gowp" {
		t.Fatalf("project override = %+v", proj.Parity.Profiles["go"])
	}
}

func TestParityProfileForBinding(t *testing.T) {
	// No parity section: enabled builtins bind implicitly.
	root := writeManifest(t, `version: 1
extensions:
  - go
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, ok := cfg.ParityProfileFor("", "go")
	if !ok || b.Use != "builtin:go" {
		t.Fatalf("implicit builtin binding = %+v ok=%v", b, ok)
	}
	if _, ok := cfg.ParityProfileFor("", "python"); ok {
		t.Fatal("disabled language must not bind implicitly")
	}
	// An explicit section owns the surface: no implicit fallback.
	root = writeManifest(t, `version: 1
extensions:
  - go
  - rust
parity:
  version: 1
  profiles:
    rust:
      use: builtin:rust
`)
	cfg, err = Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.ParityProfileFor("", "go"); ok {
		t.Fatal("explicit section must disable the implicit builtin binding")
	}
	if b, ok := cfg.ParityProfileFor("", "rust"); !ok || b.Use != "builtin:rust" {
		t.Fatalf("explicit binding = %+v ok=%v", b, ok)
	}
	// Project override wins over global for the same language.
	root = writeManifest(t, `version: 1
extensions:
  - go
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
projects:
  app:
    path: app
    extensions:
      - gowp
    parity:
      version: 1
      profiles:
        go:
          use: external:gowp
`)
	cfg, err = Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b, _ := cfg.ParityProfileFor("app", "go"); b.Use != "external:gowp" {
		t.Fatalf("project binding should win: %+v", b)
	}
	if b, _ := cfg.ParityProfileFor("other", "go"); b.Use != "builtin:go" {
		t.Fatalf("global binding for other projects: %+v", b)
	}
}

func TestParityMalformedSpecFailsClosed(t *testing.T) {
	root := writeManifest(t, `version: 1
parity:
  version: 1
  profiles:
    - not-a-map
`)
	if _, err := Load(root); err == nil {
		t.Fatal("malformed profiles shape must fail closed")
	}
}
