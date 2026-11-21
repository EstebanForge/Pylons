// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

func TestAnnotationsFromSARIFKeepsErrorsOnly(t *testing.T) {
	sarif := `{
		"runs": [{"results": [
			{"ruleId": "PYLON001", "level": "error", "message": {"text": "broken frontmatter"},
			 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "packages/a.md"}, "region": {"startLine": 3}}}]},
			{"ruleId": "PYLON039", "level": "warning", "message": {"text": "advisory"},
			 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "domain/x.md"}}}]}
		]}]
	}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 1 || len(r.Annotations) != 1 {
		t.Fatalf("only errors become annotations, got %+v", r)
	}
	a := r.Annotations[0]
	if a.Path != "packages/a.md" || a.StartLine != 3 || a.EndLine != 3 || a.AnnotationLevel != "failure" || a.Title != "PYLON001" {
		t.Fatalf("annotation must mirror the SARIF result, got %+v", a)
	}
	if !strings.Contains(a.Message, "broken frontmatter") {
		t.Fatalf("annotation must carry the finding message, got %q", a.Message)
	}
}

func TestAnnotationsFromFindingsJSONKeepsErrorsOnly(t *testing.T) {
	findings := []report.Finding{
		{RuleID: "PYLON001", Severity: report.SeverityError, File: "plans/p.md", Line: 7, Message: "forced", Remediation: "repair frontmatter"},
		{RuleID: "PYLON039", Severity: report.SeverityAdvisory, File: "domain/x.md", Message: "advisory"},
	}
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Annotations(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 1 || len(r.Annotations) != 1 {
		t.Fatalf("only errors become annotations, got %+v", r)
	}
	if !strings.Contains(r.Annotations[0].Message, "repair frontmatter") {
		t.Fatalf("the remediation hint must ride the annotation, got %q", r.Annotations[0].Message)
	}
}

func TestAnnotationsCleanInput(t *testing.T) {
	r, err := Annotations([]byte(`{"runs": [{"results": []}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 0 || len(r.Annotations) != 0 || r.Truncated {
		t.Fatalf("clean input must be an empty report, got %+v", r)
	}
	if !strings.Contains(r.Summary, "0") {
		t.Fatalf("the summary must say zero, got %q", r.Summary)
	}
}

func TestAnnotationsUnparsableFailsClosed(t *testing.T) {
	if _, err := Annotations([]byte("not json")); err == nil {
		t.Fatal("unparsable input must fail closed, never read as clean")
	}
}

func TestAnnotationsCapAtFifty(t *testing.T) {
	var results []string
	for i := 0; i < 52; i++ {
		results = append(results, `{"ruleId": "PYLON001", "level": "error", "message": {"text": "n"},
			"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}`)
	}
	sarif := `{"runs": [{"results": [` + strings.Join(results, ",") + `]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 52 || len(r.Annotations) != 50 || !r.Truncated {
		t.Fatalf("the GitHub cap must truncate with the total kept, got total=%d ann=%d trunc=%v", r.Total, len(r.Annotations), r.Truncated)
	}
	if !strings.Contains(r.Summary, "50") || !strings.Contains(r.Summary, "52") {
		t.Fatalf("the summary must report the truncation, got %q", r.Summary)
	}
}
