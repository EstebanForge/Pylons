// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
)

// Contracts whose components do not map to a registered project must
// surface as PYLON003, not skip silently.
func TestUnregisteredComponentsFlagged(t *testing.T) {
	kb := t.TempDir()
	os.MkdirAll(filepath.Join(kb, "plans"), 0o755)
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects:\n  real:\n    path: ../real\n"), 0o644)
	os.WriteFile(filepath.Join(kb, "plans", "feature-x.md"), []byte("---\ntitle: X\ntype: plan\nstatus: accepted\nticket:\n  - id: T1\ncomponents: [typo-alias]\ncontract:\n  stable:\n    - id: file:src.go\n      wrong_if: never\n---\n\n# X\n"), 0o644)
	findings := Lint(kb, LintOptions{HasCode: true})
	found := false
	for _, f := range findings {
		if f.RuleID == "PYLON003" && strings.Contains(f.Message, "components do not map") {
			found = true
		}
	}
	if !found {
		t.Fatalf("PYLON003 not raised for unregistered components: %v", findings)
	}
}

// staleness --fix must never rewrite updated: lines in body text.
func TestStalenessFixFrontmatterOnly(t *testing.T) {
	content := "---\ntitle: P\ntype: package\nstatus: active\npackage: p\nupdated: 2020-01-01\n---\n\nExample block:\nupdated: 1999-12-31\n"
	patched := patchUpdatedFrontMatter(content, "2026-11-19")
	if !strings.Contains(patched, "updated: 1999-12-31") {
		t.Fatal("body updated: line was rewritten")
	}
	if strings.Contains(patched, "updated: 2020-01-01") {
		t.Fatal("frontmatter updated: was not bumped")
	}
}

// Postmortem headings must be exact lines; ## Problems must not satisfy ## Problem.
func TestPostmortemExactHeadings(t *testing.T) {
	if hasHeading("## Problems\n", "## Problem") {
		t.Fatal("## Problems matched ## Problem")
	}
	if !hasHeading("intro\n## Cause \n", "## Cause") {
		t.Fatal("trimmed heading not matched")
	}
	kb := t.TempDir()
	os.MkdirAll(filepath.Join(kb, "quirks"), 0o755)
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects: {}\n"), 0o644)
	os.WriteFile(filepath.Join(kb, "quirks", "feature-q.md"), []byte("---\ntitle: Q\ntype: quirk\nstatus: active\ncreated: 2026-11-19\nupdated: 2026-11-19\n---\n\n## Problems\n## Causes\n## Resolutions\n"), 0o644)
	findings := ReviewPostmortem(kb, "quirks/feature-q.md", false)
	if len(findings) == 0 {
		t.Fatal("near-miss headings must not satisfy required sections")
	}
}

// Zone entry advisories must come out in deterministic order.
func TestFolderEntryAdvisoriesDeterministic(t *testing.T) {
	kb := t.TempDir()
	for _, zone := range []string{"zz-docs", "aa-docs", "mm-docs"} {
		os.MkdirAll(filepath.Join(kb, zone), 0o755)
		os.WriteFile(filepath.Join(kb, zone, "feature-d.md"), []byte("---\ntitle: D\ntype: reference\nstatus: active\ncreated: 2026-11-19\nupdated: 2026-11-19\n---\n\nBody.\n"), 0o644)
	}
	os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nprojects: {}\n"), 0o644)
	var first []string
	for i := 0; i < 30; i++ {
		var zones []string
		for _, f := range Lint(kb, LintOptions{}) {
			if f.RuleID == "PYLON034" {
				zones = append(zones, f.File)
			}
		}
		if i == 0 {
			first = zones
			continue
		}
		for j := range zones {
			if zones[j] != first[j] {
				t.Fatalf("run %d order differs: %v vs %v", i, zones, first)
			}
		}
	}
	_ = config.Config{}
}

func TestTermNoAvoid(t *testing.T) {
	kb := t.TempDir()
	missing := `---
title: "SessionToken"
type: domain-entity
status: accepted
---

# SessionToken
`
	mustWrite(t, filepath.Join(kb, "domain/entities/session-token.md"), missing)
	findings := Lint(kb, LintOptions{HasCode: false})
	var found bool
	for _, f := range findings {
		if f.RuleID == "PYLON039" {
			found = true
			if f.Severity != report.SeverityAdvisory {
				t.Fatalf("PYLON039 must be advisory, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected PYLON039 for entity without avoid:; got %+v", findings)
	}

	// A populated avoid list clears the finding; other zones are untouched.
	mustWrite(t, filepath.Join(kb, "domain/entities/session-token.md"), `---
title: "SessionToken"
type: domain-entity
status: accepted
avoid:
  - Token
  - Session
---

# SessionToken
`)
	for _, f := range Lint(kb, LintOptions{HasCode: false}) {
		if f.RuleID == "PYLON039" {
			t.Fatalf("avoid list present but PYLON039 fired: %+v", f)
		}
	}

	mustWrite(t, filepath.Join(kb, "plans/feature-x.md"), "---\ntitle: \"X\"\ntype: plan\nstatus: accepted\nticket: [T-1]\n---\n\n# X\n")
	// A live positive control: the un-avoided entity must still fire,
	// and the plan doc must stay out of scope.
	mustWrite(t, filepath.Join(kb, "domain/entities/raw-token.md"), missing)
	var onEntity int
	for _, f := range Lint(kb, LintOptions{HasCode: false}) {
		if f.RuleID == "PYLON039" {
			onEntity++
			if !strings.Contains(f.File, "domain/") {
				t.Fatalf("PYLON039 must only fire on domain entities: %+v", f)
			}
		}
	}
	if onEntity != 1 {
		t.Fatalf("PYLON039 fired %d times, want 1 (the un-avoided entity only)", onEntity)
	}
}

func TestContractUnproven(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "svc.go"), "package svc\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(proj, "svc_test.go"), "package svc\nfunc TestPing() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n")

	mustWrite(t, filepath.Join(kb, "plans/feature-unproven.md"), `---
title: "Unproven"
type: plan
status: accepted
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: symbol
      id: repo://svc/svc.go::Ping
      wrong_if: "Ping disappears or changes signature"
    - kind: route
      id: repo://svc/svc.go::Ping
      wrong_if: "route path changes"
    - kind: glob
      id: repo://svc/**
      wrong_if: "glob content changes"
    - kind: ts-export
      id: repo://svc/svc.go::Ping
      wrong_if: "export disappears"
    - kind: rust-item
      id: repo://svc/svc.go::Ping
      wrong_if: "item disappears"
---

# Unproven

Body.
`)

	var fired int
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON040" {
			fired++
			if f.Severity != report.SeverityAdvisory {
				t.Fatalf("PYLON040 must be advisory, got %s", f.Severity)
			}
		}
	}
	// Provable: symbol, route, ts-export, rust-item; glob is not provable.
	if fired != 4 {
		t.Fatalf("PYLON040 fired %d times, want 4", fired)
	}

	mustWrite(t, filepath.Join(kb, "plans/feature-unproven.md"), `---
title: "Unproven"
type: plan
status: accepted
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: symbol
      id: repo://svc/svc.go::Ping
      wrong_if: "Ping disappears or changes signature"
      verified_by: svc_test.go::TestPing
    - kind: route
      id: repo://svc/svc.go::Ping
      wrong_if: "route path changes"
      falsified_by: svc_test.go::TestPing
    - kind: glob
      id: repo://svc/**
      wrong_if: "glob content changes"
---

# Unproven

Body.
`)
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON040" {
			t.Fatalf("pointers present but PYLON040 fired: %+v", f)
		}
	}
}

