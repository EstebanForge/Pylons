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

// [ \t]* not \s*: a \s class would cross the newline and swallow the
// next key as the value. The value itself is consumed non-capturing,
// so an empty value line never matches and a missing status line is
// indistinguishable from an empty one: both refuse.
var statusLineRe = regexp.MustCompile(`(?m)^(status:[ \t]*)(?:"[^"\n]+"|'[^'\n]+'|[^\s#\n]+)(.*)$`)

// frontBlockRe matches the leading frontmatter block on raw content,
// CRLF and BOM included, ending at the first exact --- fence line.
var frontBlockRe = regexp.MustCompile(`\A(?:\x{FEFF})?---(?:\r?\n)(?s:.*?\n)---(?:\r?\n|\z)`)

// patchFrontmatter applies edit to the leading frontmatter block and
// returns content unchanged when there is no frontmatter.
func patchFrontmatter(content string, edit func(string) string) string {
	return frontBlockRe.ReplaceAllStringFunc(content, func(block string) string {
		return edit(block)
	})
}

// AllowedStatuses returns the closed status set for a zone.
func AllowedStatuses(zone string) ([]string, bool) {
	set, ok := allowedStatuses[zone]
	return set, ok
}

// StatusTargetPath returns the ledger-relative path a doc with the
// given status should live at, derived from statusMatchesFolder so the
// two cannot disagree. An already-matching path is returned unchanged;
// otherwise the first candidate folder that satisfies the predicate
// wins, flattening the doc to its basename like the PYLON007/PYLON011
// convention.
func StatusTargetPath(rootZone, rel, status string) string {
	if statusMatchesFolder(rootZone, rel, status) {
		return rel
	}
	base := filepath.Base(rel)
	for _, folder := range []string{"proposals", "archive", "."} {
		candidate := filepath.ToSlash(filepath.Join(rootZone, folder, base))
		if candidate != rel && statusMatchesFolder(rootZone, candidate, status) {
			return candidate
		}
	}
	return rel
}

// statusTransitionBlockers names the rules whose closing obligations
// the doc fails for the requested status; empty means the transition
// may proceed. The predicates mirror the lint checks exactly (heading
// match is substring-based, shipped-only artifact, audits skip
// PYLON031) so the command refuses exactly when the linter would flag
// the resulting state.
func statusTransitionBlockers(rootZone, rel string, meta resolve.DocMeta, content, newStatus, decisionPrefix string) []report.Finding {
	var out []report.Finding
	switch {
	case rootZone == "plans" && newStatus == "shipped" && !meta.Artifact.NonEmpty():
		out = append(out, errFinding("PYLON009", rel, 1, "shipped plan must declare artifact:"))
	case rootZone == "plans" && newStatus == "paused" && !strings.Contains(content, "## State"):
		out = append(out, errFinding("PYLON065", rel, 1, "paused plan must include ## State naming the resume point"))
	case rootZone == "investigations" && meta.Type != "audit" && (newStatus == "resolved" || newStatus == "abandoned") &&
		(!strings.Contains(content, "## Root Cause") || !strings.Contains(content, "## Resolution")):
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
	rel, err := filepath.Rel(kbRoot, abs)
	if err != nil {
		return "", nil, err
	}
	rel = filepath.ToSlash(rel)
	if base := filepath.Base(rel); base == "README.md" || base == "AGENTS.md" {
		return "", nil, fmt.Errorf("%s is a zone entry guide, not a lifecycle document", rel)
	}
	if fi, err := os.Lstat(abs); err != nil {
		return "", nil, err
	} else if fi.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("%s is a symlink; status moves would desync the link target", rel)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", nil, err
	}
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
	if !frontBlockRe.MatchString(content) || !statusLineRe.MatchString(content) {
		return "", nil, fmt.Errorf("%s has no status: line in its frontmatter", rel)
	}
	if blockers := statusTransitionBlockers(rootZone, rel, meta, content, newStatus, decisionPrefix); len(blockers) > 0 {
		return rel, blockers, nil
	}
	patched := patchFrontmatter(content, func(block string) string {
		block = statusLineRe.ReplaceAllString(block, "${1}"+newStatus+"${2}")
		return updatedLineRe.ReplaceAllString(block, "${1}"+today+"${2}")
	})
	var verify resolve.DocMeta
	if !resolve.ParseDocMeta(patched, &verify) || verify.Status != newStatus {
		return "", nil, fmt.Errorf("%s: patched frontmatter does not parse with status %s; nothing written", rel, newStatus)
	}
	dstRel := StatusTargetPath(rootZone, rel, newStatus)
	dst := filepath.Join(kbRoot, filepath.FromSlash(dstRel))
	if dstRel != rel {
		if _, err := os.Stat(dst); err == nil {
			return rel, []report.Finding{errFinding("PYLON011", rel, 1, "move target already exists: "+dstRel)}, nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", nil, err
		}
	}
	if err := os.WriteFile(abs, []byte(patched), 0o644); err != nil {
		return "", nil, err
	}
	if dstRel != rel {
		if err := os.Rename(abs, dst); err != nil {
			return "", nil, err
		}
	}
	return dstRel, nil, nil
}
