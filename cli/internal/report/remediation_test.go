package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRemediationRenderedInAllFormats(t *testing.T) {
	findings := []Finding{
		{RuleID: "PYLON014", Severity: SeverityError, Grade: GradeForced, File: "plans/a.md", Line: 4, Message: "missing wrong_if", Remediation: "Add wrong_if: describing what invalidates the contract."},
		{RuleID: "PYLON015", Severity: SeverityAdvisory, Grade: GradeAdvisory, File: "packages/x.md", Message: "stale doc"},
	}

	var buf bytes.Buffer
	if err := Render(&buf, FormatHuman, findings); err != nil {
		t.Fatal(err)
	}
	human := buf.String()
	if !strings.Contains(human, "fix: Add wrong_if: describing what invalidates the contract.") {
		t.Fatalf("human output missing remediation line: %s", human)
	}
	if strings.Count(human, "fix:") != 1 {
		t.Fatalf("finding without remediation must not render a fix line: %s", human)
	}

	buf.Reset()
	if err := Render(&buf, FormatJSON, findings); err != nil {
		t.Fatal(err)
	}
	var decoded []Finding
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0].Remediation != findings[0].Remediation {
		t.Fatalf("json remediation = %q, want %q", decoded[0].Remediation, findings[0].Remediation)
	}
	if decoded[1].Remediation != "" {
		t.Fatalf("json remediation should be omitted when empty, got %q", decoded[1].Remediation)
	}

	buf.Reset()
	if err := Render(&buf, FormatSARIF, findings); err != nil {
		t.Fatal(err)
	}
	sarif := buf.String()
	if !strings.Contains(sarif, `"remediation": "Add wrong_if: describing what invalidates the contract."`) {
		t.Fatalf("sarif output missing remediation property: %s", sarif)
	}
}
