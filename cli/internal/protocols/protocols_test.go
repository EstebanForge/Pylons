// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package protocols

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListAndRenderEmbeddedReflect(t *testing.T) {
	names := List("")
	if len(names) != 2 || names[0] != "agent" || names[1] != "reflect" {
		t.Fatalf("expected [agent reflect] in embedded protocols, got %v", names)
	}

	out, err := Render("reflect", Data{
		Root:       "/tmp/kb",
		Bin:        "./bin/pylon",
		Extensions: []string{"go", "typescript"},
		Projects: []ProjectInfo{
			{Alias: "core-api", Ref: "main", Tier: "core"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Render(reflect) failed: %v", err)
	}
	for _, want := range []string{"Reflection Time", "/tmp/kb", "core-api", "go, typescript", "./bin/pylon lint --ci"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected rendered reflect to contain %q", want)
		}
	}
}

func TestRenderKBProtocolsDirTakesPrecedence(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "protocols"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "protocols", "reflect.md"), []byte("KB reflect for {{.Root}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "protocols", "custom.md"), []byte("Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	names := List(kb)
	if len(names) != 3 || names[0] != "agent" || names[1] != "custom" || names[2] != "reflect" {
		t.Fatalf("expected [agent custom reflect], got %v", names)
	}
	out, err := Render("reflect", Data{Root: kb}, "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "KB reflect for "+kb+"\n" {
		t.Fatalf("expected kbRoot/protocols/reflect.md to win, got %q", out)
	}
}

func TestRenderOverrideTemplate(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.tmpl")
	if err := os.WriteFile(custom, []byte("Custom reflect for {{.Root}} ({{len .Projects}} projects)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Render("reflect", Data{Root: "/kb"}, custom)
	if err != nil {
		t.Fatalf("Render override failed: %v", err)
	}
	if out != "Custom reflect for /kb (0 projects)\n" {
		t.Fatalf("unexpected override output: %q", out)
	}
}

func TestRenderUnknownProtocolFails(t *testing.T) {
	if _, err := Render("nonexistent", Data{}, ""); err == nil {
		t.Fatal("expected error for unknown protocol")
	}
	if _, err := Render("../../etc/passwd", Data{Root: t.TempDir()}, ""); err == nil {
		t.Fatal("expected error for traversal protocol name")
	}
}

func TestListAndRenderEmbeddedAgent(t *testing.T) {
	names := List("")
	if !slices.Contains(names, "agent") {
		t.Fatalf("expected agent in embedded protocols, got %v", names)
	}
	out, err := Render("agent", Data{
		Root:       "/tmp/kb",
		Bin:        "pylon",
		Extensions: []string{"go"},
		Projects: []ProjectInfo{
			{Alias: "svc", Ref: "main"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Render(agent) failed: %v", err)
	}
	for _, want := range []string{"Agent Session Loop", "/tmp/kb", "svc", "go", "pylon check", "lint --changed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected rendered agent protocol to contain %q", want)
		}
	}
}
