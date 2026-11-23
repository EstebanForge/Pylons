package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// A checkout registered through a symlinked path and a query through the
// realpath are one directory; attribution and protected-glob checks must not
// depend on which name the caller happened to use.
func TestCheckTargetAttributionSurvivesSymlinks(t *testing.T) {
	kb := t.TempDir()
	svc := filepath.Join(kb, "svc")
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(svc, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc-link\n"
	link := filepath.Join(kb, "svc-link")
	if err := os.Symlink(svc, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(svc, "main.go")
	got := normalizeCheckTarget(kb, target, cfg)
	if got.alias != "svc" {
		t.Fatalf("realpath query through a symlinked registration attributed %q, want svc", got.alias)
	}
}

// A protected glob must also catch a file that does not exist yet: agents
// check the target before writing it, and the query rides the same
// symlinked name the project was registered under.
func TestCheckTargetProtectedGlobCatchesUnwrittenFile(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc-link\nprotected:\n  - {glob: src/**, owner: human-only}\n"
	// The link must be live: a dangling link leaves both comparison
	// sides unresolved and the test passes without exercising the bug.
	if err := os.MkdirAll(filepath.Join(kb, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(kb, "svc-link")
	if err := os.Symlink(filepath.Join(kb, "svc"), link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	// src/ does not exist yet; the file is only about to be created.
	target := filepath.Join(link, "src", "new.go")
	got := normalizeCheckTarget(kb, target, cfg)
	if got.alias != "svc" {
		t.Fatalf("unwritten file through the symlinked name attributed %q, want svc", got.alias)
	}
	if got.path != "src/new.go" {
		t.Fatalf("unwritten file path came back %q, want src/new.go", got.path)
	}
}

// Lexical cleaning before symlink resolution collapses .. through a
// link: proj/outside_link/../src/x.go resolves on disk to a file OUTSIDE
// the project, and the attribution must follow the disk, not the text.
func TestCheckTargetDotDotThroughSymlinkAttribution(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "kb", "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "kb", "manifests", "pylons.yaml"),
		[]byte("version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+filepath.Join(base, "real")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "src", "api.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// real/outside_link/../src lands at base/src: outside the project.
	if err := os.Symlink(filepath.Join(base, "elsewhere"), filepath.Join(real, "outside_link")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(base, "kb", "packages", "core.md"), `---
title: "Core"
type: package
status: active
package: core
components: [svc]
protected:
  - glob: "src/**"
    owner: human-only
    reason: "vault keys"
---

# Core
`)
	kb := filepath.Join(base, "kb")
	// real/outside_link/../src/api.go lives at base/src/api.go on disk:
	// outside the project, so it must stay unattributed. String concat
	// keeps the .. on the wire: filepath.Join would Clean it away before
	// the engine sees it.
	target := real + "/outside_link/../src/api.go"
	findings := Check(kb, []string{target})
	for _, f := range findings {
		if f.RuleID == "PYLON029" {
			t.Fatalf("a target resolving outside the project must not attribute to it: %+v", f)
		}
	}
}
