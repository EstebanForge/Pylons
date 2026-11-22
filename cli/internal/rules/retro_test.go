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
		`{"date":"`+retroDay(10)+`","lens":"review","rule":"","contract":"","path":"svc/src/api.go","text":"the queue replays on retry","source":"pr 1"}`,
		`{"date":"`+retroDay(2)+`","lens":"review","rule":"","contract":"","path":"svc/src/api.go","text":"replays happen when retries race","source":"pr 7"}`,
		`{"date":"`+retroDay(0)+`","lens":"review","rule":"","contract":"","path":"other/elsewhere.go","text":"unrelated singleton finding","source":"pr 9"}`,
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
		`{"date":"`+retroDay(10)+`","lens":"review","rule":"PYLON006","contract":"repo://svc/plans/x.md::Gate","path":"","shape":"lint-rule","text":"a","source":"p1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","rule":"PYLON006","contract":"repo://svc/plans/x.md::Gate","path":"","shape":"lint-rule","text":"b","source":"p2"}`,
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
		`{"date":"`+retroDay(10)+`","lens":"review","rule":"","contract":"","path":"svc/x.go","shape":"lint-rule","text":"a","source":"p1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","rule":"","contract":"","path":"svc/x.go","shape":"convention","text":"b","source":"p2"}`,
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
	if _, err := ReviewRetro(RetroOptions{Findings: retroFindings(t, "", `{"date":"`+retroDay(10)+`","lens":"r","text":"x"}`), Window: "bogus", Min: 2}); err == nil {
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

func TestRetroUnstructuredRecordsNeedMatchingText(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"`+retroDay(10)+`","lens":"review","text":"the queue replays on retry","source":"pr 1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","text":"the cache evicts on boot","source":"pr 2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 0 || rep.Signatures != 2 {
		t.Fatalf("differently-worded free text must never accumulate: %+v", rep)
	}
}

func TestRetroUnknownRecordShapeFailsClosed(t *testing.T) {
	findings := retroFindings(t, "", `{"date":"`+retroDay(10)+`","lens":"r","shape":"bogus","text":"x"}`)
	if _, err := ReviewRetro(RetroOptions{Findings: findings, Window: "30d", Min: 2}); err == nil {
		t.Fatal("a record with an unknown shape must fail closed")
	}
}

func TestRetroFalsifierWithoutContractFallsBack(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"`+retroDay(10)+`","lens":"review","shape":"falsifier","text":"the flag leaks state","source":"p1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","shape":"falsifier","text":"the flag leaks state","source":"p2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if rep.Candidates[0].Shape != "convention" {
		t.Fatalf("falsifier without a contract must fall back to convention: %+v", rep.Candidates[0])
	}
}

func TestRetroSlugCollisionsDisambiguated(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"`+retroDay(10)+`","lens":"review","rule":"PYLON006","path":"svc/a.go","text":"the queue replays on retry","source":"p1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","rule":"PYLON006","path":"svc/a.go","text":"the queue replays on retry","source":"p2"}`,
		`{"date":"`+retroDay(6)+`","lens":"review","rule":"PYLON007","path":"svc/b.go","text":"the queue replays on retry storm","source":"p3"}`,
		`{"date":"`+retroDay(4)+`","lens":"review","rule":"PYLON007","path":"svc/b.go","text":"the queue replays on retry storm","source":"p4"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 2 {
		t.Fatalf("two distinct signatures expected: %+v", rep.Candidates)
	}
	a, b := rep.Candidates[0].Summary, rep.Candidates[1].Summary
	if a == b {
		t.Fatalf("colliding slugs must disambiguate: %s", a)
	}
}

func TestRetroSanitizesControlsAndDots(t *testing.T) {
	findings := retroFindings(t, "",
		`{"date":"`+retroDay(10)+`","lens":"review","rule":"","contract":"","path":"svc\n## fake\nx.go","text":"ends with period.","source":"pr\u001b[2J1"}`,
		`{"date":"`+retroDay(8)+`","lens":"review","rule":"","contract":"","path":"svc\n## fake\nx.go","text":"ends with period.","source":"pr 2"}`,
	)
	rep, err := ReviewRetro(RetroOptions{Findings: findings, Window: "365d", Min: 2})
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if len(rep.Candidates) != 1 {
		t.Fatalf("one candidate expected: %+v", rep.Candidates)
	}
	all := rep.Candidates[0].Signature + rep.Candidates[0].Scaffold + rep.Candidates[0].Summary
	for _, bad := range []string{"\x1b", "\n## fake", ".."} {
		if strings.Contains(all, bad) {
			t.Fatalf("artifact must not carry %q: %s", bad, all)
		}
	}
}

func retroDay(daysAgo int) string {
	return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
}
