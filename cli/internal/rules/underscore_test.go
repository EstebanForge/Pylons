package rules

import (
	"path/filepath"
	"strings"
	"testing"
)

// A directory starting with _ is example or scratch space: no walker of the
// CLI may treat anything under it as live ledger content.
func TestUnderscoreDirsAreNotCorpus(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: []\nprojects: {}\n")
	mustWrite(t, filepath.Join(kb, "plans/proposals/real.md"),
		"---\ntitle: \"Real\"\ntype: plan\nstatus: proposed\nupdated: 2026-11-21\ntags: []\n---\n\n# Real plan\n")
	mustWrite(t, filepath.Join(kb, "plans/_examples/broken.md"), "no frontmatter at all\n")
	mustWrite(t, filepath.Join(kb, "domain/_examples/entity.md"), "---\ntitle: \"Ghost\"\n---\n\n# Ghost entity with machines: junk\n")
	mustWrite(t, filepath.Join(kb, "quirks/_examples/quirk.md"), "---\ntitle: \"Q\"\n---\n\n# Q\n")

	findings := Lint(kb, LintOptions{})
	for _, f := range findings {
		if strings.Contains(filepath.ToSlash(f.File), "_examples") {
			t.Fatalf("underscore dir leaked into lint findings: %+v", f)
		}
	}
}
