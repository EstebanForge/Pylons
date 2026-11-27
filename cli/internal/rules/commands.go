// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

const stalenessMaxDays = 90

// Closed token sets for the two post-mortem signal fields; exact
// comparison, no normalization, per the vocabulary convention.
var aiAuthoredTokens = map[string]bool{"yes": true, "no": true, "unknown": true}

var volumePressureTokens = map[string]bool{
	"review": true, "test": true, "rollout": true, "observability": true, "none": true,
}

var updatedLineRe = regexp.MustCompile(`(?m)^(updated:[ \t]*)(?:"[^"\n]+"|'[^'\n]+'|[^\s#\n]+)(.*)$`)

// CheckPlanContract validates all contract entries, falsifiers, and test refs in a plan document.
func CheckPlanContract(kbRoot, planArg string, allowExternal bool) ([]report.Finding, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return nil, err
	}
	abs := planArg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(kbRoot, planArg)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(kbRoot, abs)
	if err != nil {
		rel = planArg
	}
	rel = filepath.ToSlash(rel)
	content := string(data)
	var pf resolve.PlanFrontMatter
	if !resolve.ParsePlanFrontMatter(content, &pf) || pf.Type != "plan" {
		return nil, fmt.Errorf("%s is not a valid plan document", rel)
	}
	findings := checkContracts(kbRoot, rel, content, cfg, allowExternal)
	return findings, nil
}

// CheckStaleness checks package docs (or all packages) for outdated updated: dates.
// When fix is true, stale updated: dates are bumped to today.
func CheckStaleness(kbRoot, pkgFilter string, fix bool) []report.Finding {
	pkgDir := filepath.Join(kbRoot, "packages")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	var findings []report.Finding
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" {
			continue
		}
		p := filepath.Join(pkgDir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(raw)
		var fm resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(content, &fm) {
			continue
		}
		if pkgFilter != "" && fm.Package != pkgFilter && strings.TrimSuffix(e.Name(), ".md") != pkgFilter {
			continue
		}
		rawDate := strings.Trim(strings.TrimSpace(fm.Updated), `"'`)
		updated, err := time.Parse("2006-01-02", rawDate)
		if err != nil {
			continue
		}
		daysOld := int(time.Since(updated) / (24 * time.Hour))
		if daysOld > stalenessMaxDays {
			if fix {
				patched := patchUpdatedFrontMatter(content, today)
				if patched != content {
					if err := os.WriteFile(p, []byte(patched), 0o644); err == nil {
						fmt.Fprintf(os.Stderr, "staleness --fix: updated %s (%s -> %s)\n", filepath.ToSlash(filepath.Join("packages", e.Name())), rawDate, today)
					}
				}
				continue
			}
			findings = append(findings, report.Finding{
				RuleID:   "PYLON015",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     filepath.ToSlash(filepath.Join("packages", e.Name())),
				Line:     1,
				Message:  fmt.Sprintf("package doc updated %s (%d days old, > %d)", rawDate, daysOld, stalenessMaxDays),
			})
		}
	}
	return findings
}

// patchUpdatedFrontMatter rewrites updated: only inside frontmatter, so
// example blocks in body text are never corrupted by staleness --fix.
// patchUpdatedFrontMatter bumps the updated: line inside the leading
// frontmatter block only, on raw content (CRLF included) so the
// normalize-then-replace round-trip cannot silently no-op.
func patchUpdatedFrontMatter(content, today string) string {
	return patchFrontmatter(content, func(block string) string {
		return updatedLineRe.ReplaceAllString(block, "${1}"+today+"${2}")
	})
}

// hasHeading matches an exact trimmed heading line, not a substring.
func hasHeading(content, heading string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == heading {
			return true
		}
	}
	return false
}

// ReviewPlan validates a plan or proposal document's structure and STE-100 compliance.
// PlanReviewPath reports whether rel sits in a zone the commit gate
// reviews with the plan structure rules. One predicate serves both the
// gate and the lint pass, so neither can drift from the other.
func PlanReviewPath(rel string) bool {
	if !strings.HasSuffix(rel, ".md") {
		return false
	}
	base := filepath.Base(rel)
	if base == "README.md" || base == "AGENTS.md" {
		return false
	}
	return strings.HasPrefix(rel, "proposals/") || strings.HasPrefix(rel, "plans/proposals/")
}

// QuirkReviewPath reports whether rel sits in the zone the commit gate
// reviews with the postmortem rules.
func QuirkReviewPath(rel string) bool {
	if !strings.HasSuffix(rel, ".md") {
		return false
	}
	base := filepath.Base(rel)
	if base == "README.md" || base == "AGENTS.md" {
		return false
	}
	return strings.HasPrefix(rel, "quirks/")
}

// checkPlanStructure emits PYLON020 and PYLON021 for one plan document:
// valid plan frontmatter and the three required sections.
func checkPlanStructure(rel, content string) []report.Finding {
	var findings []report.Finding
	var fm resolve.PlanFrontMatter
	if !resolve.ParsePlanFrontMatter(content, &fm) || strings.TrimSpace(fm.Title) == "" || fm.Type != "plan" {
		findings = append(findings, errFinding("PYLON020", rel, 1, "plan lacks valid frontmatter (title, type: plan, status)"))
	}
	for _, sec := range []string{"## Problem", "## Design", "## Milestones"} {
		if !strings.Contains(content, sec) {
			findings = append(findings, errFinding("PYLON021", rel, 1, fmt.Sprintf("plan missing required section %q", sec)))
		}
	}
	return findings
}

