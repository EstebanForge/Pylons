// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// guardrailFixture builds a temp ledger with one tracked project
// (alias alpha, path proj) and a set of quirk documents.
func guardrailFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nextensions: []\nprojects:\n  alpha:\n    path: proj\n"
	if err := os.WriteFile(filepath.Join(root, "manifests", "pylons.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "quirks", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "proj", "src", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	quirk := func(name, sources string) {
		t.Helper()
		body := "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\nsources: " + sources + "\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
		if err := os.WriteFile(filepath.Join(root, "quirks", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	quirk("file-trap.md", "[repo://alpha/src/x.go]")
	quirk("dir-trap.md", "[repo://alpha/src/lib]")
	quirk("dir-slash.md", "[repo://alpha/src/lib/]")
	quirk("other-alias.md", "[repo://beta/src/x.go]")
	quirk("multi.md", "[repo://alpha/src/other.go, repo://alpha/src/lib/deep.go]")
	archived := "---\ntitle: \"Old\"\ntype: quirk\nstatus: active\nsources: [repo://alpha/src/x.go]\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "archive", "old.md"), []byte(archived), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := "Not a quirk: no frontmatter, sources repo://alpha/src/x.go as prose.\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := "---\ntitle: \"P\"\ntype: plan\nstatus: accepted\nsources: [repo://alpha/src/x.go]\ncreated: 2026-11-24\nupdated: 2026-11-24\ncontracts: []\ntags: []\n---\n\n# P\n"
	if err := os.WriteFile(filepath.Join(root, "plans", "p.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestQuirkBranchScopedSource(t *testing.T) {
	root := guardrailFixture(t)
	for _, c := range [][]string{{"git", "init", "-q"}, {"git", "symbolic-ref", "HEAD", "refs/heads/dev"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", c, err, out)
		}
	}
	quirk := func(name, sources string) {
		t.Helper()
		body := "---\ntitle: \"B\"\ntype: quirk\nstatus: active\nsources: " + sources + "\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
		if err := os.WriteFile(filepath.Join(root, "quirks", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	quirk("branch-dev.md", "[repo://alpha@dev/src/x.go]")
	quirk("branch-other.md", "[repo://alpha@other/src/x.go]")

	got := QuirkGuardrails(root, []string{"repo://alpha/src/x.go"})
	want := []string{"quirks/branch-dev.md", "quirks/file-trap.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("on dev = %v, want %v (a branch-pinned source fires only on its branch)", got, want)
	}

	if out, err := exec.Command("git", "-C", root, "symbolic-ref", "HEAD", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("switch branch: %v: %s", err, out)
	}
	got = QuirkGuardrails(root, []string{"repo://alpha/src/x.go"})
	if len(got) != 1 || got[0] != "quirks/file-trap.md" {
		t.Fatalf("on main = %v, want [quirks/file-trap.md]", got)
	}
}

// TestRippleBranchScopedSource pins the quirk to a branch: the ripple
// advisory consumes the same quirk sources and must fire only on that
// branch, the same rule quirk guardrails and the slice apply.
func TestRippleBranchScopedSource(t *testing.T) {
	root := guardrailFixture(t)
	for _, c := range [][]string{{"git", "init", "-q"}, {"git", "symbolic-ref", "HEAD", "refs/heads/dev"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", c, err, out)
		}
	}
	quirk := func(name, sources string) {
		t.Helper()
		body := "---\ntitle: \"B\"\ntype: quirk\nstatus: active\nsources: " + sources + "\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
		if err := os.WriteFile(filepath.Join(root, "quirks", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	quirk("branch-dev.md", "[repo://alpha@dev/src/x.go]")

	mentions := func(fs []report.Finding, needle string) bool {
		for _, f := range fs {
			if f.RuleID == "PYLON044" && strings.Contains(f.Message, needle) {
				return true
			}
		}
		return false
	}
	if got := Check(root, []string{"repo://alpha/src/x.go"}); !mentions(got, "quirks/branch-dev.md") {
		t.Fatalf("on dev the ripple advisory must name quirks/branch-dev.md, got %v", got)
	}

	if out, err := exec.Command("git", "-C", root, "symbolic-ref", "HEAD", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("switch branch: %v: %s", err, out)
	}
	if got := Check(root, []string{"repo://alpha/src/x.go"}); mentions(got, "quirks/branch-dev.md") {
		t.Fatalf("on main the branch-pinned quirk must stay silent, got %v", got)
	}
}

func TestSliceQuirksBranchScoped(t *testing.T) {
	root := guardrailFixture(t)
	for _, c := range [][]string{{"git", "init", "-q"}, {"git", "symbolic-ref", "HEAD", "refs/heads/dev"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", c, err, out)
		}
	}
	body := "---\ntitle: \"B\"\ntype: quirk\nstatus: active\nsources: [repo://alpha@dev/src/x.go]\ncreated: 2026-11-24\nupdated: 2026-11-24\n---\n\n## Symptom\ns\n\n## Cause\nc\n\n## Resolution\nr\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "branch-dev.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := sliceQuirks(root, []string{"alpha"})
	found := false
	for _, g := range got {
		if g == "quirks/branch-dev.md" {
			found = true
		}
		if g == "quirks/other-alias.md" {
			t.Fatalf("slice listed a foreign alias: %v", got)
		}
	}
	if !found {
		t.Fatalf("on dev: branch-scoped quirk missing from %v", got)
	}

	if out, err := exec.Command("git", "-C", root, "symbolic-ref", "HEAD", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("switch branch: %v: %s", err, out)
	}
	for _, g := range sliceQuirks(root, []string{"alpha"}) {
		if g == "quirks/branch-dev.md" {
			t.Fatal("branch-scoped quirk listed on the wrong branch")
		}
	}
}

func TestQuirkGuardrailsMatchesTargets(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/x.go"})
	want := []string{"quirks/file-trap.md"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsDirPrefixBoundary(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/lib/deep.go"})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md", "quirks/multi.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
	// A sibling whose name merely extends the source path must not
	// fire: the prefix boundary is a path boundary, not a string one.
	got = QuirkGuardrails(root, []string{"repo://alpha/src/libx"})
	if len(got) != 0 {
		t.Fatalf("boundary leak: %v", got)
	}
}

func TestQuirkGuardrailsTrailingSlashSourceAndTarget(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{"repo://alpha/src/lib/"})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsAbsoluteTargetAttribution(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{filepath.Join(root, "proj", "src", "x.go")})
	want := []string{"quirks/file-trap.md"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
}

func TestQuirkGuardrailsIgnoresLedgerTargetsAndArchive(t *testing.T) {
	root := guardrailFixture(t)
	for _, targets := range [][]string{
		{"plans/p.md"},
		{"/tmp/unregistered/src/x.go"},
	} {
		if got := QuirkGuardrails(root, targets); len(got) != 0 {
			t.Fatalf("guardrails for %v = %v, want none", targets, got)
		}
	}
	// Alias mismatch: an alpha-quirk never fires on a beta target and
	// vice versa.
	got := QuirkGuardrails(root, []string{"repo://beta/src/x.go"})
	if len(got) != 1 || got[0] != "quirks/other-alias.md" {
		t.Fatalf("guardrails for beta target = %v, want [quirks/other-alias.md]", got)
	}
}

func TestQuirkGuardrailsSortedDistinctAcrossTargets(t *testing.T) {
	root := guardrailFixture(t)
	got := QuirkGuardrails(root, []string{
		"repo://alpha/src/lib/deep.go",
		"repo://alpha/src/x.go",
		"repo://alpha/src/lib/helper.go",
	})
	want := []string{"quirks/dir-slash.md", "quirks/dir-trap.md", "quirks/file-trap.md", "quirks/multi.md"}
	if len(got) != len(want) {
		t.Fatalf("guardrails = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("guardrails = %v, want %v", got, want)
		}
	}
}

// Zone entry guides (AGENTS.md) live in quirks/ as references, not as
// quirks; the 30-day guard clock must not age them into advisories.
func TestQuirkGuardSkipsEntryDocs(t *testing.T) {
	old := resolve.DocMeta{Type: "reference", Status: "active", Created: "2026-07-15"}
	entry := corpusDoc{rel: "quirks/AGENTS.md", meta: old}
	if got := checkQuirkRules(entry); len(got) != 0 {
		t.Fatalf("entry guide must not trip the guard clock, got %v", got)
	}
	quirk := corpusDoc{rel: "quirks/q.md", meta: resolve.DocMeta{Type: "quirk", Status: "active", Created: "2026-07-15"}}
	got := checkQuirkRules(quirk)
	if len(got) != 1 || got[0].RuleID != "PYLON028" {
		t.Fatalf("a real quirk without guard must still fire PYLON028, got %v", got)
	}
}
