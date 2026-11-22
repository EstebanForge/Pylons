// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// mergeDangerRuleID is PYLON047: one merge danger line in the pull
// request body, validated against the closed door vocabulary and the
// minimum door derived from the diff.
const mergeDangerRuleID = "PYLON047"

// mergeDangerLine matches the required body line shape; the door
// vocabulary is closed and validated against mergeDangerDoors.
var mergeDangerLine = regexp.MustCompile(`^Merge danger:\s*(\S+)\s*\|\s*blast radius:\s*(\S.*)$`)

// mergeDangerDoors is the closed door vocabulary.
var mergeDangerDoors = map[string]bool{"one-way": true, "two-way": true}

// MergeDangerOptions carries the gate inputs: the pull request body
// file (or "-" for stdin) and the diff base ref.
type MergeDangerOptions struct {
	Body string
	Base string
}

// MergeDanger parses the pull request body, validates the merge
// danger line, and derives the minimum door from the diff. Blocking
// findings are error severity; the command exits nonzero on them.
func MergeDanger(kbRoot string, opts MergeDangerOptions) ([]report.Finding, error) {
	if opts.Body == "" {
		return nil, fmt.Errorf("no pull request body: pass --body <file>")
	}
	raw, err := readMergeBody(opts.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var findings []report.Finding
	fail := func(msg string) {
		findings = append(findings, report.Finding{
			RuleID:   mergeDangerRuleID,
			Severity: report.SeverityError,
			Grade:    report.GradeForced,
			File:     opts.Body,
			Line:     1,
			Message:  msg,
		})
	}

	door, blast, mergeLine, count := parseMergeDangerLine(raw)
	switch {
	case count > 1:
		fail("MERGE_DANGER_DUPLICATE: exactly one merge danger line is required in the body")
	case count == 0:
		fail("MERGE_DANGER_MISSING: the body has no 'Merge danger: <door> | blast radius: <scope>' line")
	case door == "":
		fail("MERGE_DANGER_INVALID: the merge danger line must read 'Merge danger: <one-way|two-way> | blast radius: <scope in one phrase>'")
	default:
		findings = append(findings, validateOneWayConsistency(kbRoot, opts, door, blast, raw, mergeLine)...)
	}
	findings = append(findings, deriveMergeDanger(kbRoot, opts, door, fail)...)
	return findings, nil
}

// readMergeBody reads the body file; "-" reads stdin.
func readMergeBody(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		return string(data), err
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

// parseMergeDangerLine returns (door, blast, lineNumber, lineCount).
func parseMergeDangerLine(body string) (string, string, int, int) {
	count := 0
	door, blast := "", ""
	lineNo := -1
	for i, line := range strings.Split(body, "\n") {
		m := mergeDangerLine.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m == nil {
			continue
		}
		count++
		door, blast, lineNo = m[1], strings.TrimSpace(m[2]), i+1
		if !mergeDangerDoors[door] {
			door = ""
		}
	}
	return door, blast, lineNo, count
}

// validateOneWayConsistency enforces the pairing rule: an author
// one-way door must pair with a release marker or a Data migration
// note.
func validateOneWayConsistency(kbRoot string, opts MergeDangerOptions, door, blast, body string, mergeLine int) []report.Finding {
	if door != "one-way" || blast == "" {
		return nil
	}
	if dataMigrationRe.MatchString(body) {
		return nil
	}
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return []report.Finding{notEvaluated(opts.Body)}
	}
	surfaces := schemaContractSurfaces(kbRoot)
	if len(surfaces) == 0 {
		// Nothing declared is at risk, so the one-way claim has no
		// code side to check.
		return nil
	}
	marker, evaluated := false, false
	for _, alias := range slices.Sorted(maps.Keys(surfaces)) {
		projPath := cfg.ProjectPath(kbRoot, alias)
		if projPath == "" {
			continue
		}
		found, err := projectHasReleaseMarker(projPath, opts.Base)
		if err != nil {
			continue
		}
		evaluated = true
		if found {
			marker = true
			break
		}
	}
	if !evaluated {
		// An unreadable base must never read as a verified claim.
		return []report.Finding{notEvaluated(opts.Body)}
	}
	if marker {
		return nil
	}
	return []report.Finding{{
		RuleID:   mergeDangerRuleID,
		Severity: report.SeverityError,
		Grade:    report.GradeForced,
		File:     opts.Body,
		Line:     mergeLine,
		Message:  "MERGE_DANGER_UNPAIRED: a one-way door needs a release marker (breaking commit subject or #major token) in the diff or a 'Data migration:' note at a line start in the body",
	}}
}

// deriveMergeDanger computes the minimum door from the diffs and
// blocks a downgrade. A derived one-way may never be de-escalated by
// the author line. Each project with schema surfaces diffs its own
// repository, so the KB may live outside the code it judges.
func deriveMergeDanger(kbRoot string, opts MergeDangerOptions, door string, fail func(string)) []report.Finding {
	if door == "" {
		// MISSING, INVALID, and DUPLICATE already block; a derived
		// verdict would speak for a door nobody wrote.
		return nil
	}
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return []report.Finding{notEvaluated(opts.Body)}
	}
	surfaces := schemaContractSurfaces(kbRoot)
	if len(surfaces) == 0 {
		return nil
	}
	evaluated, derived := false, false
	for _, alias := range slices.Sorted(maps.Keys(surfaces)) {
		projPath := cfg.ProjectPath(kbRoot, alias)
		if projPath == "" {
			continue
		}
		paths, err := projectDiffPaths(projPath, opts.Base)
		if err != nil {
			continue
		}
		evaluated = true
		if anyPatternMatches(surfaces[alias], paths) {
			derived = true
			break
		}
	}
	if !evaluated {
		// Fail open loudly: the line validation stands, but the
		// derived minimum is unjudged.
		return []report.Finding{notEvaluated(opts.Body)}
	}
	if !derived {
		return nil
	}
	if door == "two-way" {
		fail("MERGE_DANGER_DOWNGRADE: the diff touches a schema contract surface; the minimum door is one-way and the author two-way line cannot de-escalate it")
		return nil
	}
	return []report.Finding{{
		RuleID:   mergeDangerRuleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     opts.Body,
		Line:     1,
		Message:  "MERGE_DANGER_DERIVED: the diff touches a schema contract surface; the one-way door is consistent with the derived minimum",
	}}
}

