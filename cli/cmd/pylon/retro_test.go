// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRetroFlagPath(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := filepath.Join(base, "findings.jsonl")
	if err := os.WriteFile(findings, []byte(
		`{"date":"`+retroDay(2)+`","lens":"review","rule":"","contract":"","path":"svc/x.go","text":"replays on retry","source":"pr 1"}`+"\n"+
			`{"date":"`+retroDay(0)+`","lens":"review","rule":"","contract":"","path":"svc/x.go","text":"replay race again","source":"pr 4"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(base, "bad.jsonl")
	if err := os.WriteFile(bad, []byte("{nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Missing --findings is a usage error.
	if code := run([]string{"review", "retro", "--pylons", base}); code != 2 {
		t.Fatalf("missing --findings must exit 2, got %d", code)
	}
	// Malformed records fail closed.
	if code := run([]string{"review", "retro", "--pylons", base, "--findings", bad, "--window", "30d"}); code != 2 {
		t.Fatalf("malformed findings must exit 2, got %d", code)
	}
	// Non-integer --min is a usage error.
	if code := run([]string{"review", "retro", "--pylons", base, "--findings", findings, "--min", "x"}); code != 2 {
		t.Fatalf("non-integer --min must exit 2, got %d", code)
	}
	// Zero is not a threshold.
	if code := run([]string{"review", "retro", "--pylons", base, "--findings", findings, "--min", "0"}); code != 2 {
		t.Fatalf("--min 0 must exit 2, got %d", code)
	}

	stdout := captureStdout(t, func() {
		if code := run([]string{"review", "retro", "--pylons", base, "--findings", findings, "--window", "365d"}); code != 0 {
			t.Fatalf("a real retro must exit 0, got %d", code)
		}
	})
	if !strings.Contains(stdout, "conventions/proposals/") {
		t.Fatalf("artifact must carry the promotion candidate, got: %s", stdout)
	}
	if !strings.Contains(stdout, "2 repeats") {
		t.Fatalf("artifact must carry the repeat count, got: %s", stdout)
	}
}

// TestHarvestRecordFeedsRetro proves the loop: a harvest with --record
// appends findings that a later retro turns into a candidate.
func TestHarvestRecordFeedsRetro(t *testing.T) {
	base := t.TempDir()
	kb := filepath.Join(base, "kb")
	for _, d := range []string{filepath.Join(kb, "manifests"), filepath.Join(kb, "svc")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "svc", "api.go"), []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(base, "pr.diff")
	if err := os.WriteFile(diffPath, []byte("+++ b/svc/api.go\n@@ -1,1 +1,2 @@\n package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(base, "meta.json")
	if err := os.WriteFile(metaPath, []byte(`{"commits":[{"subject":"fix: replay","body":"Footgun: retry storms on replay."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(base, "findings.jsonl")
	stdout := captureStdout(t, func() {
		if code := run([]string{"harvest", "pr", "--pylons", kb, "--diff", diffPath, "--metadata", metaPath, "--record", recordPath}); code != 0 {
			t.Fatalf("harvest with --record must exit 0, got %d", code)
		}
	})
	if !strings.Contains(stdout, "quirks/proposals/") {
		t.Fatalf("harvest artifact missing quirk proposal: %s", stdout)
	}
	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("record file must exist: %v", err)
	}
	if !strings.Contains(string(recorded), "retry storms on replay") {
		t.Fatalf("record must carry the marker text: %s", recorded)
	}
	// One occurrence: below the default threshold of 2.
	stdout = captureStdout(t, func() {
		if code := run([]string{"review", "retro", "--pylons", kb, "--findings", recordPath, "--window", "30d"}); code != 0 {
			t.Fatalf("retro must exit 0, got %d", code)
		}
	})
	if !strings.Contains(stdout, "no repeats") {
		t.Fatalf("one record must stay under the threshold: %s", stdout)
	}
	// A second merge with the same marker crosses it.
	stdout = captureStdout(t, func() {
		if code := run([]string{"harvest", "pr", "--pylons", kb, "--diff", diffPath, "--metadata", metaPath, "--record", recordPath}); code != 0 {
			t.Fatalf("second harvest must exit 0, got %d", code)
		}
		if code := run([]string{"review", "retro", "--pylons", kb, "--findings", recordPath, "--window", "30d"}); code != 0 {
			t.Fatalf("second retro must exit 0, got %d", code)
		}
	})
	if !strings.Contains(stdout, "conventions/proposals/") || !strings.Contains(stdout, "2 repeats") {
		t.Fatalf("two occurrences must open one candidate: %s", stdout)
	}
}

func retroDay(daysAgo int) string {
	return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
}
