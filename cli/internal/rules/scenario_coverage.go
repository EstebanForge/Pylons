// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// scenarioRuleID is PYLON046: a slice plan's behavior contracts must
// appear in at least one declared scenario.
const scenarioRuleID = "PYLON046"

// scenarioCovers returns the contract ids one scenario covers.
func scenarioCovers(entry map[string]any) []string {
	raw, ok := entry["covers"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// checkScenarioCoverage applies CONTRACT_NO_SCENARIO to slice plans.
// Scenarios run in the mapped repo's QA suite through verified_by; the
// engine only checks the declared coverage intersection.
func checkScenarioCoverage(pf resolve.PlanFrontMatter, rel string) []report.Finding {
	if !pf.Slice {
		return nil
	}
	if !strings.HasPrefix(rel, "plans/") {
		return nil
	}
	if pf.Status != "accepted" && pf.Status != "in-progress" {
		return nil
	}
	covers := map[string]bool{}
	for _, s := range pf.Scenarios {
		if !wellFormedScenario(s) {
			continue
		}
		for _, id := range scenarioCovers(s) {
			covers[id] = true
		}
	}
	seen := map[string]bool{}
	var out []report.Finding
	for _, entry := range pf.Contract.Stable {
		if isBoolTrue(entry["external"]) {
			continue
		}
		if !behaviorContractKind(entry) {
			continue
		}
		idStr, _ := entry["id"].(string)
		if idStr == "" || covers[idStr] || seen[idStr] {
			continue
		}
		seen[idStr] = true
		out = append(out, report.Finding{
			RuleID:   scenarioRuleID,
			Severity: report.SeverityAdvisory,
			Grade:    report.GradeAdvisory,
			File:     rel,
			Line:     1,
			Message:  "CONTRACT_NO_SCENARIO: behavior contract " + idStr + " has no covering scenario",
		})
	}
	return out
}

// wellFormedScenario requires the declared example to be complete: a
// covers-only entry proves nothing and never counts as coverage.
func wellFormedScenario(entry map[string]any) bool {
	for _, key := range []string{"given", "when", "expect"} {
		v, _ := entry[key].(string)
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}
