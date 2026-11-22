// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// gateKB builds a committed git repo holding a KB so gate-surface has a
// base ref to diff against. It returns the kb root and the base sha.
func gateKB(t *testing.T) (string, string) {
	t.Helper()
	kb := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", kb}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nexclude:\n  - old/**\n")
	mustWrite(t, filepath.Join(kb, "manifests/rules.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "domain/machine.md"), "# machine\n")
	run("add", "-Af", ".")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")
	return kb, base[:len(base)-1]
}

func findingRules(fs []report.Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.RuleID+" "+f.File)
	}
	return out
}

func TestGateSurfaceFlagsManifests(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, "manifests/ratchet.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "docs/other.md"), "# note\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "touch ratchet")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	fs := GateSurface(kb, base)
	found := findingRules(fs)
	if len(found) != 1 || found[0] != "PYLON062 manifests/ratchet.json" {
		t.Fatalf("expected exactly the ratchet finding, got %v", found)
	}
	for _, f := range fs {
		if f.Severity != report.SeverityError || f.Grade != report.GradeForced {
			t.Fatalf("PYLON062 must be error/FORCED: %+v", f)
		}
	}
}

func TestGateSurfaceFlagsHooksAndKnobs(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, ".githooks/pre-commit"), "#!/usr/bin/env bash\nexit 0\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nexclude:\n  - old/**\n  - everything/**\nschemes: [legacy]\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "widen")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	fs := GateSurface(kb, base)
	found := findingRules(fs)
	if len(found) != 3 {
		t.Fatalf("expected hook, knob key, and added list item findings, got %v", found)
	}
	joined := strings.Join(found, "\n")
	for _, want := range []string{"PYLON062 .githooks/pre-commit", "PYLON062 manifests/pylons.yaml"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s: %v", want, found)
		}
	}
	items := 0
	for _, f := range fs {
		if f.RuleID == "PYLON062" && f.File == "manifests/pylons.yaml" {
			items++
			if f.Line <= 0 {
				t.Fatalf("knob finding must carry a real line number, got %+v", f)
			}
		}
	}
	if items != 2 {
		t.Fatalf("expected both the knob key and its added list item to fire, got %d knob findings: %v", items, found)
	}
}

func TestGateSurfaceFlagsRenamedSurface(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, "manifests/ratchet.json"), "{}\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "add ratchet")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(kb, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(kb, "manifests/ratchet.json"), filepath.Join(kb, "docs/moved.json")); err != nil {
		t.Fatal(err)
	}
	run3 := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run3.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run4 := exec.Command("git", "-C", kb, "commit", "-qm", "move it")
	if out, err := run4.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	fs := GateSurface(kb, base)
	found := findingRules(fs)
	if len(found) != 1 || !strings.Contains(found[0], "manifests/ratchet.json") {
		t.Fatalf("a rename away from a gate surface must still flag the old path, got %v", found)
	}
}

func TestKnobPatchParsing(t *testing.T) {
	patch := "diff --git a/m b/m\n" +
		"@@ -1,2 +1,4 @@\n" +
		" exclude:\n" +
		"+  - a/**\n" +
		"+  - b/**\n" +
		"\\ No newline at end of file\n" +
		"+schemes: [x]\n"
	hits := parseKnobPatch(patch)
	if len(hits) != 3 {
		t.Fatalf("expected 3 knob hits (two list items, one key), got %v", hits)
	}
	if hits[0].line != 2 || hits[1].line != 3 || hits[2].line != 4 {
		t.Fatalf("line math wrong: %v", hits)
	}
}

func TestGateSurfaceCleanSilent(t *testing.T) {
	kb, base := gateKB(t)
	mustWrite(t, filepath.Join(kb, "docs/only.md"), "# note\n")
	run := exec.Command("git", "-C", kb, "add", "-Af", ".")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("add: %v: %s", err, out)
	}
	run2 := exec.Command("git", "-C", kb, "commit", "-qm", "docs only")
	if out, err := run2.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v: %s", err, out)
	}
	if fs := GateSurface(kb, base); len(fs) != 0 {
		t.Fatalf("doc-only diff must stay silent, got %v", findingRules(fs))
	}
}

