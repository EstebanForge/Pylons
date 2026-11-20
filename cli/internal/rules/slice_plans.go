// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// sliceRuleID is PYLON045: a slice plan needs both sides of the
// vertical cut, a behavior surface and the test that proves it.
const sliceRuleID = "PYLON045"

// behaviorContractKind reports whether a contract entry names a
// behavior surface: symbol, route, cli, event, schema, and their
// language-prefixed or extension variants.
func behaviorContractKind(entry map[string]any) bool {
	return provableContractKind(entry)
}

// testContractEntry reports whether a contract entry declares a test
// surface: a file or glob contract whose path carries the test
// conventions (a _test/.test/.spec basename, or a test/tests/spec
// directory segment). Substrings like "latest" or "contest" never
// count.
func testContractEntry(entry map[string]any) bool {
	kindStr, _ := entry["kind"].(string)
	base := kindStr
	if i := strings.LastIndex(kindStr, "-"); i >= 0 {
		base = kindStr[i+1:]
	}
	if base != "file" && base != "glob" {
		return false
	}
	idStr, _ := entry["id"].(string)
	low := strings.ToLower(idStr)
	if i := strings.Index(low, "://"); i >= 0 {
		low = low[i+3:]
	}
	if i := strings.Index(low, "::"); i >= 0 {
		low = low[:i]
	}
	segs := strings.Split(low, "/")
	name := segs[len(segs)-1]
	if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_test.ts") ||
		strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
		return true
	}
	for _, seg := range segs[:len(segs)-1] {
		if seg == "test" || seg == "tests" || seg == "spec" || seg == "specs" {
			return true
		}
	}
	return false
}

// checkSlicePlan applies the vertical-slice advisory to one plan. An
// ordinary plan is never affected; the flavor is opt-in, and gate
// statuses only: draft-state plans (proposed, abandoned, shipped) stay
// out of the advisory ratchet's way.
func checkSlicePlan(pf resolve.PlanFrontMatter, rel string) []report.Finding {
	if !pf.Slice {
		return nil
	}
	if !strings.HasPrefix(rel, "plans/") {
		return nil
	}
	if pf.Status != "accepted" && pf.Status != "in-progress" {
		return nil
	}
	behavior, test := false, false
	for _, entry := range pf.Contract.Stable {
		if behaviorContractKind(entry) {
			behavior = true
		}
		if testContractEntry(entry) {
			test = true
		}
	}
	if behavior && test {
		return nil
	}
	missing := "a test surface (file or glob contract naming a test or spec)"
	if !behavior {
		missing = "a behavior contract (symbol, route, cli, event, or schema)"
		if !test {
			missing = "a behavior contract and a test surface"
		}
	}
	return []report.Finding{{
		RuleID:   sliceRuleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     rel,
		Line:     1,
		Message:  "SLICE_INCOMPLETE: the slice plan lacks " + missing,
	}}
}