func TestPostmortemFieldTokens(t *testing.T) {
	kb := t.TempDir()
	base := func(ai, vp string) string {
		return "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nai_authored_contributed: \"" + ai + "\"\nvolume_pressure: \"" + vp + "\"\n---\n\n# Q\n\n## Symptom\n\nS\n\n## Cause\n\nC\n\n## Resolution\n\nR\n"
	}

	mustWrite(t, filepath.Join(kb, "quirks/q-bad.md"), base("sometimes", "huge"))
	findings := ReviewPostmortem(kb, "", false)
	var fired int
	for _, f := range findings {
		if f.RuleID == "PYLON041" {
			fired++
			if f.Severity != report.SeverityError {
				t.Fatalf("PYLON041 must be error, got %s", f.Severity)
			}
		}
	}
	if fired != 2 {
		t.Fatalf("PYLON041 fired %d times, want 2 (one per bad token)", fired)
	}

	mustWrite(t, filepath.Join(kb, "quirks/q-bad.md"), base("yes", "review"))
	for _, f := range ReviewPostmortem(kb, "", false) {
		if f.RuleID == "PYLON041" {
			t.Fatalf("valid tokens but PYLON041 fired: %+v", f)
		}
	}

	// Missing fields never gate: the values are human-declared, not inferred.
	mustWrite(t, filepath.Join(kb, "quirks/q-absent.md"), "---\ntitle: \"Q2\"\ntype: quirk\nstatus: active\ncreated: 2026-11-01\n---\n\n# Q2\n\n## Symptom\n\nS\n\n## Cause\n\nC\n\n## Resolution\n\nR\n")
	for _, f := range ReviewPostmortem(kb, "", false) {
		if f.RuleID == "PYLON041" {
			t.Fatalf("absent fields must not fire PYLON041: %+v", f)
		}
	}
}

