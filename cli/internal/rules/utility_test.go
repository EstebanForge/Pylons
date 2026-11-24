package rules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// utilityFixture builds a temp ledger with one quirk and one convention
// doc, a real git repository (telemetry target), and a citation marker
// file when cited is true.
func utilityFixture(t *testing.T, cited bool) string {
	t.Helper()
	root := t.TempDir()
	if err := exec.Command("git", "init", "-q", root).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "quirks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "conventions/proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	quirk := "---\ntitle: \"Trap\"\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\n---\n\n# Trap\n"
	if err := os.WriteFile(filepath.Join(root, "quirks", "trap.md"), []byte(quirk), 0o644); err != nil {
		t.Fatal(err)
	}
	if cited {
		if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
			t.Fatal(err)
		}
		note := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
		if err := os.WriteFile(filepath.Join(root, "plans", "note.md"), []byte(note), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestUtilityPruneCriteria(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2026-11-20\",\"path\":\"quirks/trap.md\"}\n", 2) +
		strings.Repeat("{\"date\":\"2026-11-21\",\"path\":\"quirks/trap.md\"}\n", 2)
	gitDir := filepath.Join(root, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PruneCandidates) != 1 || rep.PruneCandidates[0] != "quirks/trap.md" {
		t.Fatalf("prune candidate missing: %+v", rep)
	}
	// Report-only default: the file must not move.
	if _, err := os.Stat(filepath.Join(root, "quirks", "trap.md")); err != nil {
		t.Fatal("report-only run moved the quirk")
	}
}

func TestUtilityCitedQuirkNeverPrunes(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, true)
	state := strings.Repeat("{\"date\":\"2026-11-20\",\"path\":\"quirks/trap.md\"}\n", 4)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PruneCandidates) != 0 {
		t.Fatalf("cited quirk offered for prune: %+v", rep.PruneCandidates)
	}
	if len(rep.Scored) != 1 || rep.Scored[0].Citations != 1 {
		t.Fatalf("citation not counted: %+v", rep.Scored)
	}
}

func TestUtilityFreshInjectionsAreNotPruneEvidence(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2026-11-23\",\"path\":\"quirks/trap.md\"}\n", 2)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PruneCandidates) != 0 {
		t.Fatalf("thin telemetry offered for prune: %+v", rep.PruneCandidates)
	}
}

func TestUtilitySameDayVolumeNeverPrunes(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2026-11-23\",\"path\":\"quirks/trap.md\"}\n", 5)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PruneCandidates) != 0 {
		t.Fatalf("one heavy day counted as repeated surfacing: %+v", rep.PruneCandidates)
	}
}

func TestUtilityGhostDocumentNeverScores(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	markers := strings.Repeat("<!-- pylon:referenced-doc: quirks/ghost.md -->\n", 3)
	for _, f := range []string{"plans/a.md", "plans/b.md"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte(markers), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Promote: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 0 || len(rep.PromoteCandidates) != 0 || len(rep.Promoted) != 0 {
		t.Fatalf("ghost document scored: %+v", rep)
	}
	if entries, statErr := os.ReadDir(filepath.Join(root, "conventions", "proposals")); statErr == nil && len(entries) != 0 {
		t.Fatalf("ghost promote wrote a stub: %v", entries)
	}
}

func TestUtilityStubSlugSanitizedAndDistinct(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	other := "---\ntitle: \"Trap two\"\ntype: convention\nstatus: proposed\ncreated: 2026-11-01\nupdated: 2026-11-01\n---\n\n# Trap two\n"
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plans", "trap.md"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"quirks/trap.md", "plans/trap.md"} {
		body := strings.Repeat(fmt.Sprintf("<!-- pylon:referenced-doc: %s -->\n", target), 3)
		for _, f := range []string{"plans/a.md", "plans/b.md"} {
			if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Promote: true}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "conventions", "proposals"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	want := []string{
		fmt.Sprintf("conv-utility-%s.md", utilityStubSlug("plans/trap")),
		fmt.Sprintf("conv-utility-%s.md", utilityStubSlug("quirks/trap")),
	}
	slices.Sort(want)
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("stub names = %v, want %v", names, want)
	}
}

