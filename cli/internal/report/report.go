// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
	FormatSARIF Format = "sarif"
)

const (
	SeverityError    = "error"
	SeverityAdvisory = "advisory"
)

const (
	GradeForced   = "FORCED"
	GradeAdvisory = "ADVISORY"
)

// Finding is a single deterministic check or lint result.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Grade    string `json:"grade,omitempty"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// ValidateFormat reports whether format is one of human, json, or sarif.
func ValidateFormat(format Format) bool {
	return format == FormatHuman || format == FormatJSON || format == FormatSARIF
}

// HasBlocking reports whether any finding has error severity.
func HasBlocking(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Render writes findings in human, json, or sarif format.
func Render(w io.Writer, format Format, findings []Finding) error {
	if findings == nil {
		findings = []Finding{}
	}
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(findings)
	case FormatSARIF:
		return renderSARIF(w, findings)
	default:
		for _, f := range findings {
			f.RuleID, f.Severity, f.Grade, f.File, f.Message = cleanText(f.RuleID), cleanText(f.Severity), cleanText(f.Grade), cleanText(f.File), cleanText(f.Message)
			grade := f.Grade
			if grade == "" {
				if f.Severity == SeverityError {
					grade = GradeForced
				} else {
					grade = GradeAdvisory
				}
			}
			if f.Line > 0 {
				if _, err := fmt.Fprintf(w, "%s [%s] %s:%d: %s\n", grade, f.RuleID, f.File, f.Line, f.Message); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s [%s] %s: %s\n", grade, f.RuleID, f.File, f.Message); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// cleanText strips control characters (including ANSI escapes) so hostile
// finding text cannot repaint the terminal.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		if r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// RenderStopBlocks renders FORCED findings as structured agent stop blocks.
func RenderStopBlocks(w io.Writer, findings []Finding) error {
	for _, f := range findings {
		f.RuleID, f.File, f.Message = cleanText(f.RuleID), cleanText(f.File), cleanText(f.Message)
		if f.Severity == SeverityError {
			if _, err := fmt.Fprintf(w, "=== PYLON STOP BLOCK [%s] ===\nFile: %s\nReason: %s\nAction: Stop and resolve before editing.\n=== END STOP BLOCK ===\n", f.RuleID, f.File, f.Message); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "ADVISORY [%s] %s: %s\n", f.RuleID, f.File, f.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderSARIF(w io.Writer, findings []Finding) error {
	results := make([]map[string]any, 0, len(findings))
	for _, f := range findings {
		level := "warning"
		if f.Severity == SeverityError {
			level = "error"
		}
		loc := map[string]any{
			"physicalLocation": map[string]any{
				"artifactLocation": map[string]any{"uri": f.File},
			},
		}
		if f.Line > 0 {
			loc["physicalLocation"].(map[string]any)["region"] = map[string]any{"startLine": f.Line}
		}
		results = append(results, map[string]any{
			"ruleId":    f.RuleID,
			"level":     level,
			"message":   map[string]any{"text": f.Message},
			"locations": []any{loc},
		})
	}
	doc := map[string]any{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": []any{
			map[string]any{
				"tool": map[string]any{
					"driver": map[string]any{
						"name": "pylon",
					},
				},
				"results": results,
			},
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
