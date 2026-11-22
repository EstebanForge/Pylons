// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Fusion weights are fixed in code on purpose: ranking must stay
// deterministic across machines and CI runs, and the retrieval recall
// suite gates every change to this table.
const (
	fusionStatusBoost   = 0.15
	fusionRecencyBoost  = 0.10
	fusionIdentifierHit = 0.40
	fusionProjectBoost  = 0.25
	fusionRecentDays    = 90
	fusionKnownDays     = 365
)

// statusTier ranks lifecycle relevance: live work outranks terminal or
// archival records at equal relevance.
var statusTier = map[string]int{
	"active": 2, "accepted": 2, "in-progress": 2,
	"proposed": 1, "changed": 1, "resolved": 1, "review": 1,
}

func docTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func isRuleIdToken(t string) bool {
	rest, ok := strings.CutPrefix(t, "pylon")
	if !ok {
		return false
	}
	return (len(rest) == 3 || len(rest) == 4) && allDigits(rest)
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// fusionBoost returns the multiplier for one document's BM25 score.
func fusionBoost(d docEntry, queryTokens []string, projs []string) float64 {
	boost := 1.0 + fusionStatusBoost*float64(statusTier[d.Status])

	if updated, err := time.Parse("2006-01-02", d.Updated); err == nil {
		age := time.Since(updated)
		switch {
		case age <= time.Duration(fusionRecentDays)*24*time.Hour:
			boost += fusionRecencyBoost * 2
		case age <= time.Duration(fusionKnownDays)*24*time.Hour:
			boost += fusionRecencyBoost
		}
	}

	bodyTokens := docTokens(d.Body)
	for _, q := range queryTokens {
		if isRuleIdToken(q) && slices.Contains(bodyTokens, q) {
			boost += fusionIdentifierHit
			break
		}
	}

	if slices.Contains(projs, d.Package) {
		boost += fusionProjectBoost
	} else {
		for _, c := range d.Components {
			if slices.Contains(projs, c) {
				boost += fusionProjectBoost
				break
			}
		}
	}
	return boost
}

// rerank applies fusionBoost to BM25 scores and returns the documents in
// fused order, strongest first, ties broken by path for determinism.
func rerank(ids []string, scores map[string]float64, docs []docEntry, projs []string, queryTokens []string) []docEntry {
	byPath := make(map[string]docEntry, len(docs))
	for _, d := range docs {
		byPath[d.Path] = d
	}
	type fused struct {
		doc   docEntry
		score float64
	}
	out := make([]fused, 0, len(ids))
	for _, id := range ids {
		d, ok := byPath[id]
		if !ok {
			continue
		}
		out = append(out, fused{doc: d, score: scores[id] * fusionBoost(d, queryTokens, projs)})
	}
	slices.SortFunc(out, func(a, b fused) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return strings.Compare(a.doc.Path, b.doc.Path)
	})
	result := make([]docEntry, 0, len(out))
	for _, f := range out {
		result = append(result, f.doc)
	}
	return result
}

// queryProjectAliases returns the registered project aliases whose roots
// contain the working directory, so proximity ranking reflects where the
// query runs from.
func queryProjectAliases(kbRoot string) []string {
	cfg := loadConfigOrWarn(kbRoot)
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Projects))
	for _, alias := range slices.Sorted(maps.Keys(cfg.Projects)) {
		root := cfg.ProjectPath(kbRoot, alias)
		if root == "" {
			continue
		}
		if cwd == root || strings.HasPrefix(cwd, root+string(filepath.Separator)) {
			out = append(out, alias)
		}
	}
	return out
}