func TestAdvisoryRatchet(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	mustWrite(t, filepath.Join(proj, "svc.go"), "package svc\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n")
	// One old guard-less quirk seeds an advisory that scales per document.
	oldQuirk := "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\ncreated: %s\n---\n\n# Q\n\n## Symptom\n\nS\n\n## Cause\n\nC\n\n## Resolution\n\nR\n"
	mustWrite(t, filepath.Join(kb, "quirks/q-old1.md"), fmt.Sprintf(oldQuirk, "2020-01-01"))

	findings, err := RunRatchet(kb, RatchetOptions{Update: true})
	if err != nil {
		t.Fatalf("baseline update failed: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("--update must produce no findings, got %+v", findings)
	}

	// Same corpus: ratchet passes.
	findings, err = RunRatchet(kb, RatchetOptions{})
	if err != nil {
		t.Fatalf("ratchet failed: %v", err)
	}
	for _, f := range findings {
		if f.Severity == report.SeverityError {
			t.Fatalf("unchanged corpus must pass: %+v", f)
		}
	}

	// A second guard-less quirk in the same zone: ratchet must fail closed.
	mustWrite(t, filepath.Join(kb, "quirks/q-old2.md"), fmt.Sprintf(oldQuirk, "2020-02-02"))
	findings, err = RunRatchet(kb, RatchetOptions{})
	if err != nil {
		t.Fatalf("ratchet failed: %v", err)
	}
	var regressed bool
	for _, f := range findings {
		if f.RuleID == "PYLON042" && f.Severity == report.SeverityError {
			regressed = true
		}
	}
	if !regressed {
		t.Fatalf("expected PYLON042 regression finding, got %+v", findings)
	}

	// Re-baseline accepts the new count.
	if _, err := RunRatchet(kb, RatchetOptions{Update: true}); err != nil {
		t.Fatalf("re-baseline failed: %v", err)
	}
	findings, _ = RunRatchet(kb, RatchetOptions{})
	for _, f := range findings {
		if f.Severity == report.SeverityError {
			t.Fatalf("re-baselined corpus must pass: %+v", f)
		}
	}
}

func TestReworkRate(t *testing.T) {
	kb := t.TempDir()
	proj := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", proj}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	mustWrite(t, filepath.Join(proj, "a.go"), "a\n")
	run("add", "-A")
	run("commit", "-qm", "fix: predate a")
	mustWrite(t, filepath.Join(proj, "a.go"), "a2\n")
	run("add", "-A")
	run("commit", "-qm", "feat: add a")
	mustWrite(t, filepath.Join(proj, "b.go"), "b\n")
	run("add", "-A")
	run("commit", "-qm", "fix: unrelated b")
	mustWrite(t, filepath.Join(proj, "a.go"), "a3\n")
	run("add", "-A")
	run("commit", "-qm", "fix: repair a")
	mustWrite(t, filepath.Join(proj, "a.go"), "a4\n")
	run("add", "-A")
	run("commit", "-qm", "refactor: rework a")

	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n")

	// An unborn second project must be skipped with a warning, not fail the run.
	unborn := t.TempDir()
	out, err := exec.Command("git", "-C", unborn, "init", "-q").CombinedOutput()
	if err != nil {
		t.Fatalf("git init unborn: %v: %s", err, out)
	}
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  svc:\n    path: "+proj+"\n    ref: main\n  unborn:\n    path: "+unborn+"\n    ref: main\n")

	reports, err := ReworkRate(kb, "", "30d")
	if err != nil {
		t.Fatalf("ReworkRate: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1 (unborn project skipped)", len(reports))
	}
	r := reports[0]
	if r.Project != "svc" {
		t.Fatalf("project = %s, want svc", r.Project)
	}
	if r.Feat != 1 || r.Fix != 3 || r.Refactor != 1 {
		t.Fatalf("counts feat=%d fix=%d refactor=%d, want 1/3/1", r.Feat, r.Fix, r.Refactor)
	}
	// The predating fix (older than the feat that made a hot) must not
	// count; only the later "repair a" commit is rework.
	if r.ReworkFix != 1 {
		t.Fatalf("rework fix = %d, want 1 (fixes before their feat do not rework)", r.ReworkFix)
	}
	if r.ReworkRefactor != 1 {
		t.Fatalf("rework refactor = %d, want 1", r.ReworkRefactor)
	}
	if r.HotPaths != 1 {
		t.Fatalf("hot paths = %d, want 1", r.HotPaths)
	}

	if _, err := ReworkRate(kb, "", "bogus"); err == nil {
		t.Fatal("expected error for unparsable window")
	}
	if _, err := ReworkRate(kb, "ghost-pkg", "90d"); !errors.Is(err, ErrPackageNotFound) {
		t.Fatalf("unknown package must be ErrPackageNotFound, got %v", err)
	}
}

func TestAdvisoryRatchetIgnoresTimeDriven(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects: {}\n")
	stale := "---\ntitle: \"P\"\ntype: package\nstatus: active\npackage: p\nupdated: 2020-01-01\n---\n\n# P\n"
	mustWrite(t, filepath.Join(kb, "packages/p1.md"), stale)
	if _, err := RunRatchet(kb, RatchetOptions{Update: true}); err != nil {
		t.Fatalf("baseline update failed: %v", err)
	}
	// A second stale package doc ages the corpus by content, not by the
	// wall clock; the ratchet must stay green.
	mustWrite(t, filepath.Join(kb, "packages/p2.md"), stale)
	findings, err := RunRatchet(kb, RatchetOptions{})
	if err != nil {
		t.Fatalf("ratchet failed: %v", err)
	}
	for _, f := range findings {
		if f.RuleID == "PYLON042" {
			t.Fatalf("time-driven advisories must not regress the ratchet: %+v", f)
		}
	}
}

func TestAdvisoryRatchetBrokenConfig(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects: {a: {path: [unclosed")
	if _, err := RunRatchet(kb, RatchetOptions{}); err == nil {
		t.Fatal("broken manifests/pylons.yaml must fail the ratchet closed")
	}
	if _, err := RunRatchet(kb, RatchetOptions{Update: true}); err == nil {
		t.Fatal("broken manifests/pylons.yaml must refuse --update")
	}
}

func TestAdvisoryRatchetCorruptBaseline(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects: {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/ratchet.json"), "{not json")
	if _, err := RunRatchet(kb, RatchetOptions{}); err == nil {
		t.Fatal("corrupt baseline must error, not pass")
	}
}

func TestImportBoundary(t *testing.T) {
	kb := t.TempDir()
	svc, helper := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(svc, "go.mod"), "module example.com/svc\n\ngo 1.27\n")
	mustWrite(t, filepath.Join(helper, "go.mod"), "module example.com/helper\n\ngo 1.27\n")
	mustWrite(t, filepath.Join(helper, "util.go"), "package helper\n")
	mustWrite(t, filepath.Join(svc, "main.go"), "package main\n\nimport \"example.com/helper/util\"\n")
	mustWrite(t, filepath.Join(svc, "inner.go"), "package main\n\nimport \"example.com/svc/inner\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n  helper:\n    path: "+helper+"\n    ref: main\n")
	pkg := "---\ntitle: \"Svc\"\ntype: package\nstatus: active\npackage: svc\ncomponents: [svc]\nupdated: 2026-11-19\ndepends_on: []\n---\n\n# Svc\n"
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), pkg)

	importFired := func() int {
		n := 0
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON043" {
				n++
				if f.Severity != report.SeverityAdvisory {
					t.Fatalf("PYLON043 must be advisory, got %s", f.Severity)
				}
			}
		}
		return n
	}
	// The cross-module import is undeclared; the intra-project one is not.
	if n := importFired(); n != 1 {
		t.Fatalf("PYLON043 fired %d times, want 1", n)
	}

	mustWrite(t, filepath.Join(kb, "packages/svc.md"), strings.Replace(pkg, "depends_on: []", "depends_on: [helper]", 1))
	if n := importFired(); n != 0 {
		t.Fatalf("declared dependency must clear PYLON043, fired %d", n)
	}
}

func TestImportBoundaryTS(t *testing.T) {
	kb := t.TempDir()
	app, lib := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(lib, "package.json"), `{"name": "@platform/libb"}`)
	mustWrite(t, filepath.Join(lib, "index.ts"), "export const x = 1\n")
	mustWrite(t, filepath.Join(app, "index.ts"), "import { x } from '@platform/libb'\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [typescript]\nprojects:\n  app:\n    path: "+app+"\n    ref: main\n  libb:\n    path: "+lib+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), "---\ntitle: \"App\"\ntype: package\nstatus: active\npackage: app\ncomponents: [app]\nupdated: 2026-11-19\ndepends_on: []\n---\n\n# App\n")
	found := false
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON043" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected PYLON043 for the undeclared ts import")
	}
}

