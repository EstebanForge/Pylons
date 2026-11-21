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
	fps := []string{Fingerprint(matched)}
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBaselineFile(t, kb, fps)

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
	writeBaselineFile(t, kb, []string{"deadbeef"})
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
	writeBaselineFile(t, kb, []string{Fingerprint(cfgFinding)})
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
	if len(b.Fingerprints) != 1 || b.Fingerprints[0] != Fingerprint(forced) {
		t.Fatalf("baseline = %v, want only the FORCED fingerprint", b.Fingerprints)
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

func writeBaselineFile(t *testing.T, kb string, fps []string) {
	t.Helper()
	b := Baseline{Version: 1, Fingerprints: fps}
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
