// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import "github.com/estebanforge/pylons/cli/internal/report"

// Remediate fills each finding's Remediation from the rule catalog fix
// hint, so one render answers "how do I fix this" without an explain
// lookup. Explicit remediations are never overwritten.
func Remediate(findings []report.Finding) {
	for i := range findings {
		f := &findings[i]
		if f.Remediation != "" {
			continue
		}
		if spec, ok := Catalog[f.RuleID]; ok {
			f.Remediation = spec.FixHint
		}
	}
}