func TestRippleReport(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\n// SessionToken is the durable session key.\ntype SessionToken struct{}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")

	// A plan declares the contract; a package exposes the same id; a
	// quirk anchors the same file; a domain term appears in the code.
	plan := `---
title: "Tokens"
type: plan
status: accepted
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/svc.go::SessionToken
      wrong_if: "SessionToken changes shape"
---

# Tokens
`
	mustWrite(t, filepath.Join(kb, "plans/feature-tokens.md"), plan)
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), "---\ntitle: \"Svc\"\ntype: package\nstatus: active\npackage: svc\nupdated: 2026-11-19\n---\n\n# Svc\n")
	mustWrite(t, filepath.Join(kb, "quirks/q-token.md"), "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\nsources: [repo://svc/svc.go]\n---\n\n# Q\n\n## Symptom\n\nS\n\n## Cause\n\nC\n\n## Resolution\n\nR\n")
	mustWrite(t, filepath.Join(kb, "domain/entities/session-token.md"), "---\ntitle: \"SessionToken\"\ntype: domain-entity\nstatus: accepted\navoid:\n  - Token\n---\n\n# SessionToken\n")

	target := filepath.Join(svc, "svc.go")
	findings := Check(kb, []string{target})
	var ripple []report.Finding
	for _, f := range findings {
		if f.RuleID == "PYLON044" {
			ripple = append(ripple, f)
			if f.Severity != report.SeverityAdvisory {
				t.Fatalf("RIPPLE_OPEN must be advisory, got %s", f.Severity)
			}
		}
	}
	if len(ripple) == 0 {
		t.Fatalf("expected PYLON044 ripple findings, got %+v", findings)
	}
	joined := ""
	for _, f := range ripple {
		joined += f.Message + "\n"
	}
	for _, want := range []string{"plans/feature-tokens.md", "quirks/q-token.md", "domain/entities/session-token.md"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ripple must name %s; got %s", want, joined)
		}
	}
}

func TestSliceContext(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	plan := `---
title: "Tokens"
type: plan
status: in-progress
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/svc.go::Ping
      wrong_if: "Ping changes"
---

# Tokens

The SessionToken keys the durable session store.
`
	mustWrite(t, filepath.Join(kb, "plans/feature-tokens.md"), plan)
	mustWrite(t, filepath.Join(kb, "quirks/q-token.md"), "---\ntitle: \"Q\"\ntype: quirk\nstatus: active\nsources: [repo://svc/svc.go]\n---\n\n# Q\n\n## Symptom\n\nS\n\n## Cause\n\nC\n\n## Resolution\n\nR\n")
	mustWrite(t, filepath.Join(kb, "domain/entities/session-token.md"), "---\ntitle: \"SessionToken\"\ntype: domain-entity\nstatus: accepted\navoid:\n  - Token\n---\n\n# SessionToken\n")

	rep, err := SliceContext(kb, "plans/feature-tokens.md", false)
	if err != nil {
		t.Fatalf("SliceContext: %v", err)
	}
	if rep.Plan != "plans/feature-tokens.md" || len(rep.Components) != 1 || rep.Components[0] != "svc" {
		t.Fatalf("bad plan header: %+v", rep)
	}
	if len(rep.Contracts) != 1 || !rep.Contracts[0].Resolved {
		t.Fatalf("expected 1 resolved contract, got %+v", rep.Contracts)
	}
	if len(rep.Quirks) != 1 || !strings.Contains(rep.Quirks[0], "q-token.md") {
		t.Fatalf("expected the quirk link, got %v", rep.Quirks)
	}
	if len(rep.Terms) != 1 || !strings.Contains(rep.Terms[0], "session-token.md") {
		t.Fatalf("expected the domain term, got %v", rep.Terms)
	}

	if _, err := SliceContext(kb, "plans/missing.md", false); err == nil {
		t.Fatal("missing plan must error")
	}
}

func TestSliceIncomplete(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	slicePlan := `---
title: "Slice"
type: plan
status: accepted
slice: true
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/svc.go::Ping
      wrong_if: "Ping changes"
---

# Slice
`
	mustWrite(t, filepath.Join(kb, "plans/feature-slice.md"), slicePlan)
	fired := func() bool {
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON045" {
				if f.Severity != report.SeverityAdvisory {
					t.Fatalf("SLICE_INCOMPLETE must be advisory, got %s", f.Severity)
				}
				return true
			}
		}
		return false
	}
	if !fired() {
		t.Fatal("slice plan without a test contract must fire SLICE_INCOMPLETE")
	}

	mustWrite(t, filepath.Join(kb, "plans/feature-slice.md"), strings.Replace(slicePlan,
		"      wrong_if: \"Ping changes\"", "      wrong_if: \"Ping changes\"\n    - kind: glob\n      id: repo://svc/svc_test.go\n      wrong_if: \"tests drift\"", 1))
	if fired() {
		t.Fatal("slice plan with a behavior and a test contract must not fire")
	}

	// Ordinary plans stay untouched.
	mustWrite(t, filepath.Join(kb, "plans/feature-plain.md"), "---\ntitle: \"Plain\"\ntype: plan\nstatus: accepted\ncomponents: [svc]\nticket: [T-2]\n---\n\n# Plain\n")
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON045" && strings.Contains(f.File, "feature-plain.md") {
			t.Fatalf("SLICE_INCOMPLETE must not fire on ordinary plans: %+v", f)
		}
	}
}

func TestScenarioCoverage(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	slicePlan := `---
title: "Slice"
type: plan
status: accepted
slice: true
components: [svc]
ticket: [T-1]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/svc.go::Ping
      wrong_if: "Ping changes"
    - kind: glob
      id: repo://svc/svc_test.go
      wrong_if: "tests drift"
---

# Slice
`
	mustWrite(t, filepath.Join(kb, "plans/feature-slice.md"), slicePlan)
	count := func() int {
		n := 0
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON046" {
				n++
				if f.Severity != report.SeverityAdvisory {
					t.Fatalf("CONTRACT_NO_SCENARIO must be advisory, got %s", f.Severity)
				}
			}
		}
		return n
	}
	// One behavior contract without a covering scenario; the test glob
	// is not a behavior surface.
	if n := count(); n != 1 {
		t.Fatalf("PYLON046 fired %d times, want 1", n)
	}

	mustWrite(t, filepath.Join(kb, "plans/feature-slice.md"), strings.Replace(slicePlan, "---\n\n# Slice",
		"scenarios:\n  - given: \"a running service\"\n    when: \"Ping is called\"\n    expect: \"the call returns\"\n    covers: [repo://svc/svc.go::Ping]\n---\n\n# Slice", 1))
	if n := count(); n != 0 {
		t.Fatalf("covering scenario must clear PYLON046, fired %d", n)
	}

	// An ordinary plan with contracts is never gated on scenarios.
	mustWrite(t, filepath.Join(kb, "plans/feature-plain.md"), "---\ntitle: \"Plain\"\ntype: plan\nstatus: accepted\ncomponents: [svc]\nticket: [T-2]\n---\n\n# Plain\n")
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON046" && strings.Contains(f.File, "feature-plain.md") {
			t.Fatalf("PYLON046 must not fire on ordinary plans: %+v", f)
		}
	}
}

