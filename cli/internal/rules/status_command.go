// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

var statusLineRe = regexp.MustCompile(`(?m)^(status:\s*)("[^"\n]+"|'[^'\n]+'|[^\s#\n]+)(.*)$`)

// AllowedStatuses returns the closed status set for a zone.
func AllowedStatuses(zone string) ([]string, bool) {
	set, ok := allowedStatuses[zone]
	return set, ok
}

// statusArchives reports whether the status files a doc into the zone
// archive; the mirror of statusMatchesFolder's archive branches.
func statusArchives(rootZone, status string) bool {
	switch rootZone {
	case "plans":
		return status == "shipped" || status == "abandoned"
	case "decisions":
		return status == "superseded" || status == "deprecated"
	case "conventions":
		return status == "superseded"
	case "investigations":
		return slices.Contains([]string{"resolved", "abandoned", "superseded", "withdrawn", "retired"}, status)
	case "packages":
		return status == "retired"
	case "tickets":
		return status == "done" || status == "wontfix"
	default:
		return false
	}
}

// StatusTargetPath returns the ledger-relative path a doc with the
// given status should live at; an already-matching path is returned
// unchanged, archive statuses flatten to <zone>/archive/<base>, and
// promotions flatten to <zone>/<base>.
func StatusTargetPath(rootZone, rel, status string) string {
	if statusMatchesFolder(rootZone, rel, status) {
		return rel
	}
	base := filepath.Base(rel)
	if statusArchives(rootZone, status) {
		return filepath.ToSlash(filepath.Join(rootZone, "archive", base))
	}
	return filepath.ToSlash(filepath.Join(rootZone, base))
}

// statusTransitionBlockers names the rules whose closing obligations
// the doc fails for the requested status; empty means the transition
// may proceed.
func statusTransitionBlockers(rootZone, rel string, meta resolve.DocMeta, content, newStatus, decisionPrefix string) []report.Finding {
	var out []report.Finding
	switch {
	case rootZone == "plans" && (newStatus == "shipped" || newStatus == "abandoned") && !meta.Artifact.NonEmpty():
		out = append(out, errFinding("PYLON009", rel, 1, "shipped plan must declare artifact:"))
	case rootZone == "plans" && newStatus == "paused" && !hasHeading(content, "## State"):
		out = append(out, errFinding("PYLON065", rel, 1, "paused plan must include ## State naming the resume point"))
	case rootZone == "investigations" && (newStatus == "resolved" || newStatus == "abandoned") &&
		(!hasHeading(content, "## Root Cause") || !hasHeading(content, "## Resolution")):
		out = append(out, errFinding("PYLON031", rel, 1, "closed investigation must include ## Root Cause and ## Resolution"))
	case rootZone == "decisions" && newStatus == "accepted":
		nameRe := regexp.MustCompile(`^` + decisionPrefix + `\d{4}-.+\.md$`)
		if !nameRe.MatchString(filepath.Base(rel)) {
			out = append(out, errFinding("PYLON010", rel, 1, "accepted decision must be named "+decisionPrefix+"NNNN-slug.md"))
		}
	}
	return out
}

// ApplyStatus moves a doc to a new status: it validates the status
// against the zone's closed set, refuses fail-closed on unmet closing
// obligations or an existing move target, patches status and updated
// in the frontmatter, and renames the file when the status places it
// elsewhere. Returned findings are blocking; the caller renders them.
func ApplyStatus(kbRoot, docArg, newStatus, decisionPrefix string) (string, []report.Finding, error) {
	today := time.Now().Format("2006-01-02")
	abs := docArg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(kbRoot, docArg)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(kbRoot, abs)
	if err != nil {
		return "", nil, err
	}
	rel = filepath.ToSlash(rel)
	rootZone, _, _ := strings.Cut(rel, "/")
	allowed, known := allowedStatuses[rootZone]
	if !known {
		return "", nil, fmt.Errorf("%s is not a status-tracked zone document", rel)
	}
	if !slices.Contains(allowed, newStatus) {
		return "", nil, fmt.Errorf("status %s is not valid for %s/ (valid: %s)", newStatus, rootZone, strings.Join(allowed, ", "))
	}
	content := string(data)
	var meta resolve.DocMeta
	if !resolve.ParseDocMeta(content, &meta) {
		return "", nil, fmt.Errorf("%s has no parseable frontmatter", rel)
	}
	if blockers := statusTransitionBlockers(rootZone, rel, meta, content, newStatus, decisionPrefix); len(blockers) > 0 {
		return rel, blockers, nil
	}
	dstRel := StatusTargetPath(rootZone, rel, newStatus)
	dst := filepath.Join(kbRoot, filepath.FromSlash(dstRel))
	if dstRel != rel {
		if _, err := os.Stat(dst); err == nil {
			return rel, []report.Finding{errFinding("PYLON011", rel, 1, "move target already exists: "+dstRel)}, nil
		}
	}
	patched := patchStatusFrontMatter(content, newStatus, today)
	if patched != content {
		if err := os.WriteFile(abs, []byte(patched), 0o644); err != nil {
			return "", nil, err
		}
	}
	if dstRel != rel {
		if err := os.Rename(abs, dst); err != nil {
			return "", nil, err
		}
	}
	return dstRel, nil, nil
}

// patchStatusFrontMatter rewrites the status and updated lines in
// place, leaving the rest of the document byte-identical.
func patchStatusFrontMatter(content, status, today string) string {
	front := resolve.ExtractFrontMatter(content)
	if front == "" {
		return content
	}
	patchedFront := statusLineRe.ReplaceAllString(front, "${1}"+status+"${3}")
	patchedFront = updatedLineRe.ReplaceAllString(patchedFront, "${1}"+today+"${2}")
	if patchedFront == front {
		return content
	}
	return strings.Replace(content, front, patchedFront, 1)
}
