package rules

import (
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

func TestRemediateFillsHintsFromCatalog(t *testing.T) {
	findings := []report.Finding{
		{RuleID: "PYLON014", Message: "missing wrong_if"},
		{RuleID: "PYLON999", Message: "unknown rule"},
		{RuleID: "PYLON015", Message: "stale", Remediation: "already set"},
	}
	Remediate(findings)
	if findings[0].Remediation != Catalog["PYLON014"].FixHint {
		t.Fatalf("Remediation = %q, want catalog hint %q", findings[0].Remediation, Catalog["PYLON014"].FixHint)
	}
	if findings[1].Remediation != "" {
		t.Fatalf("unknown rule must keep empty remediation, got %q", findings[1].Remediation)
	}
	if findings[2].Remediation != "already set" {
		t.Fatalf("existing remediation must not be overwritten, got %q", findings[2].Remediation)
	}
}