func TestImportBoundaryOwnership(t *testing.T) {
	// H1: sibling components of one package are internal, and
	// depends_on clears edges by owning package, not by alias.
	kb := t.TempDir()
	base, ui := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "go.mod"), "module example.com/wbase\n")
	mustWrite(t, filepath.Join(base, "b.go"), "package wbase\n")
	mustWrite(t, filepath.Join(ui, "go.mod"), "module example.com/wui\n")
	mustWrite(t, filepath.Join(ui, "u.go"), "package wui\n\nimport \"example.com/wbase\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  wbase:\n    path: "+base+"\n    ref: main\n  wui:\n    path: "+ui+"\n    ref: main\n")
	fired := func() int {
		n := 0
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON043" {
				n++
			}
		}
		return n
	}
	// One package legacy claims both components: internal, silent.
	mustWrite(t, filepath.Join(kb, "packages/legacy.md"), "---\ntitle: \"Legacy\"\ntype: package\nstatus: active\npackage: legacy\ncomponents: [wbase, wui]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Legacy\n")
	if n := fired(); n != 0 {
		t.Fatalf("sibling components must stay internal, fired %d", n)
	}
	// Separate packages: undeclared until depends_on names the owner.
	mustWrite(t, filepath.Join(kb, "packages/wbase.md"), "---\ntitle: \"WBase\"\ntype: package\nstatus: active\npackage: wbase\ncomponents: [wbase]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# WBase\n")
	mustWrite(t, filepath.Join(kb, "packages/wui.md"), "---\ntitle: \"Wui\"\ntype: package\nstatus: active\npackage: wui\ncomponents: [wui]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Wui\n")
	if n := fired(); n != 1 {
		t.Fatalf("cross-package edge must fire once per target, fired %d", n)
	}
	// depends_on: [wbase] clears it even though the alias is wbase's
	// own package name here; use an alias that differs from the name.
}

func TestImportBoundaryDepOnOwnerPackage(t *testing.T) {
	// The alias differs from the owning package name: depends_on
	// naming that owning package clears the edge.
	kb := t.TempDir()
	base, ui := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(base, "go.mod"), "module example.com/wbase\n")
	mustWrite(t, filepath.Join(base, "b.go"), "package wbase\n")
	mustWrite(t, filepath.Join(ui, "go.mod"), "module example.com/wui\n")
	mustWrite(t, filepath.Join(ui, "u.go"), "package wui\n\nimport \"example.com/wbase\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  wbase:\n    path: "+base+"\n    ref: main\n  wui:\n    path: "+ui+"\n    ref: main\n")
	pkg := "---\ntitle: \"Ui\"\ntype: package\nstatus: active\npackage: ui-core\ncomponents: [wui]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Ui\n"
	mustWrite(t, filepath.Join(kb, "packages/ui-core.md"), pkg)
	count := func() int {
		n := 0
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON043" {
				n++
			}
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("undeclared cross-package edge must fire, got %d", n)
	}
	mustWrite(t, filepath.Join(kb, "packages/ui-core.md"), strings.Replace(pkg, "depends_on: []", "depends_on: [wbase]", 1))
	if n := count(); n != 0 {
		t.Fatalf("depends_on naming the owning package must clear the edge, fired %d", n)
	}
}

func TestImportBoundaryGoNoise(t *testing.T) {
	// The Go extractor parses imports only: strings in comments, rune
	// literals, and error text never become specifiers.
	kb := t.TempDir()
	helper := t.TempDir()
	mustWrite(t, filepath.Join(helper, "go.mod"), "module example.com/helper\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+t.TempDir()+"\n    ref: main\n  helper:\n    path: "+helper+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/helper.md"), "---\ntitle: \"Helper\"\ntype: package\nstatus: active\npackage: helper\ncomponents: [helper]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Helper\n")
	if specs := extractGoImports([]byte(`package main

// import "example.com/helper" in a comment must not match.
const q = '"'
var s = ` + "`" + `import "example.com/helper"` + "`" + `

func f() { fmt.Errorf("import \"example.com/helper\" inside a string") }
`)); specs != nil {
		t.Fatalf("comment and string noise must not extract, got %v", specs)
	}
}

func TestImportBoundaryPython(t *testing.T) {
	kb := t.TempDir()
	pyA, pyB := t.TempDir(), t.TempDir()
	// Real package dirs with __init__.py become identities; generic
	// dirs do not.
	mustWrite(t, filepath.Join(pyA, "tokA/__init__.py"), "")
	mustWrite(t, filepath.Join(pyB, "tokB/__init__.py"), "")
	mustWrite(t, filepath.Join(pyB, "src"), "")
	osRemove := filepath.Join(pyB, "src")
	_ = osRemove
	mustWrite(t, filepath.Join(pyB, "mod.py"), "from tokA import x\nimport tokA\n\nimport os, sys\n\n# import tokA in a comment\n")
	mustWrite(t, filepath.Join(pyA, "mod.py"), "import tokB.y\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [python]\nprojects:\n  pya:\n    path: "+pyA+"\n    ref: main\n  pyb:\n    path: "+pyB+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/py.md"), "---\ntitle: \"Py\"\ntype: package\nstatus: active\npackage: py\ncomponents: [pya]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Py\n")
	n := 0
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON043" {
			n++
			if !strings.Contains(f.Message, "pyb") {
				t.Fatalf("expected edge to pyb, got %s", f.Message)
			}
		}
	}
	if n != 1 {
		t.Fatalf("PYLON043 fired %d times, want 1 (per-target dedupe)", n)
	}
	// Multi-import lines and as-clauses parse; stdlib names stay quiet.
	specs := extractPyImports([]byte("import a, b as c\nfrom pkg.mod import X\n"))
	if len(specs) != 3 || specs[0] != "a" || specs[1] != "b" || specs[2] != "pkg.mod" {
		t.Fatalf("bad python extraction: %v", specs)
	}
}

