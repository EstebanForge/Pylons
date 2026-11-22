// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"
	"time"
)

func retroFindings(t *testing.T, dir string, records ...string) []byte {
	t.Helper()
	return []byte(strings.Join(records, "\n") + "\n")
}

func TestRetroGroupsSignaturesAndOpensOneCandidate(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"2026-06-01","lens":"review","rule":"","contract":"","path":"svc/src/api.go","text":"the queue replays on retry","source":"pr 1"}`,
		`{"date":"2026-06-20","lens":"review","rule":"","contract":"","path":"svc/src/api.go","text":"replays happen when retries race","source":"pr 7"}`,
		`{"date":"2026-06-25","lens":"review","rule":"","contract":"","path":"other/elsewhere.go","text":"unrelated singleton finding","source":"pr 9"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 1 {
		t.Fatalf("exactly one candidate expected, got %+v", rep.Candidates)
	}
	c := rep.Candidates[0]
	if c.Count != 2 {
		t.Fatalf("count wrong: %+v", c)
	}
	if c.Shape != "convention" {
		t.Fatalf("default shape must be convention, got %s", c.Shape)
	}
	if !strings.Contains(c.Scaffold, "conventions/proposals/") && !strings.Contains(c.Summary, "conventions/proposals/") {
		t.Fatalf("convention candidate must name the proposals path: %+v", c)
	}
	if rep.Examined != 3 || rep.Signatures != 2 {
		t.Fatalf("totals wrong: %+v", rep)
	}
}

func TestRetroWindowFiltersOldRecords(t *testing.T) {
	old := time.Now().AddDate(0, 0, -400).Format("2006-01-02")
	recent := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	findings := retroFindings(t, "",
		`{"date":"`+old+`","lens":"review","rule":"","contract":"","path":"svc/x.go","text":"old finding","source":"pr 1"}`,
		`{"date":"`+recent+`","lens":"review","rule":"","contract":"","path":"svc/x.go","text":"recent finding","source":"pr 2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "30d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 0 {
		t.Fatalf("records outside the window must not accumulate: %+v", rep.Candidates)
	}
	if rep.Examined != 1 {
		t.Fatalf("examined must count only in-window records: %+v", rep)
	}
}

func TestRetroShapeAgreementAndOverride(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"2026-06-01","lens":"review","rule":"PYLON006","contract":"","path":"","shape":"lint-rule","text":"a","source":"p1"}`,
		`{"date":"2026-06-02","lens":"review","rule":"PYLON006","contract":"","path":"","shape":"lint-rule","text":"b","source":"p2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if rep.Candidates[0].Shape != "lint-rule" {
		t.Fatalf("unanimous shape must win: %+v", rep.Candidates[0])
	}
	rep, err = ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2, Shape: "falsifier"})
	if err != nil {
		t.Fatalf("retro override: %v", err)
	}
	if rep.Candidates[0].Shape != "falsifier" {
		t.Fatalf("--shape must override: %+v", rep.Candidates[0])
	}
	if !strings.Contains(rep.Candidates[0].Summary, "wrong_if") && !strings.Contains(rep.Candidates[0].Scaffold, "wrong_if") {
		t.Fatalf("falsifier candidate must carry the wrong_if entry: %+v", rep.Candidates[0])
	}
}

func TestRetroShapeDisagreementFallsBackToConvention(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"2026-06-01","lens":"review","rule":"","contract":"","path":"svc/x.go","shape":"lint-rule","text":"a","source":"p1"}`,
		`{"date":"2026-06-02","lens":"review","rule":"","contract":"","path":"svc/x.go","shape":"convention","text":"b","source":"p2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if rep.Candidates[0].Shape != "convention" {
		t.Fatalf("disagreement must fall back to convention: %+v", rep.Candidates[0])
	}
}

func TestRetroFailsClosedOnMalformedRecords(t *testing.T) {
	if _, err := ReviewRetro(RetroOptions{Findings: []byte("{not json}\n"), Window: "30d", Min: 2}); err == nil {
		t.Fatal("malformed record must fail closed")
	}
	if _, err := ReviewRetro(RetroOptions{Findings: []byte(`{"date":"not-a-date","lens":"r","text":"x"}` + "\n"), Window: "30d", Min: 2}); err == nil {
		t.Fatal("unparsable record date must fail closed")
	}
	if _, err := ReviewRetro(RetroOptions{Findings: retroFindings(t, "", `{"date":"2026-06-01","lens":"r","text":"x"}`), Window: "bogus", Min: 2}); err == nil {
		t.Fatal("unparsable window must fail closed")
	}
}

func TestRetroEmptyFindingsIsClean(t *testing.T) {
	rep, err := ReviewRetro(RetroOptions{Findings: []byte(""), Window: "30d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 0 || rep.Examined != 0 {
		t.Fatalf("empty findings must be a clean run: %+v", rep)
	}
}
