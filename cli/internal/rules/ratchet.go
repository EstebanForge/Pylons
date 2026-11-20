// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// RatchetBaseline is the committed advisory budget: per-package and
// per-zone advisory counts from a full lint run.
type RatchetBaseline struct {
	Counts map[string]int `json:"counts"`
}

// ratchetBaselinePath is the baseline file inside the ledger; a human
// commits the updated baseline after reviewing it.
func ratchetBaselinePath(kbRoot string) string {
	return filepath.Join(kbRoot, "manifests", "ratchet.json")
}

// clockDrivenRules drift without any edit: package staleness
// (PYLON015), audit reaudit dates (PYLON026), plan branches (PYLON027),
// and cross-repo import edges (PYLON043), which move with sibling
// repos the ratchet baseline cannot see
// local checkouts (PYLON027). They would make the ratchet fail with no
// change to the corpus, so they are outside the budget.
var clockDrivenRules = map[string]bool{"PYLON015": true, "PYLON026": true, "PYLON027": true, "PYLON043": true}

// RatchetOptions controls the advisory ratchet run.
type RatchetOptions struct {
	Update        bool
	AllowExternal bool
}

// RunRatchet compares current advisory counts against the committed
// baseline. A count above its baseline is a FORCED regression; --update
// rewrites the baseline as a reviewable candidate.
func RunRatchet(kbRoot string, opts RatchetOptions) ([]report.Finding, error) {
	findings := Lint(kbRoot, LintOptions{HasCode: true, AllowExternal: opts.AllowExternal})
	// A manifest that does not load degrades every scan to a single
	// PYLON001 error; the counts would be empty and --update would
	// clobber a good baseline with it. Fail closed instead.
	for _, f := range findings {
		if f.Severity == report.SeverityError && strings.HasSuffix(f.File, "manifests/pylons.yaml") {
			return nil, fmt.Errorf("manifests/pylons.yaml does not load: %s", f.Message)
		}
	}
	counts := advisoryCounts(findings)

	if opts.Update {
		if err := writeRatchetBaseline(kbRoot, counts); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "ratchet: baseline written with %d advisories across %d groups\n", len(findings), len(counts))
		return nil, nil
	}

	raw, err := os.ReadFile(ratchetBaselinePath(kbRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return []report.Finding{{
				RuleID:      "PYLON042",
				Severity:    report.SeverityAdvisory,
				Grade:       report.GradeAdvisory,
				File:        "manifests/ratchet.json",
				Line:        1,
				Message:     "no advisory baseline; the ratchet adopts on the first --update",
				Remediation: "Run pylon review ratchet --update to create manifests/ratchet.json, review it, and commit it.",
			}}, nil
		}
		return nil, err
	}
	var baseline RatchetBaseline
	if err := json.Unmarshal(raw, &baseline); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ratchetBaselinePath(kbRoot), err)
	}
	var out []report.Finding
	for _, key := range slices.Sorted(maps.Keys(counts)) {
		base := baseline.Counts[key]
		if counts[key] > base {
			out = append(out, ratchetRegression(key, counts[key], base, findings))
		}
	}
	return out, nil
}

// writeRatchetBaseline stores the baseline atomically so a crash cannot
// leave a truncated file behind.
func writeRatchetBaseline(kbRoot string, counts map[string]int) error {
	data, err := json.MarshalIndent(RatchetBaseline{Counts: counts}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ratchet baseline: %w", err)
	}
	data = append(data, '\n')
	path := ratchetBaselinePath(kbRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "ratchet-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ratchetRegression builds the PYLON042 finding, naming the advisory
// files behind the rise so the finding is actionable on its own.
func ratchetRegression(key string, cur, base int, findings []report.Finding) report.Finding {
	var culprits []string
	for _, f := range findings {
		if f.Severity == report.SeverityAdvisory && advisoryKey(f.File) == key {
			culprits = append(culprits, f.File)
		}
	}
	slices.Sort(culprits)
	detail := ""
	if len(culprits) > 0 {
		if len(culprits) > 5 {
			detail = fmt.Sprintf(" (%s and %d more)", strings.Join(culprits[:5], ", "), len(culprits)-5)
		} else {
			detail = fmt.Sprintf(" (%s)", strings.Join(culprits, ", "))
		}
	}
	return report.Finding{
		RuleID:   "PYLON042",
		Severity: report.SeverityError,
		Grade:    report.GradeForced,
		File:     "manifests/ratchet.json",
		Line:     1,
		Message:  fmt.Sprintf("advisory count for %s rose to %d (baseline %d)%s", key, cur, base, detail),
	}
}

// advisoryCounts groups advisory findings by package (packages/<pkg>.md)
// or by zone for everything else, outside the clock-driven rules.
func advisoryCounts(findings []report.Finding) map[string]int {
	counts := make(map[string]int)
	for _, f := range findings {
		if f.Severity != report.SeverityAdvisory || clockDrivenRules[f.RuleID] {
			continue
		}
		counts[advisoryKey(f.File)]++
	}
	return counts
}

// advisoryKey attributes a finding to its owning package or zone.
func advisoryKey(file string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(file), "./")
	if pkg, rest, ok := strings.Cut(rel, "/"); ok && pkg == "packages" && rest != "" && rest != "AGENTS.md" && rest != "README.md" && !strings.Contains(rest, "/") {
		return "package:" + strings.TrimSuffix(rest, ".md")
	}
	zone, _, _ := strings.Cut(rel, "/")
	if zone == rel {
		return "zone:root"
	}
	return "zone:" + zone
}