func TestImportBoundaryCollisions(t *testing.T) {
	// An identity name registered under two aliases is ambiguous and
	// never fires.
	kb := t.TempDir()
	a, b := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(a, "go.mod"), "module example.com/twin\n")
	mustWrite(t, filepath.Join(b, "go.mod"), "module example.com/twin\n")
	mustWrite(t, filepath.Join(a, "a.go"), "package twin\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  twina:\n    path: "+a+"\n    ref: main\n  twinb:\n    path: "+b+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/t.md"), "---\ntitle: \"T\"\ntype: package\nstatus: active\npackage: t\ncomponents: [twina]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# T\n")
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON043" {
			t.Fatalf("ambiguous identity must never fire: %s", f.Message)
		}
	}
}

func TestImportBoundaryNestedRoot(t *testing.T) {
	// A project root nested inside another project's tree belongs to
	// the inner alias: the outer walk must not read the inner files,
	// or the inner imports would fire against the outer package.
	kb := t.TempDir()
	outer, other := t.TempDir(), t.TempDir()
	inner := filepath.Join(outer, "nested")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(outer, "go.mod"), "module example.com/outer\n")
	mustWrite(t, filepath.Join(outer, "o.go"), "package outer\n")
	mustWrite(t, filepath.Join(inner, "go.mod"), "module example.com/inner\n")
	mustWrite(t, filepath.Join(inner, "i.go"), "package inner\n\nimport \"example.com/other\"\n")
	mustWrite(t, filepath.Join(other, "go.mod"), "module example.com/other\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  outer:\n    path: "+outer+"\n    ref: main\n  inner:\n    path: "+inner+"\n    ref: main\n  other:\n    path: "+other+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/o.md"), "---\ntitle: \"O\"\ntype: package\nstatus: active\npackage: o\ncomponents: [outer]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# O\n")
	mustWrite(t, filepath.Join(kb, "packages/i.md"), "---\ntitle: \"I\"\ntype: package\nstatus: active\npackage: i\ncomponents: [inner]\nupdated: 2026-11-20\ndepends_on: [other]\n---\n\n# I\n")
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON043" {
			t.Fatalf("inner files must not fire against the outer package: %s", f.Message)
		}
	}
}

func TestImportBoundaryChangedOnly(t *testing.T) {
	// --changed lints only staged package docs: an unstaged package
	// doc stays out of the import scan.
	kb := t.TempDir()
	svc, helper := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(svc, "go.mod"), "module example.com/svc\n")
	mustWrite(t, filepath.Join(helper, "go.mod"), "module example.com/helper\n")
	mustWrite(t, filepath.Join(svc, "s.go"), "package svc\n\nimport \"example.com/helper\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n  helper:\n    path: "+helper+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), "---\ntitle: \"Svc\"\ntype: package\nstatus: active\npackage: svc\ncomponents: [svc]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Svc\n")
	// Without git, stagedMarkdownFiles returns nil: the full scan
	// runs. Simulate by calling the boundary scan directly.
	cfg, err := config.Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	if got := lintImportBoundaries(kb, cfg, nil); len(got) != 1 {
		t.Fatalf("full scan fires once, got %d", len(got))
	}
	if got := lintImportBoundaries(kb, cfg, map[string]bool{"packages/other.md": true}); len(got) != 0 {
		t.Fatalf("unchanged package docs must stay silent, got %d", len(got))
	}
}

func TestRippleGlobsAndStatus(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(svc, "svc_test.go"), "package svc\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	// Glob-kind contract ripples through gitignore-style matching.
	mustWrite(t, filepath.Join(kb, "plans/feature-g.md"), "---\ntitle: \"G\"\ntype: plan\nstatus: accepted\ncomponents: [svc]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: glob\n      id: repo://svc/**/*_test.go\n      wrong_if: \"tests drift\"\n---\n\n# G\n")
	mkPkg := func(status string) string {
		return "---\ntitle: \"S\"\ntype: package\nstatus: " + status + "\npackage: svc\ncomponents: [svc]\nupdated: 2026-11-20\nexposes:\n  - kind: go-symbol\n    id: repo://svc/svc.go::Ping\n---\n\n# S\n"
	}
	findings := func() []report.Finding {
		var out []report.Finding
		for _, f := range Check(kb, []string{"repo://svc/svc_test.go", "repo://svc/svc.go"}) {
			if f.RuleID == "PYLON044" {
				out = append(out, f)
			}
		}
		return out
	}
	fs := findings()
	if len(fs) != 1 || !strings.Contains(fs[0].Message, "feature-g.md") {
		t.Fatalf("glob contract must ripple, got %+v", fs)
	}
	// Deprecated packages keep no live ripple surface.
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), mkPkg("deprecated"))
	if fs := findings(); len(fs) != 1 {
		t.Fatalf("deprecated exposes must not ripple, got %+v", fs)
	}
	// The finding anchors to a real KB doc, not alias/path.
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), mkPkg("active"))
	fs = findings()
	// One ripple per touched target: svc_test.go via the glob plan,
	// svc.go via the package exposes.
	if len(fs) != 2 {
		t.Fatalf("expected two ripple findings, got %d", len(fs))
	}
	for _, f := range fs {
		if f.File == "svc/svc_test.go" || f.File == "svc/svc.go" {
			t.Fatalf("finding must anchor to a KB doc, got %s", f.File)
		}
		if !strings.HasPrefix(f.File, "packages/") && !strings.HasPrefix(f.File, "plans/") {
			t.Fatalf("finding file must be a KB doc, got %s", f.File)
		}
	}
}

