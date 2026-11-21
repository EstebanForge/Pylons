// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// CheckAnnotation mirrors one GitHub Check Run annotation. The
// annotations API caps a single call at 50 entries.
type CheckAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"`
	Message         string `json:"message"`
	Title           string `json:"title"`
}

// AnnotationReport is the CI payload: the annotation array for the
// Check Run API plus a summary body for the single updating PR comment.
// The engine emits the payload; the workflow posts it.
type AnnotationReport struct {
	Annotations []CheckAnnotation `json:"annotations"`
	Summary     string            `json:"summary"`
	Total       int               `json:"total"`
	Truncated   bool              `json:"truncated"`
}

// annotationCap is the GitHub Check Run annotations limit.
const annotationCap = 50

// sarifInput decodes the SARIF subset Annotations needs.
type sarifInput struct {
	Runs []struct {
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

// Annotations converts pylon lint output (SARIF or findings JSON) into
// GitHub Check annotations plus a summary comment body. Blocking errors
// only: advisories never annotate, so a clean-but-advisory run cannot
// fail a check.
func Annotations(input []byte) (AnnotationReport, error) {
	if err := validateJSON(input); err != nil {
		return AnnotationReport{}, err
	}
	out := AnnotationReport{Annotations: []CheckAnnotation{}}
	var findings []report.Finding
	var sarif sarifInput
	if json.Unmarshal(input, &sarif) == nil && len(sarif.Runs) > 0 {
		for _, run := range sarif.Runs {
			for _, r := range run.Results {
				if r.Level != "error" {
					continue
				}
				var file string
				var line int
				if len(r.Locations) > 0 {
					file = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
					line = r.Locations[0].PhysicalLocation.Region.StartLine
				}
				findings = append(findings, report.Finding{
					RuleID:   r.RuleID,
					Severity: report.SeverityError,
					File:     file,
					Line:     line,
					Message:  r.Message.Text,
				})
			}
		}
	} else {
		if err := json.Unmarshal(input, &findings); err != nil {
			return AnnotationReport{}, fmt.Errorf("input is neither SARIF nor a findings array: %w", err)
		}
	}
	for _, f := range findings {
		if f.Severity != report.SeverityError {
			continue
		}
		out.Total++
		if len(out.Annotations) >= annotationCap {
			out.Truncated = true
			continue
		}
		msg := f.Message
		if f.Remediation != "" {
			msg += "\n\nFix: " + f.Remediation
		}
		line := f.Line
		if line < 1 {
			line = 1
		}
		out.Annotations = append(out.Annotations, CheckAnnotation{
			Path:            f.File,
			StartLine:       line,
			EndLine:         line,
			AnnotationLevel: "failure",
			Message:         msg,
			Title:           f.RuleID,
		})
	}
	out.Summary = summaryBody(out)
	return out, nil
}

// validateJSON rejects input that is not a JSON document, so a stray
// human log can never read as a clean run.
func validateJSON(input []byte) error {
	var v any
	if err := json.Unmarshal(input, &v); err != nil {
		return fmt.Errorf("input is not JSON: %w", err)
	}
	return nil
}

// summaryBody renders the single updating comment: one line per rule
// with a representative message, newest severities first.
func summaryBody(r AnnotationReport) string {
	var b strings.Builder
	b.WriteString("### Pylons review\n")
	if r.Total == 0 {
		b.WriteString("0 blocking findings.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d blocking finding(s)", r.Total)
	if r.Truncated {
		fmt.Fprintf(&b, " (%d annotated; the GitHub cap is %d)", len(r.Annotations), annotationCap)
	}
	b.WriteString(".\n\n")
	byRule := map[string][]CheckAnnotation{}
	for _, a := range r.Annotations {
		byRule[a.Title] = append(byRule[a.Title], a)
	}
	for _, rule := range slices.Sorted(maps.Keys(byRule)) {
		anns := byRule[rule]
		fmt.Fprintf(&b, "- `%s` x%d: %s\n", rule, len(anns), firstLine(anns[0].Message))
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
