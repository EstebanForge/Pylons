// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

func forcedFinding(file, msg string) report.Finding {
	return report.Finding{RuleID: "PYLON006", Severity: report.SeverityError, Grade: report.GradeForced, File: file, Message: msg}
}

func TestFingerprintIgnoresVolatileCounts(t *testing.T) {
	a := Fingerprint(forcedFinding("plans/x.md", "sentence has 25 words"))
	b := Fingerprint(forcedFinding("plans/x.md", "sentence has 26 words"))
	if a != b {
		t.Fatal("digit stripping must make count drift fingerprint-stable")
	}
	c := Fingerprint(forcedFinding("plans/x.md", "sentence has 25 words"))
	if a != c {
		t.Fatal("same finding must fingerprint identically")
	}
	if Fingerprint(forcedFinding("plans/y.md", "sentence has 25 words")) == a {
		t.Fatal("a different file must fingerprint differently")
	}
}

func TestApplyBaselineSplitsFindings(t *testing.T) {
	kb := t.TempDir()
	matched := forcedFinding("plans/x.md", "sentence has 25 words")
	other := forcedFinding("plans/y.md", "different sentence entirely")
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBaselineFile(t, kb, []BaselineEntry{{Rule: matched.RuleID, File: matched.File, Message: matched.Message, Count: 1}})

	kept, accepted, stale := ApplyBaseline(kb, []report.Finding{matched, other})
	if accepted != 1 {
		t.Errorf("accepted = %d, want 1", accepted)
	}
	if stale != 0 {
		t.Errorf("stale = %d, want 0", stale)
	}
	if len(kept) != 1 || kept[0].File != "plans/y.md" {
		t.Fatalf("kept = %+v, want only the unmatched finding", kept)
	}
}

func TestApplyBaselineReportsStaleEntries(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBaselineFile(t, kb, []BaselineEntry{{Rule: "PYLON006", File: "plans/gone.md", Message: "sentence has 25 words", Count: 1}})
	_, _, stale := ApplyBaseline(kb, nil)
	if stale != 1 {
		t.Fatalf("stale = %d, want 1 for an unmatched baseline entry", stale)
	}
}

func TestApplyBaselineNeverSuppressesBrokenConfig(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgFinding := report.Finding{RuleID: "PYLON001", Severity: report.SeverityError, Grade: report.GradeForced, File: "manifests/pylons.yaml", Message: "invalid manifests/pylons.yaml"}
	writeBaselineFile(t, kb, []BaselineEntry{{Rule: cfgFinding.RuleID, File: cfgFinding.File, Message: cfgFinding.Message, Count: 1}})
	kept, accepted, _ := ApplyBaseline(kb, []report.Finding{cfgFinding})
	if accepted != 0 || len(kept) != 1 {
		t.Fatalf("the config-error finding must never be suppressed: accepted=%d kept=%d", accepted, len(kept))
	}
}

func TestUpdateBaselineWritesForcedOnly(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	forced := forcedFinding("plans/x.md", "sentence has 25 words")
	advisory := report.Finding{RuleID: "PYLON034", Severity: report.SeverityAdvisory, Grade: report.GradeAdvisory, File: "domain/", Message: "no AGENTS.md"}
	if err := UpdateBaseline(kb, []report.Finding{forced, advisory}); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}
	b := readBaseline(t, kb)
	if len(b.Accepted) != 1 || b.Accepted[0].Rule != forced.RuleID || b.Accepted[0].File != forced.File {
		t.Fatalf("baseline = %+v, want only the FORCED entry", b.Accepted)
	}
}

func TestBaselineRoundTripSuppresses(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	matched := forcedFinding("plans/x.md", "sentence has 25 words")
	if err := UpdateBaseline(kb, []report.Finding{matched}); err != nil {
		t.Fatal(err)
	}
	kept, accepted, stale := ApplyBaseline(kb, []report.Finding{matched})
	if accepted != 1 || len(kept) != 0 || stale != 0 {
		t.Fatalf("round trip: accepted=%d kept=%d stale=%d", accepted, len(kept), stale)
	}
}