func TestUtilityStubSlugInjective(t *testing.T) {
	Paths := []string{"quirks/a-b.md", "quirks/a_b.md", "plans/a-b.md"}
	seen := map[string]string{}
	for _, p := range Paths {
		slug := utilityStubSlug(p)
		if prev, dup := seen[slug]; dup {
			t.Fatalf("slug collision: %q and %q both map to %q", prev, p, slug)
		}
		seen[slug] = p
		for _, r := range slug {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				t.Fatalf("hostile byte %q in slug %q", r, slug)
			}
		}
	}
}

func TestUtilityFutureDatedStateFailsClosed(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2099-01-01\",\"path\":\"quirks/trap.md\"}\n", 2) +
		strings.Repeat("{\"date\":\"2099-01-02\",\"path\":\"quirks/trap.md\"}\n", 2)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Prune: true}); err == nil {
		t.Fatal("future-dated telemetry accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "quirks", "trap.md")); err != nil {
		t.Fatal("future-dated telemetry moved the quirk")
	}
}

func TestUtilityUnterminatedFenceKeepsCitations(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	marker := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
	if err := os.MkdirAll(filepath.Join(root, "investigations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "investigations", "broken.md"), []byte("# Note\n\n```\n"+marker), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 1 || rep.Scored[0].Citations != 1 {
		t.Fatalf("unterminated fence lost evidence: %+v", rep.Scored)
	}
	if len(rep.PruneCandidates) != 0 {
		t.Fatalf("cited quirk offered for prune off a broken fence: %+v", rep.PruneCandidates)
	}
}

func TestUtilityTildeFenceStripped(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	marker := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
	if err := os.MkdirAll(filepath.Join(root, "investigations"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A tilde line inside a backtick block must not close the block.
	doc := "# Note\n\n```\n" + marker + "~~~\n" + marker + "```\n"
	if err := os.WriteFile(filepath.Join(root, "investigations", "note.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 0 {
		t.Fatalf("fenced markers counted: %+v", rep.Scored)
	}
	// A tilde block on its own strips too.
	doc = "# Note\n\n~~~\n" + marker + "~~~\n"
	if err := os.WriteFile(filepath.Join(root, "investigations", "note.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err = ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 0 {
		t.Fatalf("tilde-fenced marker counted: %+v", rep.Scored)
	}
}

func TestUtilityStateTraversalFailsClosed(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2026-11-20\",\"path\":\"quirks/../../escape.md\"}\n", 4)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Dir(root)
	before, _ := os.ReadDir(outside)
	if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Prune: true}); err == nil {
		t.Fatal("traversal state accepted")
	}
	after, _ := os.ReadDir(outside)
	if len(after) != len(before) {
		t.Fatal("traversal state moved a file outside the ledger")
	}
}

func TestUtilitySkipsRootDocsAndFencedMarkers(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	marker := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Guidelines\n\n"+marker), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := "# Doc\n\n```\n" + marker + "```\n\nReal use:\n\n" + marker
	if err := os.MkdirAll(filepath.Join(root, "investigations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "investigations", "note.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 1 || rep.Scored[0].Citations != 1 {
		t.Fatalf("root docs and fences counted as evidence: %+v", rep.Scored)
	}
}

func TestRecordInjectionsCorruptStateNoAppend(t *testing.T) {
	root := utilityFixture(t, false)
	statePath := filepath.Join(root, ".git", "pylon-utility.json")
	if err := os.WriteFile(statePath, []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RecordInjections(root, []string{"quirks/trap.md"})
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{not json}\n" {
		t.Fatalf("corrupt state was modified: %q", got)
	}
}

func TestUtilityPruneAppliesAndSkipsExisting(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	state := strings.Repeat("{\"date\":\"2026-11-20\",\"path\":\"quirks/trap.md\"}\n", 2) +
		strings.Repeat("{\"date\":\"2026-11-21\",\"path\":\"quirks/trap.md\"}\n", 2)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "quirks", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Collision: an existing archive entry is never overwritten, and
	// the source stays put.
	if err := os.WriteFile(filepath.Join(root, "quirks", "archive", "trap.md"), []byte("taken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 0 {
		t.Fatalf("overwrite attempted: %+v", rep.Pruned)
	}
	if _, err := os.Stat(filepath.Join(root, "quirks", "trap.md")); err != nil {
		t.Fatal("source quirk moved despite the archive collision")
	}
	// No collision: the move lands and the doc leaves the candidates.
	if err := os.Remove(filepath.Join(root, "quirks", "archive", "trap.md")); err != nil {
		t.Fatal(err)
	}
	rep, err = ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 1 || rep.Pruned[0] != "quirks/trap.md" {
		t.Fatalf("prune did not land: %+v", rep.Pruned)
	}
	if _, err := os.Stat(filepath.Join(root, "quirks", "trap.md")); !os.IsNotExist(err) {
		t.Fatal("source quirk still present after prune")
	}
	if _, err := os.Stat(filepath.Join(root, "quirks", "archive", "trap.md")); err != nil {
		t.Fatal("archived quirk missing")
	}
}

func TestUtilityPromoteCriteriaAndStub(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	// Three citations across two distinct files, plus heavy injection
	// telemetry: promotable.
	markers := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plans", "a.md"), []byte(markers+markers), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plans", "b.md"), []byte(markers), 0o644); err != nil {
		t.Fatal(err)
	}
	state := strings.Repeat("{\"date\":\"2026-11-20\",\"path\":\"quirks/trap.md\"}\n", 4)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PromoteCandidates) != 1 || rep.PromoteCandidates[0].Path != "quirks/trap.md" {
		t.Fatalf("promote candidate missing: %+v", rep.PromoteCandidates)
	}
	if _, err := os.Stat(filepath.Join(root, "conventions", "proposals")); err != nil {
		t.Fatal("report-only run wrote a stub")
	}
	// Same citations from one file only: not promotable.
	if err := os.Remove(filepath.Join(root, "plans", "b.md")); err != nil {
		t.Fatal(err)
	}
	rep, err = ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PromoteCandidates) != 0 {
		t.Fatalf("single-file citations promoted: %+v", rep.PromoteCandidates)
	}
	// Applying: the stub lands once and a second run does not duplicate.
	if err := os.WriteFile(filepath.Join(root, "plans", "b.md"), []byte(markers), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Promote: true}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadDir(filepath.Join(root, "conventions", "proposals"))
	if err != nil || len(first) != 1 {
		t.Fatalf("promote stub missing: %v %v", first, err)
	}
	if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now, Promote: true}); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadDir(filepath.Join(root, "conventions", "proposals"))
	if err != nil || len(again) != 1 {
		t.Fatalf("promote duplicated the stub: %d entries", len(again))
	}
}