func TestGateSurfaceUnresolvableBaseAdvises(t *testing.T) {
	kb, _ := gateKB(t)
	fs := GateSurface(kb, "definitely-not-a-ref")
	if len(fs) != 1 {
		t.Fatalf("expected one advisory, got %v", findingRules(fs))
	}
	if fs[0].RuleID != "PYLON063" || fs[0].Severity != report.SeverityAdvisory {
		t.Fatalf("expected advisory PYLON063, got %+v", fs[0])
	}
}

func TestSecretAndGateNeverBaselined(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	fs := []report.Finding{
		errFinding("PYLON060", "domain/x.md", 3, "possible credential: rotate it; deleting the line does not remove it from git history"),
		errFinding("PYLON062", "manifests/ratchet.json", 1, "gate surface changed"),
		errFinding("PYLON018", "investigations/a.md", 9, "finding row 1 is invalid"),
	}
	if err := UpdateBaseline(kb, fs); err != nil {
		t.Fatal(err)
	}
	kept, accepted, _ := ApplyBaseline(kb, fs)
	if accepted != 1 {
		t.Fatalf("only the ordinary finding may be accepted, got %d", accepted)
	}
	if len(kept) != 2 {
		t.Fatalf("secret and gate findings must survive the baseline, kept %d", len(kept))
	}
}

func TestPackagesManifestEmptyIsArray(t *testing.T) {
	// The generated manifest must stay a JSON array even with zero
	// packages: null breaks every consumer that indexes it.
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	out := BuildPackagesManifest(kb)
	if out == nil {
		t.Fatal("BuildPackagesManifest must return an empty slice, not nil")
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty packages manifest must marshal as [], got %s", data)
	}
}

func TestMergeDangerNotEvaluatedWithoutGit(t *testing.T) {
	// A KB whose project diffs cannot be read degrades to the
	// not-evaluated advisory; it never reads as a clean verdict.
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n")
	plan := "---\ntitle: ship\ntype: plan\nstatus: accepted\ncreated: 2026-11-01\nupdated: 2026-11-01\nowners: [Pylons]\ncomponents: [svc]\nticket: [EX-1]\ncontract:\n  stable:\n    - kind: go-schema\n      id: repo://svc/schema.sql::tables\n---\n\n## Problem\n\nShip it.\n"
	mustWrite(t, filepath.Join(kb, "plans/feature-ship.md"), plan)
	if err := os.MkdirAll(filepath.Join(kb, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(kb, "svc/schema.sql"), "CREATE TABLE ship;\n")
	body := "---\ntitle: body\n---\n\nMerge danger: two-way | blast radius: docs\n"
	mustWrite(t, filepath.Join(kb, "body.md"), body)
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: filepath.Join(kb, "body.md"), Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if strings.Contains(f.Message, "MERGE_DANGER_NOT_EVALUATED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a not-evaluated advisory, got %v", fs)
	}
}