func writeBaselineFile(t *testing.T, kb string, entries []BaselineEntry) {
	t.Helper()
	b := Baseline{Version: 2, Accepted: entries}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "baseline.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBaseline(t *testing.T, kb string) Baseline {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(kb, "manifests", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestApplyBaselineNeverSuppressesExemptRules(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := report.Finding{RuleID: "PYLON060", Severity: report.SeverityError, Grade: report.GradeForced, File: "references/aws.md", Message: "possible AWS access key id"}
	writeBaselineFile(t, kb, []BaselineEntry{{Rule: secret.RuleID, File: secret.File, Message: secret.Message, Count: 1}})
	kept, accepted, stale := ApplyBaseline(kb, []report.Finding{secret})
	if accepted != 0 || len(kept) != 1 {
		t.Fatalf("an exempt rule must never be suppressed: accepted=%d kept=%d", accepted, len(kept))
	}
	if stale != 1 {
		t.Fatalf("an exempt-rule entry must report stale so it gets cleaned up: stale=%d", stale)
	}
}

func TestBaselineV2RoundTripCounts(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	three := forcedFinding("plans/a.md", "sentence exceeds 25 words under ASD-STE100")
	other := forcedFinding("plans/b.md", "sentence exceeds 25 words under ASD-STE100")
	// Three identical messages in one file: the count, not the hash,
	// must carry the multiplicity.
	if err := UpdateBaseline(kb, []report.Finding{three, three, three, other}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(baselinePath(kb))
	if err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Version  int `json:"version"`
		Accepted []struct {
			Rule    string `json:"rule"`
			File    string `json:"file"`
			Message string `json:"message"`
			Count   int    `json:"count"`
		} `json:"accepted"`
	}
	if json.Unmarshal(raw, &disk) != nil {
		t.Fatalf("baseline must stay human-readable JSON: %v", err)
	}
	if disk.Version != 2 {
		t.Fatalf("version = %d, want 2", disk.Version)
	}
	if len(disk.Accepted) != 2 || disk.Accepted[0].Count != 3 || disk.Accepted[1].Count != 1 {
		t.Fatalf("counts wrong: %+v", disk.Accepted)
	}
	if disk.Accepted[0].File != "plans/a.md" || disk.Accepted[0].Message == "" {
		t.Fatalf("entries must carry the finding, not a hash: %+v", disk.Accepted)
	}
	// Apply suppresses exactly what the count carries; a fourth twin
	// and the other file's finding stay new.
	fourth := append(mkFindings(three, 4), other)
	kept, accepted, _ := ApplyBaseline(kb, fourth)
	if accepted != 4 {
		t.Fatalf("accepted = %d, want 4 (3 counted + 1 for the other file)", accepted)
	}
	if len(kept) != 1 || kept[0].File != "plans/a.md" {
		t.Fatalf("the fourth twin must stay: kept=%v", kept)
	}
}

func mkFindings(f report.Finding, n int) []report.Finding {
	out := make([]report.Finding, n)
	for i := range out {
		out[i] = f
	}
	return out
}

func TestBaselineV1RejectedLoudly(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath(kb), []byte(`{"version": 1, "fingerprints": ["deadbeef"}`+""+`]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Correct the JSON body: the literal above must be valid v1 JSON.
	if err := os.WriteFile(baselinePath(kb), []byte("{\"version\": 1, \"fingerprints\": [\"deadbeef\"]}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := forcedFinding("plans/a.md", "sentence exceeds 25 words under ASD-STE100")
	kept, accepted, _ := ApplyBaseline(kb, []report.Finding{f})
	if accepted != 0 {
		t.Fatalf("a v1 baseline must suppress nothing, accepted=%d", accepted)
	}
	found := false
	for _, k := range kept {
		if k.RuleID == "PYLON001" && k.File == "manifests/baseline.json" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a v1 baseline must surface a loud finding, kept=%v", kept)
	}
}

func TestBaselineV2StaleCountReports(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := UpdateBaseline(kb, mkFindings(forcedFinding("plans/a.md", "sentence exceeds 25 words under ASD-STE100"), 3)); err != nil {
		t.Fatal(err)
	}
	kept, accepted, stale := ApplyBaseline(kb, mkFindings(forcedFinding("plans/a.md", "sentence exceeds 25 words under ASD-STE100"), 1))
	if accepted != 1 || len(kept) != 0 {
		t.Fatalf("the matching finding suppresses: accepted=%d kept=%d", accepted, len(kept))
	}
	if stale != 1 {
		t.Fatalf("an over-accepted entry is already stale: stale=%d", stale)
	}
	// Debt paid down: the entry now over-accepts, which is stale.
	_, _, stale = ApplyBaseline(kb, nil)
	if stale != 1 {
		t.Fatalf("an over-accepted entry must report stale, stale=%d", stale)
	}
}