// notEvaluated builds the advisory for a diff the gate could not read.
func notEvaluated(body string) report.Finding {
	return report.Finding{
		RuleID:   mergeDangerRuleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     body,
		Line:     1,
		Message:  "MERGE_DANGER_NOT_EVALUATED: no project diff could be evaluated; the derived minimum door is unjudged (shallow clone, missing base, or unresolvable ref)",
	}
}

// projectDiffPaths lists the paths changed between the base ref and
// HEAD in the project's repository, relative to the project root. The
// KB may live inside or outside the code repo.
func projectDiffPaths(projectPath, base string) ([]string, error) {
	if base == "" {
		base = "origin/main"
	}
	top, err := resolve.GitRun(projectPath, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	toplevel := strings.TrimSpace(string(top))
	prefix, err := filepath.Rel(toplevel, projectPath)
	if err != nil {
		return nil, err
	}
	prefix = filepath.ToSlash(prefix)
	out, err := resolve.GitRun(projectPath, nil, "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if prefix == "." || prefix == "" {
			paths = append(paths, filepath.ToSlash(line))
			continue
		}
		if rest, ok := strings.CutPrefix(filepath.ToSlash(line), prefix+"/"); ok {
			paths = append(paths, rest)
		}
	}
	return paths, nil
}

// releaseMarkerRe matches a breaking change subject: feat!:,
// feat(scope)!:, fix!:, refactor!:, and any other breaking type.
var releaseMarkerRe = regexp.MustCompile(`^[a-z]+(\([^)]*\))?!:`)

// majorRe matches the #major token without matching #majority.
var majorRe = regexp.MustCompile(`#major\b`)

// dataMigrationRe matches a Data migration: note at a line start; a
// quoted mention in the body proves nothing.
var dataMigrationRe = regexp.MustCompile(`(?m)^Data migration:\s*\S`)

// projectHasReleaseMarker reports whether the commits in the code
// project's diff carry a release marker: a breaking type subject or a
// #major token. The proof reads the project's own repository; a
// ledger outside the code never speaks for its commits.
func projectHasReleaseMarker(projectPath, base string) (bool, error) {
	if base == "" {
		base = "origin/main"
	}
	out, err := resolve.GitRun(projectPath, nil, "log", "--pretty=format:%s", base+"..HEAD")
	if err != nil {
		return false, err
	}
	for _, subject := range strings.Split(string(out), "\n") {
		if releaseMarkerRe.MatchString(subject) || majorRe.MatchString(subject) {
			return true, nil
		}
	}
	return false, nil
}

// schemaContractSurfaces collects the schema-kind contract surfaces of
// accepted and in-progress plans, keyed by the project alias of the
// repo:// URI.
func schemaContractSurfaces(kbRoot string) map[string][]string {
	out := map[string][]string{}
	seen := map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(kbRoot, "plans"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := d.Name()
		if name == "README.md" || name == "AGENTS.md" {
			return nil
		}
		data, err := readManifest(path)
		if err != nil {
			return nil
		}
		var pf resolve.PlanFrontMatter
		if !resolve.ParsePlanFrontMatter(string(data), &pf) || pf.Type != "plan" {
			return nil
		}
		if pf.Status != "accepted" && pf.Status != "in-progress" {
			return nil
		}
		for _, entry := range pf.Contract.Stable {
			kindStr, _ := entry["kind"].(string)
			kind := kindStr
			if i := strings.LastIndex(kindStr, "-"); i >= 0 {
				kind = kindStr[i+1:]
			}
			if kind != "schema" {
				continue
			}
			idStr, _ := entry["id"].(string)
			u, ok := resolve.ParseURI(idStr)
			if !ok || u.Scheme != "repo" || u.Path == "" {
				continue
			}
			key := u.Alias + ":" + uriSurfacePath(u.Path)
			if seen[key] {
				continue
			}
			seen[key] = true
			out[u.Alias] = append(out[u.Alias], uriSurfacePath(u.Path))
		}
		return nil
	})
	return out
}

// anyPatternMatches reports whether a diff path matches any pattern by
// gitignore-style glob, exact path, or directory prefix.
func anyPatternMatches(patterns, paths []string) bool {
	for _, p := range paths {
		if slices.Contains(patterns, p) {
			return true
		}
		for _, g := range patterns {
			// ParseURI rejects trailing slashes, so surfaces are
			// exact files or globs; the glob branch covers both.
			if strings.ContainsAny(g, "*?[") && resolve.MatchGlob(g, p) {
				return true
			}
		}
	}
	return false
}
