// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func fusionDoc(path, status, updated string, components ...string) docEntry {
	return docEntry{Path: path, Status: status, Updated: updated, Components: components, Title: path}
}

func paths(docs []docEntry) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Path)
	}
	return out
}

func TestFusionPrefersLiveStatus(t *testing.T) {
	cands := []docEntry{
		fusionDoc("quirks/old.md", "abandoned", "2024-01-01"),
		fusionDoc("plans/live.md", "accepted", "2024-01-01"),
	}
	got := rerank([]string{"quirks/old.md", "plans/live.md"}, map[string]float64{"quirks/old.md": 1.0, "plans/live.md": 1.0}, cands, nil, nil)
	if p := paths(got); p[0] != "plans/live.md" {
		t.Fatalf("order %v; live status must outrank a terminal one at equal score", p)
	}
}

func TestFusionPrefersRecentUpdates(t *testing.T) {
	recent := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	stale := time.Now().AddDate(0, 0, -400).Format("2006-01-02")
	cands := []docEntry{
		fusionDoc("quirks/stale.md", "active", stale),
		fusionDoc("quirks/fresh.md", "active", recent),
	}
	got := rerank([]string{"quirks/stale.md", "quirks/fresh.md"}, map[string]float64{"quirks/stale.md": 1.0, "quirks/fresh.md": 1.0}, cands, nil, nil)
	if p := paths(got); p[0] != "quirks/fresh.md" {
		t.Fatalf("order %v; a recent update must outrank a stale one at equal score", p)
	}
}

func TestFusionPrefersExactRuleIdentifier(t *testing.T) {
	withRule := fusionDoc("plans/rule.md", "active", "2024-01-01")
	withRule.Body = "The gate reports PYLON005 when a term is unlinked."
	without := fusionDoc("docs/mention.md", "active", "2024-01-01")
	without.Body = "A passing mention of related lint ideas."
	cands := []docEntry{without, withRule}
	got := rerank([]string{"docs/mention.md", "plans/rule.md"}, map[string]float64{"docs/mention.md": 1.0, "plans/rule.md": 1.0}, cands, nil, []string{"pylon005"})
	if p := paths(got); p[0] != "plans/rule.md" {
		t.Fatalf("order %v; the doc carrying the exact rule id must outrank prose mentions", p)
	}
}

func TestFusionPrefersQueryProject(t *testing.T) {
	near := fusionDoc("plans/near.md", "active", "2024-01-01", "svc")
	far := fusionDoc("plans/far.md", "active", "2024-01-01", "other")
	cands := []docEntry{far, near}
	got := rerank([]string{"plans/far.md", "plans/near.md"}, map[string]float64{"plans/far.md": 1.0, "plans/near.md": 1.0}, cands, []string{"svc"}, nil)
	if p := paths(got); p[0] != "plans/near.md" {
		t.Fatalf("order %v; documents owned by the queried project must outrank others", p)
	}
}

func TestFusionNeverInvertsStrongScores(t *testing.T) {
	weak := fusionDoc("plans/weak.md", "accepted", "2026-06-01")
	strong := fusionDoc("quirks/strong.md", "abandoned", "2020-01-01")
	cands := []docEntry{weak, strong}
	got := rerank([]string{"plans/weak.md", "quirks/strong.md"}, map[string]float64{"plans/weak.md": 1.0, "quirks/strong.md": 10.0}, cands, []string{"svc"}, []string{"pylon005"})
	if p := paths(got); p[0] != "quirks/strong.md" {
		t.Fatalf("order %v; fusion boosts must stay smaller than a dominant BM25 gap", p)
	}
}

func TestFusionIsDeterministic(t *testing.T) {
	cands := []docEntry{
		fusionDoc("a.md", "active", "2026-01-01"),
		fusionDoc("b.md", "active", "2026-01-01"),
		fusionDoc("c.md", "active", "2026-01-01"),
	}
	first := rerank([]string{"a.md", "b.md", "c.md"}, map[string]float64{"a.md": 1.0, "b.md": 1.0, "c.md": 1.0}, cands, nil, nil)
	second := rerank([]string{"a.md", "b.md", "c.md"}, map[string]float64{"a.md": 1.0, "b.md": 1.0, "c.md": 1.0}, cands, nil, nil)
	if got, want := paths(first), paths(second); len(got) != len(want) {
		t.Fatal("rerank length changed between runs")
	} else {
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("order %v then %v; rerank must be deterministic", want, got)
			}
		}
	}
}
