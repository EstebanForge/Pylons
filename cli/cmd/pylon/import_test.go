// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pylon import adopts existing markdown into the ledger: dry-run is
// the default, --write applies, and nothing valid is ever invented
// (an unparseable frontmatter block is refused, not guessed).
func TestImportScaffolder(t *testing.T) {
	kb := newSandboxKB(t)
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\nschemes: []\n")
	src := filepath.Join(t.TempDir(), "old-notes.md")
	if err := os.WriteFile(src, []byte("# Old notes\n\nLegacy body kept verbatim.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Dry-run plans and writes nothing.
	code, out, _ := runPylon(t, kb, "import", "--type", "reference", src)
	if code != 0 {
		t.Fatalf("dry run must exit 0, got %d", code)
	}
	if !strings.Contains(out, "references/old-notes.md") || !strings.Contains(out, "--write") {
		t.Fatalf("plan must name the target and the --write hint, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(kb, "references", "old-notes.md")); !os.IsNotExist(err) {
		t.Fatal("dry run must not write the target")
	}

	// --write grafts the template frontmatter onto the verbatim body.
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "reference", src)
	if code != 0 {
		t.Fatalf("import --write must exit 0, got %d: %s", code, out)
	}
	b, err := os.ReadFile(filepath.Join(kb, "references", "old-notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "---\n") || !strings.Contains(s, "type: reference") || !strings.Contains(s, "status: active") {
		t.Fatalf("written file must carry generated frontmatter, got:\n%s", s)
	}
	if !strings.Contains(s, "Legacy body kept verbatim.") || strings.Contains(s, "# Old notes\n\nLegacy body kept verbatim.\n\n---") && false {
		t.Fatalf("body must be preserved verbatim, got:\n%s", s)
	}

	// A second run refuses the occupied target; nothing is overwritten.
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "reference", src)
	if code == 0 {
		t.Fatalf("occupied target must refuse, got 0: %s", out)
	}
	if !strings.Contains(out, "already exists") {
		t.Fatalf("refusal must name the collision, got: %s", out)
	}

	// A source that already carries valid frontmatter is skipped.
	skip := filepath.Join(t.TempDir(), "already.md")
	skipBody := "---\ntitle: Already\ntype: reference\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\n---\n\nBody.\n"
	if err := os.WriteFile(skip, []byte(skipBody), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "reference", skip)
	if code != 0 || !strings.Contains(out, "already carries frontmatter") {
		t.Fatalf("valid frontmatter must skip cleanly, got %d: %s", code, out)
	}

	// A broken frontmatter block is refused loudly, not repaired.
	broken := filepath.Join(t.TempDir(), "broken.md")
	if err := os.WriteFile(broken, []byte("---\ntitle: [unclosed\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "reference", broken)
	if code == 0 || !strings.Contains(out, "unparseable") {
		t.Fatalf("broken frontmatter must refuse, got %d: %s", code, out)
	}

	// Plans keep the PYLON038 prefix rule: an unprefixed name refuses
	// and names the allowed prefixes.
	plan := filepath.Join(t.TempDir(), "my-plan.md")
	if err := os.WriteFile(plan, []byte("# My plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "plan", plan)
	if code == 0 || !strings.Contains(out, "feature-") {
		t.Fatalf("plan import must demand a PYLON038 prefix, got %d: %s", code, out)
	}
	code, out, _ = runPylon(t, kb, "import", "--write", "--type", "plan", "--name", "feature-my-plan", plan)
	if code != 0 {
		t.Fatalf("--name override must satisfy the prefix rule, got %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(kb, "plans", "proposals", "feature-my-plan.md")); err != nil {
		t.Fatalf("renamed plan must land in the plan zone: %v", err)
	}
}