func TestSliceRulesGating(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	plan := func(status string) string {
		return "---\ntitle: \"S\"\ntype: plan\nstatus: " + status + "\ncomponents: [svc]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: go-symbol\n      id: repo://svc/svc.go::Ping\n      wrong_if: \"Ping changes\"\n---\n\n# S\n"
	}
	count := func(rel string) int {
		n := 0
		for _, f := range Lint(kb, LintOptions{HasCode: true}) {
			if f.RuleID == "PYLON045" && strings.Contains(f.File, rel) {
				n++
			}
		}
		return n
	}
	// proposed and abandoned slice plans never gate
	for _, status := range []string{"proposed", "abandoned", "shipped"} {
		mustWrite(t, filepath.Join(kb, "plans/feature-s.md"), strings.Replace(plan("accepted"), "status: accepted", "status: "+status, 1))
		// add slice: true
		mustWrite(t, filepath.Join(kb, "plans/feature-s.md"), strings.Replace(plan(status), "status: "+status, "status: "+status+"\nslice: true", 1))
		if n := count("plans/feature-s.md"); n != 0 {
			t.Fatalf("status %s must not fire PYLON045, fired %d", status, n)
		}
	}
	// accepted slice plans gate
	mustWrite(t, filepath.Join(kb, "plans/feature-s.md"), strings.Replace(plan("accepted"), "status: accepted", "status: accepted\nslice: true", 1))
	if n := count("plans/feature-s.md"); n != 1 {
		t.Fatalf("accepted slice plan must fire once, fired %d", n)
	}
	// A proposals/ slice plan is out of scope.
	mustWrite(t, filepath.Join(kb, "proposals/feature-x.md"), strings.Replace(plan("accepted"), "status: accepted", "status: accepted\nslice: true", 1))
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON045" && strings.Contains(f.File, "proposals/") {
			t.Fatalf("proposals/ plans must not gate: %+v", f)
		}
	}
}

