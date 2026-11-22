// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// A FORCED baseline lets a ledger adopt with known debt: findings
// whose fingerprint sits in manifests/baseline.json stop blocking,
// while anything new still fails. The baseline is expected to shrink
// as the debt is paid; entries that no longer match are reported so
// the file cannot rot.

// volatileNumRe strips digits from messages so count drift inside a
// message (a sentence now at 26 words) keeps the same fingerprint.
var volatileNumRe = regexp.MustCompile(`[0-9]+`)

// Baseline is the on-disk accepted-debt ledger.
type Baseline struct {
	Version      int      `json:"version"`
	Fingerprints []string `json:"fingerprints"`
}

func baselinePath(kbRoot string) string {
	return filepath.Join(kbRoot, "manifests", "baseline.json")
}

// Fingerprint identifies one accepted finding: rule, file, and the
// message with volatile digits stripped. Line numbers are excluded
// on purpose; edits above the finding must not orphan the entry.
func Fingerprint(f report.Finding) string {
	stable := f.RuleID + "\x00" + f.File + "\x00" + volatileNumRe.ReplaceAllString(f.Message, "#")
	sum := sha256.Sum256([]byte(stable))
	return hex.EncodeToString(sum[:])
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
	var fps []string
	for _, f := range findings {
		if f.Grade != report.GradeForced || f.File == "manifests/pylons.yaml" || baselineExemptRules[f.RuleID] {
			continue
		}
		fps = append(fps, Fingerprint(f))
	}
	slices.Sort(fps)
	fps = slices.Compact(fps)
	data, err := json.MarshalIndent(Baseline{Version: 1, Fingerprints: fps}, "", "  ")
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
	if json.Unmarshal(data, &b) != nil {
		return findings, 0, 0
	}
	entries := map[string]bool{}
	for _, fp := range b.Fingerprints {
		entries[fp] = true
	}
	matched := map[string]bool{}
	kept = findings[:0:0]
	for _, f := range findings {
		fp := Fingerprint(f)
		if f.Grade == report.GradeForced && f.File != "manifests/pylons.yaml" && entries[fp] {
			accepted++
			matched[fp] = true
			continue
		}
		kept = append(kept, f)
	}
	for fp := range entries {
		if !matched[fp] {
			stale++
		}
	}
	return kept, accepted, stale
}
