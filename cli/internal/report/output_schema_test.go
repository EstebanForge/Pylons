// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleFindings() []Finding {
	return []Finding{
		{RuleID: "PYLON001", Severity: SeverityError, Grade: GradeForced, File: "domain/x.md", Line: 4, Message: "broken frontmatter", Remediation: "fix it"},
		{RuleID: "PYLON039", Severity: SeverityAdvisory, Grade: GradeAdvisory, File: "domain/y.md", Message: "no avoid list"},
	}
}

func TestFindingsJSONShape(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, FormatJSON, sampleFindings()); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
		t.Fatalf("findings JSON must decode as an array: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(out))
	}
	for _, f := range out {
		for _, key := range []string{"rule_id", "severity", "file", "message"} {
			v, ok := f[key].(string)
			if !ok || v == "" {
				t.Fatalf("finding %v misses non-empty %s", f["rule_id"], key)
			}
		}
		if f["grade"] != GradeForced && f["grade"] != GradeAdvisory {
			t.Fatalf("grade must be %s or %s, got %v", GradeForced, GradeAdvisory, f["grade"])
		}
	}
}

func TestFindingsJSONEmptyIsArray(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, FormatJSON, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(b.String()); got != "[]" {
		t.Fatalf("empty findings must render as [], got %s", got)
	}
}

func TestSarifShape(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, FormatSARIF, sampleFindings()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID  string `json:"ruleId"`
				Level   string `json:"level"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(b.String()), &doc); err != nil {
		t.Fatalf("SARIF must decode: %v", err)
	}
	if doc.Version != "2.1.0" {
		t.Fatalf("SARIF version must be 2.1.0, got %s", doc.Version)
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Results) != 2 {
		t.Fatalf("expected 1 run with 2 results, got %+v", doc)
	}
	for _, r := range doc.Runs[0].Results {
		if r.RuleID == "" || r.Message.Text == "" {
			t.Fatalf("SARIF result misses ruleId or message text: %+v", r)
		}
	}
}
