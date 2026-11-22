// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarvestFlagPath(t *testing.T) {
	base := t.TempDir()
	kb := filepath.Join(base, "kb")
	for _, d := range []string{filepath.Join(kb, "manifests"), filepath.Join(kb, "svc", "src"), filepath.Join(kb, "packages")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(kb, "packages", "core.md"), []byte("---\ntitle: Core\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: core\ncomponents: [svc]\n---\nThe core package.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "svc", "src", "api.go"), []byte("package src\n\nfunc Known() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(base, "pr.diff")
	if err := os.WriteFile(diffPath, []byte("+++ b/svc/src/api.go\n@@ -1,2 +1,3 @@\n func Known() {}\n+func Fresh() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(base, "meta.json")
	if err := os.WriteFile(metaPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Missing --diff is a usage error.
	if code := run([]string{"harvest", "pr", "--pylons", kb}); code != 2 {
		t.Fatalf("missing --diff must exit 2, got %d", code)
	}
	// Unparsable metadata fails closed.
	if code := run([]string{"harvest", "pr", "--pylons", kb, "--diff", diffPath, "--metadata", metaPath}); code != 2 {
		t.Fatalf("unparsable metadata must exit 2, got %d", code)
	}
	// Unknown subcommand is a usage error.
	if code := run([]string{"harvest", "nope", "--pylons", kb, "--diff", diffPath}); code != 2 {
		t.Fatalf("unknown harvest subcommand must exit 2, got %d", code)
	}

	if err := os.WriteFile(metaPath, []byte(`{"commits":[{"subject":"fix: thing","body":"Footgun: retry storms on replay."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout := captureStdout(t, func() {
		if code := run([]string{"harvest", "pr", "--pylons", kb, "--diff", diffPath, "--metadata", metaPath}); code != 0 {
			t.Fatalf("a real harvest must exit 0, got %d", code)
		}
	})
	if !strings.Contains(stdout, "repo://svc/src/api.go::Fresh") {
		t.Fatalf("artifact must carry the proposal id, got: %s", stdout)
	}
	if !strings.Contains(stdout, "quirks/proposals/") {
		t.Fatalf("artifact must name the quirk proposal path, got: %s", stdout)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, rerr := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(buf)
}