func ReviewPlan(kbRoot, planArg string) []report.Finding {
	abs := planArg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(kbRoot, planArg)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return []report.Finding{errFinding("PYLON001", planArg, 1, "cannot read plan: "+err.Error())}
	}
	rel := planArg
	if r, err := filepath.Rel(kbRoot, abs); err == nil {
		rel = filepath.ToSlash(r)
	}
	content := string(raw)
	findings := checkPlanStructure(rel, content)
	findings = append(findings, checkSTE100(rel, resolve.WithoutFrontMatter(content))...)
	return findings
}

// ReviewPostmortem validates quirks/*.md structure, guard:, and optionally sources: paths.
func ReviewPostmortem(kbRoot, quirkArg string, checkSources bool) []report.Finding {
	cfg, cfgErr := config.Load(kbRoot)
	var findings []report.Finding
	if cfgErr != nil {
		findings = append(findings, errFinding("PYLON001", "manifests/pylons.yaml", 1, "invalid manifests/pylons.yaml: "+cfgErr.Error()))
	}
	var paths []string
	if quirkArg != "" {
		abs := quirkArg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(kbRoot, quirkArg)
		}
		paths = []string{abs}
	} else {
		qDir := filepath.Join(kbRoot, "quirks")
		entries, err := os.ReadDir(qDir)
		if err != nil {
			return findings
		}
		var junk []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" {
				continue
			}
			if strings.HasPrefix(e.Name(), "._") {
				junk = append(junk, e.Name())
				continue
			}
			paths = append(paths, filepath.Join(qDir, e.Name()))
		}
		covered := IgnoredByGit(kbRoot, junk)
		for _, name := range junk {
			if !covered[name] {
				paths = append(paths, filepath.Join(qDir, name))
			}
		}
	}

	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			findings = append(findings, errFinding("PYLON022", p, 1, "cannot read quirk: "+err.Error()))
			continue
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		findings = append(findings, postmortemFindings(kbRoot, rel, string(raw), cfg, checkSources)...)
	}
	return findings
}

// postmortemFindings checks one quirk document's required sections and,
// when checkSources is set, its sources: paths against registered projects.
func postmortemFindings(kbRoot, rel, content string, cfg config.Config, checkSources bool) []report.Finding {
	var findings []report.Finding
	var fm resolve.DocMeta
	_ = resolve.ParseDocMeta(content, &fm)
	for _, sec := range []string{"## Symptom", "## Cause", "## Resolution"} {
		if !hasHeading(content, sec) {
			findings = append(findings, errFinding("PYLON022", rel, 1, fmt.Sprintf("postmortem missing required section %q", sec)))
		}
	}
	if token := strings.TrimSpace(fm.AIAuthoredContributed); token != "" && !aiAuthoredTokens[token] {
		findings = append(findings, errFinding("PYLON041", rel, 1, "ai_authored_contributed must be one of yes|no|unknown, got "+token))
	}
	if token := strings.TrimSpace(fm.VolumePressure); token != "" && !volumePressureTokens[token] {
		findings = append(findings, errFinding("PYLON041", rel, 1, "volume_pressure must be one of review|test|rollout|observability|none, got "+token))
	}
	if checkSources {
		for _, src := range fm.Sources {
			if !resolveSourceExists(kbRoot, src, fm.Package, cfg) {
				findings = append(findings, errFinding("PYLON025", rel, 1, "postmortem source not found: "+src))
			}
		}
	}
	return findings
}

func resolveSourceExists(kbRoot, src, pkg string, cfg config.Config) bool {
	if strings.Contains(src, "://") {
		u, ok := resolve.ParseURI(src)
		if !ok || u.Scheme != "repo" {
			return false
		}
		projRoot := cfg.ProjectPath(kbRoot, u.Alias)
		if projRoot == "" {
			return false
		}
		return resolve.StatProjectFile(projRoot, cfg.ProjectRef(u.Alias), u.Path)
	}
	if pkg != "" {
		if projRoot := cfg.ProjectPath(kbRoot, pkg); projRoot != "" {
			return resolve.StatProjectFile(projRoot, cfg.ProjectRef(pkg), strings.TrimPrefix(src, "/"))
		}
	}
	target, ok := resolve.SafeJoin(kbRoot, strings.TrimPrefix(src, "/"))
	if !ok {
		return false
	}
	_, err := os.Stat(target)
	return err == nil
}

// PackageManifestEntry summarizes one packages/*.md doc for manifests/packages.json.
type PackageManifestEntry struct {
	Path      string   `json:"path"`
	Package   string   `json:"package"`
	Status    string   `json:"status"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// BuildPackagesManifest walks packages/ and returns sorted package entries.
func BuildPackagesManifest(kbRoot string) []PackageManifestEntry {
	kbRoot = resolve.EvalRoot(kbRoot)
	pkgDir := filepath.Join(kbRoot, "packages")
	var out []PackageManifestEntry
	_ = filepath.WalkDir(pkgDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "archive" || strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		if d.Name() == "README.md" || d.Name() == "AGENTS.md" {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var fm resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(string(raw), &fm) || fm.Type != "package" {
			return nil
		}
		rel, _ := filepath.Rel(kbRoot, p)
		out = append(out, PackageManifestEntry{
			Path:      filepath.ToSlash(rel),
			Package:   fm.Package,
			Status:    fm.Status,
			DependsOn: fm.DependsOn,
		})
		return nil
	})
	slices.SortFunc(out, func(a, b PackageManifestEntry) int { return strings.Compare(a.Path, b.Path) })
	if out == nil {
		out = []PackageManifestEntry{}
	}
	return out
}
