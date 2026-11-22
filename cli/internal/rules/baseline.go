// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// A FORCED baseline lets a ledger adopt with known debt: findings
// whose fingerprint sits in manifests/baseline.json stop blocking,
// while anything new still fails. The baseline is expected to shrink
// as the debt is paid; entries that no longer match are reported so
// the file cannot rot.

// volatileNumRe strips word counts from messages so count drift
// inside a message (a sentence now at 26 words) keeps the same key.
// Digits anywhere else stay literal: path-numbered targets such as
// plans/0042.md must not collapse into one key and swap silently.
var volatileNumRe = regexp.MustCompile(`[0-9]+ words`)

// Baseline is the on-disk accepted-debt ledger. Entries carry the
// finding itself, not a hash, so reviewers can read what debt the
// ledger accepted; Count carries multiplicity because one entry
// groups repeated messages in one file.
type Baseline struct {
	Version  int             `json:"version"`
	Accepted []BaselineEntry `json:"accepted"`
}

// BaselineEntry is one accepted finding kind.
type BaselineEntry struct {
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// baselineKey groups findings the way the baseline judges them: the
// message with volatile digits stripped, no line number.
func baselineKey(f report.Finding) string {
	return f.RuleID + "\x00" + f.File + "\x00" + volatileNumRe.ReplaceAllString(f.Message, "# words")
}

func baselinePath(kbRoot string) string {
	return filepath.Join(kbRoot, "manifests", "baseline.json")
}

// baselineExemptRules never enter the baseline: a suppressed secret
// would hide every later secret of the same kind in the file, and a
// silenced gate surface defeats the guard itself.
var baselineExemptRules = map[string]bool{
	"PYLON060": true,
	"PYLON062": true,
}

// UpdateBaseline writes the current FORCED findings as the accepted
// set. A config-error finding is never recorded: a broken
// manifests/pylons.yaml must always block.
func UpdateBaseline(kbRoot string, findings []report.Finding) error {
	counts := map[string]*BaselineEntry{}
	for _, f := range findings {
		if f.Grade != report.GradeForced || f.File == "manifests/pylons.yaml" || baselineExemptRules[f.RuleID] {
			continue
		}
		if e, ok := counts[baselineKey(f)]; ok {
			e.Count++
			continue
		}
		counts[baselineKey(f)] = &BaselineEntry{Rule: f.RuleID, File: f.File, Message: f.Message, Count: 1}
	}
	accepted := make([]BaselineEntry, 0, len(counts))
	for _, e := range counts {
		accepted = append(accepted, *e)
	}
	slices.SortFunc(accepted, func(a, b BaselineEntry) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := strings.Compare(a.Rule, b.Rule); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
	data, err := json.MarshalIndent(Baseline{Version: 1, Accepted: accepted}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(baselinePath(kbRoot), data, 0o644)
}

// ApplyBaseline drops findings accepted by manifests/baseline.json
// and reports how many were suppressed and how many baseline entries
// no longer match anything.
func ApplyBaseline(kbRoot string, findings []report.Finding) (kept []report.Finding, accepted, stale int) {
	data, err := os.ReadFile(baselinePath(kbRoot))
	if err != nil {
		return findings, 0, 0
	}
	var b Baseline
	if json.Unmarshal(data, &b) != nil || b.Version != 1 {
		// The version field guards foreign files only; the format
		// always carried counts, and nothing predates it.
		reject := errFinding("PYLON001", "manifests/baseline.json", 1,
			"baseline format is not version 1; regenerate with pylon lint --update-baseline")
		out := findings[:0:0]
		out = append(out, findings...)
		out = append(out, reject)
		return out, 0, 0
	}
	budget := map[string]int{}
	entryExists := map[string]bool{}
	for _, e := range b.Accepted {
		key := e.Rule + "\x00" + e.File + "\x00" + volatileNumRe.ReplaceAllString(e.Message, "# words")
		budget[key] += e.Count
		entryExists[key] = true
	}
	matched := map[string]int{}
	kept = findings[:0:0]
	for _, f := range findings {
		key := baselineKey(f)
		if f.Grade == report.GradeForced && f.File != "manifests/pylons.yaml" && !baselineExemptRules[f.RuleID] && matched[key] < budget[key] {
			matched[key]++
			accepted++
			continue
		}
		kept = append(kept, f)
	}
	for key, count := range budget {
		if matched[key] < count || !entryExists[key] {
			stale++
		}
	}
	return kept, accepted, stale
}
