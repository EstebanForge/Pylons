// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
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

// GitHub caps the annotation title at 255 characters and the message at
// 64 KiB; overflow is truncated rather than rejected wholesale.
const (
	annotationTitleCap = 255
	annotationMsgCap   = 65536
)

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
// fail a check. Findings without a usable path count in the totals and
// the summary but never annotate, because GitHub rejects a whole batch
// over one invalid entry.
func Annotations(input []byte) (AnnotationReport, error) {
	if err := validateJSON(input); err != nil {
		return AnnotationReport{}, err
	}
	out := AnnotationReport{Annotations: []CheckAnnotation{}}
	findings, err := decodeFindings(input)
	if err != nil {
		return AnnotationReport{}, err
	}
	Remediate(findings)
	// Per-rule counts and a representative message cover every finding,
	// including the ones the annotation cap drops.
	counts := map[string]int{}
	reps := map[string]string{}
	for i := range findings {
		f := &findings[i]
		f.RuleID = report.CleanText(f.RuleID)
		f.File = report.CleanText(f.File)
		f.Message = report.CleanText(f.Message)
		f.Remediation = report.CleanText(f.Remediation)
		if f.Severity != report.SeverityError {
			continue
		}
		out.Total++
		counts[f.RuleID]++
		if _, ok := reps[f.RuleID]; !ok {
			reps[f.RuleID] = f.Message
		}
		if len(out.Annotations) >= annotationCap {
			out.Truncated = true
			continue
		}
		// One rejected path drops the whole GitHub batch: an empty,
		// absolute, or parent-relative path counts in the totals but
		// never annotates.
		if !annotationPathSafe(f.File) {
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
			Message:         capRunes(msg, annotationMsgCap),
			Title:           capRunes(f.RuleID, annotationTitleCap),
		})
	}
	out.Summary = summaryBody(out, counts, reps)
	return out, nil
}

// decodeFindings parses the input as SARIF when a runs key is present
// (an empty runs array stays a valid clean run) and as the findings
// array otherwise, failing closed on anything else.
func decodeFindings(input []byte) ([]report.Finding, error) {
	var probe map[string]json.RawMessage
	if json.Unmarshal(input, &probe) == nil {
		if _, isSarif := probe["runs"]; isSarif {
			var sarif sarifInput
			if err := json.Unmarshal(input, &sarif); err != nil {
				return nil, fmt.Errorf("input carries a runs key but is not valid SARIF: %w", err)
			}
			var findings []report.Finding
			for _, run := range sarif.Runs {
				for _, r := range run.Results {
					if r.Level != "error" {
						continue
					}
					var file string
					var line int
					if len(r.Locations) > 0 {
						file = strings.TrimPrefix(r.Locations[0].PhysicalLocation.ArtifactLocation.URI, "file://")
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
			return findings, nil
		}
	}
	var findings []report.Finding
	if err := json.Unmarshal(input, &findings); err != nil {
		return nil, fmt.Errorf("input is neither SARIF nor a findings array: %w", err)
	}
	return findings, nil
}

// validateJSON rejects input that is not a JSON document (a stray human
// log or a bare null can never read as a clean run).
func validateJSON(input []byte) error {
	var v any
	if err := json.Unmarshal(input, &v); err != nil {
		return fmt.Errorf("input is not JSON: %w", err)
	}
	if v == nil {
		return errors.New("input is not a JSON document")
	}
	return nil
}

// capRunes truncates to the cap with an ellipsis, by rune so multi-byte
// text cannot land mid-sequence.
// annotationPathSafe reports whether a finding path is usable as a
// GitHub annotation path: non-empty, relative, and free of parent
// hops.
func annotationPathSafe(p string) bool {
	if p == "" {
		return false
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func capRunes(s string, cap int) string {
	if len([]rune(s)) <= cap {
		return s
	}
	runes := []rune(s)
	return string(runes[:cap-1]) + "\u2026"
}

// summaryBody renders the single updating comment: one line per rule
// with a representative message. The hidden marker lets a CI workflow
// find and update the comment instead of appending a new one.
func summaryBody(r AnnotationReport, counts map[string]int, reps map[string]string) string {
	var b strings.Builder
	b.WriteString("<!-- pylons:review -->\n### Pylons review\n")
	if r.Total == 0 {
		b.WriteString("0 blocking findings.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d blocking finding(s)", r.Total)
	if r.Truncated {
		fmt.Fprintf(&b, " (%d annotated; the GitHub cap is %d)", len(r.Annotations), annotationCap)
	}
	b.WriteString(".\n\n")
	for _, rule := range slices.Sorted(maps.Keys(counts)) {
		fmt.Fprintf(&b, "- `%s` x%d: %s\n", rule, counts[rule], summaryLine(reps[rule]))
	}
	return b.String()
}

// summaryLine makes one representative message safe for markdown:
// backticks cannot break out of the code span and @ cannot ping anyone
// from a token-bearing CI comment.
func summaryLine(s string) string {
	s = report.CleanText(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "`", "'")
	return strings.ReplaceAll(s, "@", "&#64;")
}
