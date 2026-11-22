// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

type normalizedTarget struct {
	alias string
	path  string
}

// Check intersects target paths against active plans and package protected globs.
func Check(kbRoot string, targets []string) []report.Finding {
	cfg, cfgErr := config.Load(kbRoot)
	var findings []report.Finding
	if cfgErr != nil {
		findings = append(findings, errFinding("PYLON001", "manifests/pylons.yaml", 1, "invalid manifests/pylons.yaml: "+cfgErr.Error()))
	}
	normTargets := make([]normalizedTarget, 0, len(targets))
	for _, t := range targets {
		normTargets = append(normTargets, normalizeCheckTarget(kbRoot, t, cfg))
	}

	_ = filepath.WalkDir(kbRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isRootSkippedDir(kbRoot, p) || strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/archive/") || strings.HasPrefix(rel, "templates/") {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		content := string(data)

		var pf resolve.PlanFrontMatter
		pfParsed := resolve.ParsePlanFrontMatter(content, &pf) && pf.Type == "plan"
		if pfParsed {
			if pf.Status != "accepted" && pf.Status != "in-progress" {
				pfParsed = false
			}
		} else if strings.HasPrefix(content, "---\n") {
			// A plan doc whose plan frontmatter cannot parse must
			// fail loudly: silently dropping it would skip the
			// collision walk and ripple for this document.
			var probe resolve.DocMeta
			if resolve.ParseDocMeta(content, &probe) && probe.Type == "plan" {
				findings = append(findings, errFinding("PYLON001", rel, 1, "plan frontmatter cannot parse; collisions and ripple are skipped for this document"))
			}
		}
		if pfParsed {
			for _, nt := range normTargets {
				for _, raw := range pf.Contract.Editing {
					if gs, ok := extractGlobString(raw); ok && globMatchesTarget(gs, nt, pf.Components) {
						findings = append(findings, report.Finding{
							RuleID:   "PYLON-CHECK-EDITING",
							Severity: report.SeverityError,
							Grade:    report.GradeForced,
							File:     rel,
							Message:  "target " + nt.path + " collides with active contract.editing glob " + gs,
						})
					}
				}
				for _, raw := range pf.Contract.Unstable {
					if gs, ok := extractGlobString(raw); ok && globMatchesTarget(gs, nt, pf.Components) {
						findings = append(findings, report.Finding{
							RuleID:   "PYLON-CHECK-UNSTABLE",
							Severity: report.SeverityError,
							Grade:    report.GradeForced,
							File:     rel,
							Message:  "target " + nt.path + " touches unstable glob " + gs,
						})
					}
				}
			}
		}

		var pkg resolve.PackageFrontMatter
		if resolve.ParsePackageFrontMatter(content, &pkg) && pkg.Type == "package" {
			var comps []string
			if pkg.Package != "" {
				comps = []string{pkg.Package}
			}
			for _, raw := range pkg.Protected {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				glob, _ := m["glob"].(string)
				owner, _ := m["owner"].(string)
				reason, _ := m["reason"].(string)
				if glob == "" || owner == "" {
					continue
				}
				for _, nt := range normTargets {
					if globMatchesTarget(glob, nt, comps) {
						sev := report.SeverityError
						grade := report.GradeForced
						if owner == "ask-first" {
							sev = report.SeverityAdvisory
							grade = report.GradeAdvisory
						}
						findings = append(findings, report.Finding{
							RuleID:   "PYLON029",
							Severity: sev,
							Grade:    grade,
							File:     rel,
							Message:  "target " + nt.path + " is protected (" + owner + "): " + reason,
						})
					}
				}
			}
		}
		return nil
	})
	findings = append(findings, rippleFindings(kbRoot, cfg, normTargets)...)
	return findings
}

func normalizeCheckTarget(kbRoot, raw string, cfg config.Config) normalizedTarget {
	if u, ok := resolve.ParseURI(raw); ok && u.Scheme == "repo" {
		return normalizedTarget{alias: u.Alias, path: filepath.ToSlash(filepath.Clean(u.Path))}
	}
	// A relative KB root must still match absolute targets: make the
	// root absolute before comparing paths. Plain relative targets
	// stay ledger-relative by design; only repo:// and absolute
	// targets carry project attribution.
	if !filepath.IsAbs(kbRoot) {
		if abs, err := filepath.Abs(kbRoot); err == nil {
			kbRoot = abs
		}
	}
	if filepath.IsAbs(raw) {
		absTarget := filepath.Clean(raw)
		type match struct {
			alias string
			rel   string
		}
		var matches []match
		for _, alias := range slices.Sorted(maps.Keys(cfg.Projects)) {
			projPath := cfg.ProjectPath(kbRoot, alias)
			if projPath == "" {
				continue
			}
			if rel, err := filepath.Rel(projPath, absTarget); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				matches = append(matches, match{alias: alias, rel: filepath.ToSlash(rel)})
			}
		}
		// Deepest root wins: the shortest rel is the most specific project;
		// alias breaks ties so nested-checkout attribution never flips run to run.
		if len(matches) > 0 {
			best := matches[0]
			for _, m := range matches[1:] {
				if len(m.rel) < len(best.rel) || (len(m.rel) == len(best.rel) && m.alias < best.alias) {
					best = m
				}
			}
			return normalizedTarget{alias: best.alias, path: best.rel}
		}
	}
	clean := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(raw, "/")))
	return normalizedTarget{path: clean}
}

func extractGlobString(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	case map[string]any:
		id, _ := v["id"].(string)
		if strings.TrimSpace(id) == "" {
			return "", false
		}
		return id, true
	default:
		return "", false
	}
}

func globMatchesTarget(pattern string, nt normalizedTarget, components []string) bool {
	if u, ok := resolve.ParseURI(pattern); ok && u.Scheme == "repo" {
		if nt.alias != "" && nt.alias != u.Alias {
			return false
		}
		return resolve.MatchGlob(u.Path, nt.path)
	}
	if nt.alias != "" && len(components) > 0 {
		matchedComp := false
		for _, c := range components {
			if c == nt.alias {
				matchedComp = true
				break
			}
		}
		if !matchedComp {
			return false
		}
	}
	return resolve.MatchGlob(pattern, nt.path)
}
