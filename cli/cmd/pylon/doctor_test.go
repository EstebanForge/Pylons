// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/install"
)

// doctorLedger builds a git ledger whose product layer is complete
// enough that doctor --fix can clear every error-grade finding: the
// exact files exist, bin/pylon is executable, hooks are fixable, and
// only the zone docs, keepers, and generated manifests are broken.
func doctorLedger(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/pylons.yaml", "version: 1\nschemes: []\n")
	for _, rel := range install.ExactFiles() {
		write(rel, rel+"\n")
	}
	write("bin/pylon", "#!/bin/sh\necho 0\n")
	if err := os.Chmod(filepath.Join(dir, "bin", "pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("manifests/rules.json", "{not json\n")
	if out, err := exec.Command("git", "init", "-q", "-b", "main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestDoctorCommandE2E(t *testing.T) {
	dir := doctorLedger(t)

	// Broken: exit 1 with the structural findings in human output.
	code, out, _ := runPylon(t, dir, "doctor", "--pylons", ".")
	if code != 1 {
		t.Fatalf("doctor exit = %d on a broken ledger, want 1; output:\n%s", code, out)
	}
	if !containsAll(out, "DOCTOR002", "DOCTOR010", "== ERROR ==") {
		t.Fatalf("doctor output misses the structural findings:\n%s", out)
	}

	// JSON mode parses and carries the same ids.
	code, out, _ = runPylon(t, dir, "doctor", "--pylons", ".", "--format", "json")
	if code != 1 {
		t.Fatalf("doctor --format json exit = %d, want 1", code)
	}
	var fs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &fs); err != nil {
		t.Fatalf("doctor --format json does not parse: %v\n%s", err, out)
	}
	seen := map[string]bool{}
	for _, f := range fs {
		seen[f.ID] = true
	}
	if !seen["DOCTOR002"] {
		t.Fatalf("json findings miss DOCTOR002: %v", seen)
	}

	// Fix clears every error-grade finding on this ledger.
	code, out, _ = runPylon(t, dir, "doctor", "--fix", "--pylons", ".")
	if code != 0 {
		t.Fatalf("doctor --fix exit = %d, want 0; output:\n%s", code, out)
	}
	if !containsAll(out, "DOCTOR002: wrote", "DOCTOR010: regenerated") {
		t.Fatalf("fix output misses the repair lines:\n%s", out)
	}

	// Healthy rerun: exit 0, advisory at most.
	code, out, _ = runPylon(t, dir, "doctor", "--fix", "--pylons", ".")
	if code != 0 {
		t.Fatalf("healthy doctor exit = %d, want 0; output:\n%s", code, out)
	}
	if !containsAll(out, "nothing fixable to repair") {
		t.Fatalf("converged fix must report nothing to repair:\n%s", out)
	}

	// Sarif is not a doctor surface.
	code, _, _ = runPylon(t, dir, "doctor", "--pylons", ".", "--format", "sarif")
	if code != 2 {
		t.Fatalf("doctor --format sarif exit = %d, want 2", code)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestDoctorJsonHygieneAndRepairExit(t *testing.T) {
	dir := doctorLedger(t)

	// Broken: json stdout parses as findings with no prose.
	code, out, _ := runPylon(t, dir, "doctor", "--pylons", ".", "--format", "json")
	if code != 1 {
		t.Fatalf("doctor json exit = %d, want 1", code)
	}
	var fs []map[string]any
	if err := json.Unmarshal([]byte(out), &fs); err != nil {
		t.Fatalf("json stdout is not pure JSON: %v\n%s", err, out)
	}
	if len(fs) == 0 {
		t.Fatal("json findings empty on a broken ledger")
	}

	// Healthy: complete the layer doctor cannot build (trees, product
	// record, valid baseline), let --fix write the rest, then stdout
	// must be exactly [].
	for _, rel := range install.RequiredTreeFiles() {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tree := range install.ProductTrees() {
		filler := filepath.Join(dir, filepath.FromSlash(tree), "filler.md")
		if err := os.MkdirAll(filepath.Dir(filler), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filler, []byte("filler\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/product.json", `{"version": "test", "files": {"LICENSE": "x"}}`+"\n")
	write("manifests/baseline.json", `{"version": 1}`+"\n")

	code, out, _ = runPylon(t, dir, "doctor", "--fix", "--pylons", ".", "--format", "json")
	if code != 0 {
		t.Fatalf("doctor --fix exit = %d, want 0\nstdout:\n%s", code, out)
	}
	var after []map[string]any
	if err := json.Unmarshal([]byte(out), &after); err != nil {
		t.Fatalf("post-fix json stdout is not pure JSON: %v\n%s", err, out)
	}
	if string(out) != "[]\n" {
		t.Fatalf("healthy json stdout = %q, want []", out)
	}

	// A failed repair forces exit 1 even with only warnings left: a
	// regular file occupies plans/archive so the keeper mkdir fails.
	if err := os.Remove(filepath.Join(dir, "plans", "archive", ".gitkeep")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "plans", "archive")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plans", "archive"), []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, _ = runPylon(t, dir, "doctor", "--fix", "--pylons", ".")
	if code != 1 {
		t.Fatalf("doctor --fix exit = %d with a failed repair, want 1", code)
	}
}
