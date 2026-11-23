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

func TestAnnotationsEmptyRunsIsClean(t *testing.T) {
	r, err := Annotations([]byte(`{"runs": []}`))
	if err != nil {
		t.Fatalf("a valid SARIF with empty runs is a clean run, got %v", err)
	}
	if r.Total != 0 || len(r.Annotations) != 0 {
		t.Fatalf("empty runs must be an empty report, got %+v", r)
	}
}

func TestAnnotationsNullInputFailsClosed(t *testing.T) {
	if _, err := Annotations([]byte(`null`)); err == nil {
		t.Fatal("a JSON null must fail closed, never read as a clean run")
	}
}

func TestAnnotationsMissingLocationStaysOutOfAnnotations(t *testing.T) {
	// GitHub rejects a whole annotation batch over one empty path, so
	// a result without a location counts in the totals but never
	// annotates.
	sarif := `{"runs": [{"results": [
		{"ruleId": "PYLON001", "level": "error", "message": {"text": "no location"}},
		{"ruleId": "PYLON001", "level": "error", "message": {"text": "located"},
		 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "packages/a.md"}, "region": {"startLine": 2}}}]}
	]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 2 || len(r.Annotations) != 1 {
		t.Fatalf("the locationless result must stay out of annotations, got %+v", r)
	}
	if !strings.Contains(r.Summary, "x2") {
		t.Fatalf("the summary must count all findings of the rule, got %q", r.Summary)
	}
}

func TestAnnotationsStripsFileScheme(t *testing.T) {
	// A file:// URI strips to an absolute path, which is exactly the
	// unsafe class: it counts in the totals but never annotates.
	sarif := `{"runs": [{"results": [{"ruleId": "PYLON001", "level": "error", "message": {"text": "m"},
		"locations": [{"physicalLocation": {"artifactLocation": {"uri": "file:///srv/kb/packages/a.md"}, "region": {"startLine": 1}}}]}]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 1 || len(r.Annotations) != 0 {
		t.Fatalf("an absolute stripped path must never annotate, got %+v", r)
	}
}

func TestAnnotationsSanitizesHostileText(t *testing.T) {
	sarif := `{"runs": [{"results": [{"ruleId": "PYLON001", "level": "error", "message": {"text": "\u001b[31mred ` + "`bad`" + ` ping @everyone"},
		"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	a := r.Annotations[0]
	if strings.ContainsRune(a.Message, 0x1b) {
		t.Fatalf("control characters must not reach the annotation, got %q", a.Message)
	}
	if strings.Contains(r.Summary, "\x1b") || strings.Contains(r.Summary, "@everyone") || strings.Contains(r.Summary, "`bad`") {
		t.Fatalf("the summary must not carry raw backticks, mentions, or escapes, got %q", r.Summary)
	}
}

func TestAnnotationsTitleCapped(t *testing.T) {
	sarif := `{"runs": [{"results": [{"ruleId": "` + strings.Repeat("R", 300) + `", "level": "error", "message": {"text": "m"},
		"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(r.Annotations[0].Title)); got > 255 {
		t.Fatalf("the title must fit the GitHub cap, got %d runes", got)
	}
}

func TestAnnotationsSummaryCountsTruncatedFindings(t *testing.T) {
	var results []string
	for i := 0; i < 30; i++ {
		results = append(results, `{"ruleId": "PYLONAAA", "level": "error", "message": {"text": "a"},
			"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}`)
	}
	for i := 0; i < 22; i++ {
		results = append(results, `{"ruleId": "PYLONBBB", "level": "error", "message": {"text": "b"},
			"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}`)
	}
	sarif := `{"runs": [{"results": [` + strings.Join(results, ",") + `]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "`PYLONAAA` x30") || !strings.Contains(r.Summary, "`PYLONBBB` x22") {
		t.Fatalf("per-rule counts must cover every finding, not only the annotated ones, got %q", r.Summary)
	}
}

func TestAnnotationsSARIFCarriesFixHint(t *testing.T) {
	sarif := `{"runs": [{"results": [{"ruleId": "PYLON001", "level": "error", "message": {"text": "m"},
		"locations": [{"physicalLocation": {"artifactLocation": {"uri": "f.md"}, "region": {"startLine": 1}}}]}]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Annotations[0].Message, "Fix:") {
		t.Fatalf("the SARIF path must carry the catalog fix hint like the JSON path, got %q", r.Annotations[0].Message)
	}
}

func TestAnnotationsUnsafePathsNeverAnnotate(t *testing.T) {
	// One rejected path drops the whole GitHub batch, so absolute and
	// parent-relative paths count in totals but never annotate.
	sarif := `{"runs": [{"results": [
		{"ruleId": "PYLON001", "level": "error", "message": {"text": "parent"},
		 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "../../etc/passwd"}, "region": {"startLine": 1}}}]},
		{"ruleId": "PYLON001", "level": "error", "message": {"text": "absolute"},
		 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "/abs/x.md"}, "region": {"startLine": 1}}}]},
		{"ruleId": "PYLON001", "level": "error", "message": {"text": "clean"},
		 "locations": [{"physicalLocation": {"artifactLocation": {"uri": "packages/a.md"}, "region": {"startLine": 2}}}]}
	]}]}`
	r, err := Annotations([]byte(sarif))
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 3 || len(r.Annotations) != 1 {
		t.Fatalf("unsafe paths must count but never annotate, got %+v", r)
	}
	if r.Annotations[0].Path != "packages/a.md" {
		t.Fatalf("only the clean path annotates, got %+v", r.Annotations[0])
	}
}

// A rule id arrives from parsed input; a backtick or newline inside it
// must not break the markdown list open.
func TestAnnotationsSummaryEscapesRuleId(t *testing.T) {
	evil := "Ev`il\n- injected"
	sarif, merr := json.Marshal(map[string]any{
		"runs": []any{map[string]any{
			"tool": map[string]any{"driver": map[string]any{"rules": []any{
				map[string]any{"id": evil},
			}}},
			"results": []any{map[string]any{
				"ruleId": evil, "level": "error",
				"locations": []any{map[string]any{"physicalLocation": map[string]any{
					"artifactLocation": map[string]any{"uri": "a.go"},
					"region":           map[string]any{"startLine": 1},
				}}},
			}},
		}},
	})
	if merr != nil {
		t.Fatal(merr)
	}
	rep, err := Annotations(sarif)
	if err != nil {
		t.Fatalf("Annotations: %v", err)
	}
	if strings.Contains(rep.Summary, "Ev`") || strings.Contains(rep.Summary, "injected") {
		t.Fatalf("the rule id must be sanitized in the summary: %q", rep.Summary)
	}
	if !strings.Contains(rep.Summary, "x1") {
		t.Fatalf("the finding must still be counted: %q", rep.Summary)
	}
}
