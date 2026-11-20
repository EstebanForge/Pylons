// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

const reviewBriefFile = ".pylon-review-brief.md"

var reviewLenses = []string{"contract", "security", "logic", "complexity", "decision"}

func runReviewBrief(kbRoot, planArg, base string, stdout bool, validatePath, validateResPath string, format report.Format) int {
	if validatePath != "" && validateResPath != "" {
		return rbError(format, "cannot pass both --validate and --validate-resolutions", 2)
	}
	if validatePath != "" || validateResPath != "" {
		target := validatePath
		requireRes := false
		if validateResPath != "" {
			target = validateResPath
			requireRes = true
		}
		return validateReviewBrief(target, requireRes, format)
	}

	if planArg == "" {
		return rbError(format, "review-brief requires <plan>", 2)
	}
	if base == "" {
		base = "main"
	}
	absPlan := planArg
	if !filepath.IsAbs(absPlan) {
		absPlan = filepath.Join(kbRoot, planArg)
	}
	raw, err := os.ReadFile(absPlan)
	if err != nil {
		return rbError(format, "plan not found: "+planArg, 2)
	}
	var fm resolve.PlanFrontMatter
	if !resolve.ParsePlanFrontMatter(string(raw), &fm) {
		return rbError(format, "invalid plan frontmatter: "+planArg, 2)
	}
	if fm.Status != "accepted" && fm.Status != "in-progress" {
		return rbError(format, fmt.Sprintf("plan %s status %s is not reviewable (requires accepted or in-progress)", planArg, fm.Status), 2)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Review Brief: %s\n\n", fm.Title)
	fmt.Fprintf(&b, "- Plan: %s\n", planArg)
	fmt.Fprintf(&b, "- Status: %s\n", fm.Status)
	fmt.Fprintf(&b, "- Base: %s\n\n", base)
	b.WriteString("## Coverage Grid\n\n| Lens | Status | Notes |\n|---|---|---|\n")
	for _, lens := range reviewLenses {
		fmt.Fprintf(&b, "| %s | pending | |\n", lens)
	}
	b.WriteString("\n## Contract Surfaces\n\n")
	for _, c := range fm.Contract.Stable {
		kind, _ := c["kind"].(string)
		id, _ := c["id"].(string)
		fmt.Fprintf(&b, "- `stable` `%s` `%s`\n", kind, id)
	}
	for _, u := range fm.Contract.Unstable {
		fmt.Fprintf(&b, "- `unstable` `%s`\n", formatContractGlob(u))
	}
	for _, e := range fm.Contract.Editing {
		fmt.Fprintf(&b, "- `editing` `%s`\n", formatContractGlob(e))
	}
	b.WriteString("\n## Findings\n\n")

	if stdout {
		fmt.Print(b.String())
		return 0
	}
	outPath := filepath.Join(kbRoot, reviewBriefFile)
	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		return rbError(format, fmt.Sprintf("failed to write %s: %v", outPath, err), 1)
	}
	if format == report.FormatJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"output": outPath})
	} else {
		fmt.Printf("wrote %s\n", outPath)
	}
	return 0
}

func formatContractGlob(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		id, _ := v["id"].(string)
		if kind, _ := v["kind"].(string); kind != "" && id != "" {
			return kind + ":" + id
		}
		return id
	default:
		return fmt.Sprintf("%v", raw)
	}
}

func rbError(format report.Format, msg string, code int) int {
	if format == report.FormatJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"valid": false, "error": msg})
	} else {
		fmt.Fprintf(os.Stderr, "pylon: %s\n", msg)
	}
	return code
}

func validateReviewBrief(path string, requireResolutions bool, format report.Format) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rbError(format, fmt.Sprintf("cannot read review brief %s: %v", path, err), 1)
	}
	content := string(raw)
	var problems []string
	for _, sec := range []string{"## Coverage Grid", "## Findings"} {
		if !strings.Contains(content, sec) {
			problems = append(problems, "missing section: "+sec)
		}
	}
	for _, lens := range reviewLenses {
		if !strings.Contains(content, "| "+lens+" |") {
			problems = append(problems, "missing coverage lens row: "+lens)
		} else if requireResolutions && strings.Contains(content, "| "+lens+" | pending |") {
			// --validate-resolutions rejects an untouched template row.
			problems = append(problems, "coverage lens still pending: "+lens)
		}
	}
	if requireResolutions {
		findings := strings.Count(content, "### F")
		resolutions := strings.Count(content, "resolution:")
		if findings > resolutions {
			problems = append(problems, fmt.Sprintf("findings without resolution: %d of %d", findings-resolutions, findings))
		}
	}

	if len(problems) > 0 {
		if format == report.FormatJSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"valid": false, "problems": problems})
		} else {
			for _, p := range problems {
				fmt.Fprintln(os.Stderr, "INVALID:", p)
			}
		}
		return 1
	}
	if format == report.FormatJSON {
		fmt.Println(`{"valid":true}`)
	} else {
		fmt.Println("review-brief valid")
	}
	return 0
}
