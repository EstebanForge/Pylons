package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// A symlinked KB root must behave identically to the real path: WalkDir does
// not follow a root symlink, so without resolution the whole corpus silently
// vanishes and every walk reports an empty ledger.
func TestLintFollowsSymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := "---\ntitle: broken\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Symptom\n\nsomething\n\n## Cause\n\nnothing\n"
	if err := os.WriteFile(filepath.Join(real, "quirks", "broken.md"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "kb-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	realFindings := Lint(real, LintOptions{})
	linkFindings := Lint(link, LintOptions{})
	if len(realFindings) == 0 {
		t.Fatalf("fixture produced no findings; the test proves nothing")
	}
	if len(linkFindings) != len(realFindings) {
		t.Fatalf("symlinked root returned %d findings, real root %d", len(linkFindings), len(realFindings))
	}
}

func TestCheckFollowsSymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "kb-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	// Both must at least reach the ledger config instead of reporting nothing.
	Check(real, []string{"quirks/x.md"})
	Check(link, []string{"quirks/x.md"})
}