func TestGateSurfaceFlagsSubdirectoryLedger(t *testing.T) {
	// The ledger lives at mono/ledger inside a workspace repo; the
	// guard must still see repo-root-relative gate-surface paths.
	mono := t.TempDir()
	kb := filepath.Join(mono, "ledger")
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", mono}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	mustWrite(t, filepath.Join(mono, "README.md"), "# workspace\n")
	run("add", "-Af", ".")
	run("commit", "-qm", "base")
	base := run("rev-parse", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nexclude:\n  - old/**\n")
	mustWrite(t, filepath.Join(kb, "manifests/baseline.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "manifests/rules.json"), "{}\n")
	mustWrite(t, filepath.Join(kb, "domain/machine.md"), "# machine\n")
	run("add", "-Af", ".")
	run("commit", "-qm", "seed ledger")
	fs := GateSurface(kb, base[:len(base)-1])
	found := findingRules(fs)
	// The seed touches three gate surfaces and widens exclude, which
	// adds a knob finding beside the pylons.yaml surface finding.
	if len(found) != 4 {
		t.Fatalf("a subdirectory ledger must still flag its gate surfaces, got %v", found)
	}
	if !slices.Contains(found, "PYLON062 manifests/baseline.json") || !slices.Contains(found, "PYLON062 manifests/rules.json") {
		t.Fatalf("the touched manifests must be flagged, got %v", found)
	}
	knob := false
	for _, f := range fs {
		if f.RuleID == "PYLON062" && strings.Contains(f.Message, "exclude") {
			knob = true
		}
	}
	if !knob {
		t.Fatalf("the added exclude knob must be flagged, got %v", found)
	}
}

func TestMergeDangerMarkerReadsTheCodeRepo(t *testing.T) {
	// The ledger is not a git repo; the code repo is. The marker
	// proof must read the code repo's history, so an unmarked diff
	// fires UNPAIRED instead of passing silently.
	repo := t.TempDir()
	kb := t.TempDir()
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	shell("git", "-C", repo, "config", "user.email", "t@example.com")
	shell("git", "-C", repo, "config", "user.name", "t")
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE ship;\n")
	shell("git", "-C", repo, "add", "-f", ".")
	shell("git", "-C", repo, "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	shell("git", "-C", repo, "commit", "--allow-empty", "-qm", "refactor: unmarked change")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+repo+"\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-ship.md"), "---\ntitle: ship\ntype: plan\nstatus: accepted\ncreated: 2026-11-01\nupdated: 2026-11-01\nowners: [Pylons]\ncomponents: [svc]\nticket: [EX-1]\ncontract:\n  stable:\n    - kind: go-schema\n      id: repo://svc/schema.sql::tables\n---\n\n## Problem\n\nShip it.\n")
	body := "---\ntitle: body\n---\n\nMerge danger: one-way | blast radius: docs\n"
	mustWrite(t, filepath.Join(kb, "body.md"), body)
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: filepath.Join(kb, "body.md"), Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if strings.Contains(f.Message, "MERGE_DANGER_UNPAIRED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an unmarked code diff must fire UNPAIRED even when the ledger is not a repo, got %v", fs)
	}
	// A breaking subject in the code repo proves the one-way claim.
	shell("git", "-C", repo, "commit", "--allow-empty", "-qm", "feat!: breaking change")
	fs, err = MergeDanger(kb, MergeDangerOptions{Body: filepath.Join(kb, "body.md"), Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if strings.Contains(f.Message, "MERGE_DANGER_UNPAIRED") {
			t.Fatalf("a code-repo release marker proves the one-way door, got %v", fs)
		}
	}
}

func TestMergeDangerUnreadableBaseNeverPassesSilently(t *testing.T) {
	repo := t.TempDir()
	kb := t.TempDir()
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	shell("git", "-C", repo, "config", "user.email", "t@example.com")
	shell("git", "-C", repo, "config", "user.name", "t")
	mustWrite(t, filepath.Join(repo, "schema.sql"), "CREATE TABLE ship;\n")
	shell("git", "-C", repo, "add", "-f", ".")
	shell("git", "-C", repo, "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+repo+"\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-ship.md"), "---\ntitle: ship\ntype: plan\nstatus: accepted\ncreated: 2026-11-01\nupdated: 2026-11-01\nowners: [Pylons]\ncomponents: [svc]\nticket: [EX-1]\ncontract:\n  stable:\n    - kind: go-schema\n      id: repo://svc/schema.sql::tables\n---\n\n## Problem\n\nShip it.\n")
	body := "---\ntitle: body\n---\n\nMerge danger: one-way | blast radius: docs\n"
	mustWrite(t, filepath.Join(kb, "body.md"), body)
	fs, err := MergeDanger(kb, MergeDangerOptions{Body: filepath.Join(kb, "body.md"), Base: "nosuchref"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fs {
		if strings.Contains(f.Message, "MERGE_DANGER_NOT_EVALUATED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an unreadable base must advise, never pass silently, got %v", fs)
	}
}