func TestSliceTestSurfaceConventions(t *testing.T) {
	// Only real test conventions satisfy the test-surface half.
	for id, want := range map[string]bool{
		"repo://svc/svc_test.go":      true,
		"repo://svc/src/main_test.go": true,
		"repo://svc/tests/helper.ts":  true,
		"repo://svc/spec/api.spec.ts": true,
		"repo://svc/latest.go":        false,
		"repo://svc/contest/x.go":     false,
		"repo://e2e-tests/readme.md":  false,
		"repo://svc/attestation.go":   false,
	} {
		if got := testContractEntry(map[string]any{"kind": "glob", "id": id}); got != want {
			t.Errorf("testContractEntry(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestScenarioWellFormedness(t *testing.T) {
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	plan := "---\ntitle: \"S\"\ntype: plan\nstatus: accepted\nslice: true\ncomponents: [svc]\nticket: [T-1]\nscenarios:\n  - covers: [repo://svc/svc.go::Ping]\n  - given: \"a\"\n    when: \"b\"\n    expect: \"c\"\ncontract:\n  stable:\n    - kind: go-symbol\n      id: repo://svc/svc.go::Ping\n      wrong_if: \"Ping changes\"\n---\n\n# S\n"
	mustWrite(t, filepath.Join(kb, "plans/feature-s.md"), plan)
	n := 0
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON046" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("a covers-only scenario must not count as coverage, fired %d", n)
	}
}

func TestPlanFrontMatterParseFailureLoud(t *testing.T) {
	// M11: slice typed wrong (string) breaks plan parse; the lint
	// must say so instead of silently skipping every plan rule.
	kb := t.TempDir()
	svc := t.TempDir()
	mustWrite(t, filepath.Join(svc, "svc.go"), "package svc\n\nfunc Ping() {}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-bad.md"), "---\ntitle: \"B\"\ntype: plan\nstatus: accepted\nslice: \"true\"\ncomponents: [svc]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: go-symbol\n      id: repo://svc/svc.go::Ping\n      wrong_if: \"Ping changes\"\n---\n\n# B\n")
	found := false
	for _, f := range Lint(kb, LintOptions{HasCode: true}) {
		if f.RuleID == "PYLON001" && strings.Contains(f.File, "feature-bad.md") && strings.Contains(f.Message, "plan frontmatter cannot parse") {
			found = true
			if f.Severity != report.SeverityError {
				t.Fatalf("plan parse failure must be FORCED, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Fatal("unparsable plan frontmatter must fire PYLON001")
	}
}

func TestSliceContextJSONEmpty(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-empty.md"), "---\ntitle: \"E\"\ntype: plan\nstatus: proposed\ncomponents: []\nticket: []\n---\n\n# E\n")
	rep, err := SliceContext(kb, "plans/feature-empty.md", false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Fatalf("empty slices must marshal as [], got %s", data)
	}
	if _, err := SliceContext(kb, "meta/ROADMAP.md", false); err == nil {
		t.Fatal("a non-plan doc must error in SliceContext")
	}
}

func TestMergeDanger(t *testing.T) {
	// The gate needs git history for the derived door: build a real
	// repo with a schema contract change.
	// The gate reads the diff through the KB root, so the KB lives
	// inside the code repo here, mirroring the CI shape.
	repo := t.TempDir()
	kb := repo
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE t (id int);\n")
	shell("git", "-C", repo, "init", "-q")
	shell("git", "-C", repo, "add", "-f", "schema.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n    ref: main\n")

	// A schema contract in an accepted plan makes the diff one-way.
	mustWrite(t, filepath.Join(kb, "plans/feature-schema.md"), "---\ntitle: \"S\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: schema\n      id: repo://app/schema.sql\n      wrong_if: \"schema drift\"\n---\n\n# S\n")

	twoWayBody := filepath.Join(t.TempDir(), "body.md")
	oneWayBody := filepath.Join(t.TempDir(), "body2.md")
	mustWrite(t, twoWayBody, "## How\n\ndo it\n\nMerge danger: two-way | blast radius: app schema\n")
	mustWrite(t, oneWayBody, "## How\n\ndo it\n\nMerge danger: one-way | blast radius: app schema\n\nData migration: none needed\n")
	// Advance HEAD past the base so the diff is non-empty.
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE t (id int, extra text);\n")
	shell("git", "-C", repo, "add", "-f", "schema.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat!: extend schema")

	blocking := func(body string) bool {
		fs, err := MergeDanger(kb, MergeDangerOptions{Body: body, Base: "origin/main"})
		if err != nil {
			t.Fatalf("MergeDanger: %v", err)
		}
		for _, f := range fs {
			if f.Severity == report.SeverityError && f.RuleID == "PYLON047" {
				return true
			}
		}
		return false
	}
	if !blocking(twoWayBody) {
		t.Fatal("two-way under a schema diff must be a downgrade and block")
	}
	if blocking(oneWayBody) {
		fs, _ := MergeDanger(kb, MergeDangerOptions{Body: oneWayBody, Base: "origin/main"})
		t.Fatalf("one-way with migration note must pass, got %+v", fs)
	}

	// Line-level validation.
	missing := filepath.Join(t.TempDir(), "body3.md")
	mustWrite(t, missing, "## How\n\ndo it\n")
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: missing, Base: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) == 0 || !strings.Contains(fs[0].Message, "MERGE_DANGER_MISSING") {
		t.Fatalf("missing line must block, got %+v", fs)
	}
	dup := filepath.Join(t.TempDir(), "body4.md")
	mustWrite(t, dup, "Merge danger: two-way | blast radius: a\nMerge danger: two-way | blast radius: b\n")
	fs, _ = MergeDanger(kb, MergeDangerOptions{Body: dup, Base: "origin/main"})
	if len(fs) == 0 || !strings.Contains(fs[0].Message, "MERGE_DANGER_DUPLICATE") {
		t.Fatalf("duplicate lines must block, got %+v", fs)
	}
	bad := filepath.Join(t.TempDir(), "body5.md")
	mustWrite(t, bad, "Merge danger: sideways | blast radius: a\n")
	fs, _ = MergeDanger(kb, MergeDangerOptions{Body: bad, Base: "origin/main"})
	if len(fs) == 0 || !strings.Contains(fs[0].Message, "MERGE_DANGER_INVALID") {
		t.Fatalf("unknown door must block, got %+v", fs)
	}
	if _, err := MergeDanger(kb, MergeDangerOptions{Body: "", Base: "origin/main"}); err == nil {
		t.Fatal("no body must error")
	}
}

func shell(name string, args ...string) {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(string(out) + err.Error())
	}
}

func TestMergeDangerSeparateRepo(t *testing.T) {
	// The KB lives outside the code repo: the derived door must diff
	// the project's own repository.
	kb := t.TempDir()
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE t (id int);\n")
	shell("git", "-C", repo, "init", "-q")
	shell("git", "-C", repo, "add", "-f", "schema.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-schema.md"), "---\ntitle: \"S\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: schema\n      id: repo://app/schema.sql\n      wrong_if: \"schema drift\"\n---\n\n# S\n")

	oneWayBody := filepath.Join(t.TempDir(), "body.md")
	mustWrite(t, oneWayBody, "Merge danger: one-way | blast radius: app schema\n\nData migration: none needed\n")
	// Advance the project repo's diff.
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE t (id int, extra text);\n")
	shell("git", "-C", repo, "add", "-f", "schema.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat!: extend schema")

	fs, err := MergeDanger(kb, MergeDangerOptions{Body: oneWayBody, Base: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.Message != "MERGE_DANGER_DERIVED: the diff touches a schema contract surface; the one-way door is consistent with the derived minimum" {
			t.Fatalf("unexpected finding: %+v", f)
		}
	}
	if len(fs) != 1 {
		t.Fatalf("want exactly the derived advisory, got %+v", fs)
	}
	// A missing line under a schema diff must not also claim a
	// derived verdict.
	missing := filepath.Join(t.TempDir(), "body2.md")
	mustWrite(t, missing, "## How\n")
	fs, _ = MergeDanger(kb, MergeDangerOptions{Body: missing, Base: "origin/main"})
	for _, f := range fs {
		if f.Message != "MERGE_DANGER_MISSING: the body has no 'Merge danger: <door> | blast radius: <scope>' line" {
			t.Fatalf("empty door must produce no derived verdict, got %+v", f)
		}
	}
	if len(fs) != 1 {
		t.Fatalf("want exactly the missing-line error, got %+v", fs)
	}
}

func TestReleaseMarkerAndMigrationRegexes(t *testing.T) {
	markers := map[string]bool{
		"feat!: drop legacy writes": true,
		"feat(core)!: rewrite":      true,
		"fix!: stop the bleed":      true,
		"refactor!: same":           true,
		"release: cut v2 #major":    true,
		"feat: add a field":         false,
		"feat(core): add":           false,
		"docs: update":              false,
		"Merge pull request #9":     false,
	}
	for subject, want := range markers {
		got := releaseMarkerRe.MatchString(subject) || majorRe.MatchString(subject)
		if got != want {
			t.Errorf("subject %q: marker=%v want %v", subject, got, want)
		}
	}
	if !majorRe.MatchString("cut v2 (#major)") {
		t.Error("#major inside parens must match")
	}
	if majorRe.MatchString("cut v2 #majority") {
		t.Error("#majority must not match the #major token")
	}
	if !dataMigrationRe.MatchString("intro text\nData migration: backfills users\n") {
		t.Error("a line-start Data migration: note must match")
	}
	if dataMigrationRe.MatchString("quoted \"Data migration:\" in prose") {
		t.Error("a quoted mention must not satisfy the migration note")
	}
}

func TestMergeDangerDirectoryPattern(t *testing.T) {
	kb := t.TempDir()
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "db/migrations/001.sql"), "CREATE TABLE t (id int);\n")
	shell("git", "-C", repo, "init", "-q")
	shell("git", "-C", repo, "add", "-f", "db/migrations/001.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-mig.md"), "---\ntitle: \"M\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-2]\ncontract:\n  stable:\n    - kind: schema\n      id: repo://app/db/migrations/*.sql\n      wrong_if: \"migration drift\"\n---\n\n# M\n")
	twoWay := filepath.Join(t.TempDir(), "body.md")
	mustWrite(t, twoWay, "Merge danger: two-way | blast radius: migrations\n")
	mustWrite(t, filepath.Join(repo, "db/migrations/002.sql"), "CREATE TABLE u (id int);\n")
	shell("git", "-C", repo, "add", "-f", "db/migrations/002.sql")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: add migration")
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: twoWay, Base: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if f.Message == "MERGE_DANGER_DOWNGRADE: the diff touches a schema contract surface; the minimum door is one-way and the author two-way line cannot de-escalate it" {
			found = true
		}
	}
	if !found {
		t.Fatalf("directory contract surface must force the downgrade, got %+v", fs)
	}
}
