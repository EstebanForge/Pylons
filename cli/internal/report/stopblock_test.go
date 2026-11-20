// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"strings"
	"testing"
)

func TestStopBlocksCarryRemediation(t *testing.T) {
	var b strings.Builder
	err := RenderStopBlocks(&b, []Finding{
		{RuleID: "PYLON042", Severity: SeverityError, File: "plans/x.md", Line: 1,
			Message: "m", Remediation: "Fix the new advisories."},
		{RuleID: "PYLON039", Severity: SeverityAdvisory, File: "domain/e.md", Line: 1,
			Message: "m2", Remediation: "Add avoid: entries."},
		{RuleID: "PYLON050", Severity: SeverityAdvisory, File: "quirks/q.md", Line: 1,
			Message: "m3"},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Fix: Fix the new advisories.") {
		t.Fatalf("error stop block must render the fix line: %s", out)
	}
	if !strings.Contains(out, "; fix: Add avoid: entries.") {
		t.Fatalf("advisory line must carry the fix hint: %s", out)
	}
	if strings.Contains(out, "m3; fix") {
		t.Fatalf("finding without remediation must render no fix text: %s", out)
	}
}