func TestUtilityNoTelemetryIsSafe(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Telemetry {
		t.Fatal("telemetry reported without a state file")
	}
	if len(rep.PruneCandidates) != 0 || len(rep.PromoteCandidates) != 0 {
		t.Fatal("candidates without telemetry")
	}
}

func TestUtilityScanExcludesProductTrees(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	marker := "<!-- pylon:referenced-doc: quirks/trap.md -->\n"
	for _, dir := range []string{"templates", "protocols", "pylons", "_scratch"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "doc.md"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plans", "real.md"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Scored) != 1 || rep.Scored[0].Citations != 1 {
		t.Fatalf("product-tree markers counted as citations: %+v", rep.Scored)
	}
}

func TestUtilityMalformedStateFailsClosed(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	root := utilityFixture(t, false)
	if err := os.WriteFile(filepath.Join(root, ".git", "pylon-utility.json"), []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewUtility(UtilityOptions{KBRoot: root, Now: now}); err == nil {
		t.Fatal("malformed telemetry state passed")
	}
}

func TestRecordInjectionsAppendsAndDedupesPerRun(t *testing.T) {
	root := utilityFixture(t, false)
	RecordInjections(root, []string{"quirks/trap.md", "quirks/trap.md", "plans/gone.md"})
	RecordInjections(root, []string{"quirks/trap.md"})
	data, err := os.ReadFile(filepath.Join(root, ".git", "pylon-utility.json"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("injection records wrong: %q", string(data))
	}
	if !strings.Contains(string(data), "\"path\":\"quirks/trap.md\"") {
		t.Fatalf("record shape wrong: %q", string(data))
	}
}

func TestRecordInjectionsWithoutGitDirIsSilent(t *testing.T) {
	root := t.TempDir() // no .git: telemetry has nowhere to land
	RecordInjections(root, []string{"quirks/trap.md"})
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("recorder created a .git directory")
	}
}
