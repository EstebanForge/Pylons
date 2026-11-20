// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

var (
	invFilenameRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-[a-z0-9-]+\.md$`)
	planFilenameRe = regexp.MustCompile(`^(?:feature|feat|refactor|chore|docs|perf)-[a-z0-9-]+\.md$`)
	auditRowStatus = map[string]bool{"open": true, "fixed": true, "rejected": true, "moved": true}
	reauditDateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

func parseYAMLFrontMatter(content string, dst any) bool {
	front := resolve.ExtractFrontMatter(content)
	if front == "" {
		return false
	}
	return yaml.Unmarshal([]byte(front), dst) == nil
}

func scanLifecycleRules(kbRoot string, docs []corpusDoc, cfg config.Config, hasCode bool) []report.Finding {
	var out []report.Finding
	for _, d := range docs {
		rootZone, _, _ := strings.Cut(d.rel, "/")
		base := filepath.Base(d.rel)
		if base == "README.md" || base == "AGENTS.md" {
			continue
		}

		switch rootZone {
		case "plans":
			if !strings.Contains(d.rel, "/archive/") {
				if !planFilenameRe.MatchString(base) {
					out = append(out, errFinding("PYLON038", d.rel, 1, "plan filename must start with feature-|refactor-|chore-|docs-|perf-"))
				}
				if d.meta.Status == "shipped" || d.meta.Status == "abandoned" {
					dst := filepath.Join(kbRoot, "plans", "archive", base)
					if _, err := os.Stat(dst); err == nil {
						out = append(out, errFinding("PYLON011", d.rel, 1, "archive move target already exists: plans/archive/"+base))
					}
				}
			}
		case "proposals":
			if !planFilenameRe.MatchString(base) {
				out = append(out, errFinding("PYLON038", d.rel, 1, "proposal filename must start with feature-|refactor-|chore-|docs-|perf-"))
			}
			out = append(out, ReviewPlan(kbRoot, d.rel)...)
		case "investigations":
			if !invFilenameRe.MatchString(base) {
				out = append(out, errFinding("PYLON030", d.rel, 1, "investigation filename must match YYYY-MM-DD-<slug>.md"))
			}
			if d.meta.Status == "resolved" || d.meta.Status == "abandoned" {
				if !strings.Contains(d.content, "## Root Cause") || !strings.Contains(d.content, "## Resolution") {
					out = append(out, errFinding("PYLON031", d.rel, 1, "closed investigation must include ## Root Cause and ## Resolution"))
				}
			}
		case "audits":
			out = append(out, checkAuditDoc(d)...)
		case "quirks":
			out = append(out, postmortemFindings(kbRoot, d.rel, d.content, cfg, hasCode)...)
		case "domain":
			if !strings.Contains(d.rel, "/archive/") && d.meta.Type == "domain-entity" && !avoidListed(d.meta.Avoid) {
				out = append(out, advFinding("PYLON039", d.rel, "domain entity has no avoid: anti-terms; misuse has no name"))
			}
		}
	}
	out = append(out, CheckStaleness(kbRoot, "", false)...)
	return out
}

// avoidListed reports whether the avoid list carries any non-blank entry.
func avoidListed(avoid resolve.StringOrList) bool {
	for _, v := range avoid {
		if strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func checkAuditDoc(d corpusDoc) []report.Finding {
	var out []report.Finding
	if d.meta.Status == "active" {
		raw := strings.TrimSpace(d.meta.Reaudit)
		if raw != "" {
			// String compare keeps the due day itself unflagged regardless
			// of local time zone; time.Parse-based checks flagged it early.
			switch {
			case !reauditDateRe.MatchString(raw):
				out = append(out, advFinding("PYLON026", d.rel, "unparsable reaudit date "+raw+" (want YYYY-MM-DD)"))
			case raw < time.Now().Format("2006-01-02"):
				out = append(out, advFinding("PYLON026", d.rel, "active audit reaudit date "+raw+" is overdue"))
			}
		}
	}
	for idx, line := range strings.Split(d.content, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "|") || strings.Contains(trim, "---") {
			continue
		}
		cells := strings.Split(strings.Trim(trim, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		idCell := strings.TrimSpace(cells[0])
		if idCell == "ID" || idCell == "" {
			continue
		}
		statusCell := strings.ToLower(strings.TrimSpace(cells[len(cells)-1]))
		if !auditRowStatus[statusCell] {
			out = append(out, errFinding("PYLON018", d.rel, idx+1, "audit finding row has invalid status: "+statusCell))
		}
	}
	return out
}
