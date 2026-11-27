// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"
)

// Lint must surface the plan structure rules (PYLON020/PYLON021) on
// the same path set the commit gate reviews. Otherwise the rules are
// invisible until commit time, and `pylon lint` lies about what the
// gate will block on.
func TestLintFlagsPlanStructureOnReviewPaths(t *testing.T) {
	kb := t.TempDir()
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
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\n")
	write(filepath.Join("proposals", "no-design.md"), "---\ntitle: No design\ntype: plan\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Problem\n\np\n\n## Milestones\n\nm\n")
	write(filepath.Join("proposals", "wrong-type.md"), "---\ntitle: Wrong type\ntype: proposal\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Problem\n\np\n")
	write(filepath.Join("proposals", "README.md"), "# Proposals\n\nGuide text without plan sections.\n")
	write(filepath.Join("plans", "regular.md"), "---\ntitle: Regular\ntype: plan\nstatus: accepted\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n## Problem\n\np\n")

	findings := Lint(kb, LintOptions{})

	has := func(rule, file string) bool {
		for _, f := range findings {
			if f.RuleID == rule && f.File == file {
				return true
			}
		}
		return false
	}
	if !has("PYLON021", filepath.ToSlash(filepath.Join("proposals", "no-design.md"))) {
		t.Fatalf("lint must flag missing ## Design in proposals/, got:\n%v", findings)
	}
	if !has("PYLON020", filepath.ToSlash(filepath.Join("proposals", "wrong-type.md"))) {
		t.Fatalf("lint must flag non-plan type in proposals/, got:\n%v", findings)
	}
	for _, f := range findings {
		if f.RuleID == "PYLON020" || f.RuleID == "PYLON021" {
			if f.File != filepath.ToSlash(filepath.Join("proposals", "no-design.md")) && f.File != filepath.ToSlash(filepath.Join("proposals", "wrong-type.md")) {
				t.Fatalf("plan structure rules leaked to %s outside review paths", f.File)
			}
		}
	}
}
