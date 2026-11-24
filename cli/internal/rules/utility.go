// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/sync"
)

// Utility telemetry thresholds. Prune needs repeated surfacing with no
// citation ever coming back; promote needs citations from more than one
// place. Absence of telemetry is never evidence: nothing prunes or
// promotes off an empty state file.
const (
	// utilityHalfLifeDays weights each telemetry event by
	// 0.5**(ageDays/halfLife), so stale history stops counting.
	utilityHalfLifeDays = 30.0
	utilityPruneFloor   = 3.0
	utilityPromoteCites = 3
	utilityPromoteFiles = 2
)

// UtilityStateFile lives in the git directory beside pylon-sync.json:
// machine-local telemetry that never pollutes the worktree and dies
// with the clone.
const UtilityStateFile = "pylon-utility.json"

// utilityMarker matches one citation comment. The value is the
// ledger-relative document path with its .md extension.
var utilityMarker = regexp.MustCompile(`<!--\s*pylon:referenced-doc:\s*(\S+?)\s*-->`)

// UtilityRow is one scored document. Citations count marker instances;
// CitingFiles is the distinct set of documents citing this one.
type UtilityRow struct {
	Path        string
	Injections  int
	Citations   int
	Score       float64
	CitingFiles []string
}

// UtilityReport is the review-utility outcome. The candidate slices
// preview the actions; Pruned and Promoted record what the flags did.
type UtilityReport struct {
	Telemetry         bool
	Scored            []UtilityRow
	PruneCandidates   []string
	PromoteCandidates []UtilityRow
	Pruned            []string
	Promoted          []string
}

// UtilityOptions carries the review-utility inputs. Now is injectable
// so scoring stays deterministic under test.
type UtilityOptions struct {
	KBRoot  string
	Now     time.Time
	Prune   bool
	Promote bool
}

// ReviewUtility scores ledger documents from citation markers in the
// corpus and injection telemetry recorded at context delivery time.
// Report-only by default; --prune archives uncited quirks past the
// weighted injection floor and --promote writes one convention
// proposal stub per repeatedly cited document.
func ReviewUtility(opts UtilityOptions) (UtilityReport, error) {
	kbRoot := resolve.EvalRoot(opts.KBRoot)
	rep := UtilityReport{}

	citations, err := utilityCitations(kbRoot)
	if err != nil {
		return rep, err
	}
	injections, telemetry, err := utilityInjections(kbRoot, opts.Now)
	if err != nil {
		return rep, err
	}
	rep.Telemetry = telemetry

	for _, p := range slices.Sorted(maps.Keys(injections)) {
		if !utilityDocExists(kbRoot, p) {
			continue
		}
		row := UtilityRow{Path: p, CitingFiles: citations[p].files}
		row.Injections = len(injections[p])
		row.Citations = citations[p].count
		row.Score = utilityScore(injections[p], row.Citations, opts.Now)
		rep.Scored = append(rep.Scored, row)
	}
	// Cited documents with no telemetry still surface: citations are
	// evidence on their own. Ghost paths score nothing: a marker
	// naming a deleted document is stale evidence, not a candidate.
	for _, p := range slices.Sorted(maps.Keys(citations)) {
		if _, seen := injections[p]; seen {
			continue
		}
		if !utilityDocExists(kbRoot, p) {
			continue
		}
		rep.Scored = append(rep.Scored, UtilityRow{
			Path:        p,
			Citations:   citations[p].count,
			CitingFiles: citations[p].files,
		})
	}
	slices.SortFunc(rep.Scored, func(a, b UtilityRow) int { return strings.Compare(a.Path, b.Path) })

	for _, row := range rep.Scored {
		if utilityPrunable(kbRoot, row, injections[row.Path], opts.Now) {
			rep.PruneCandidates = append(rep.PruneCandidates, row.Path)
		}
	}
	rep.PromoteCandidates = utilityPromoteCandidates(rep.Scored)

	if opts.Prune {
		for _, p := range rep.PruneCandidates {
			moved, err := utilityArchiveQuirk(kbRoot, p)
			if err != nil {
				return rep, err
			}
			if moved {
				rep.Pruned = append(rep.Pruned, p)
			}
		}
	}
	if opts.Promote {
		today := opts.Now.UTC().Format("2006-01-02")
		for _, row := range rep.PromoteCandidates {
			wrote, err := utilityWriteStub(kbRoot, row, today)
			if err != nil {
				return rep, err
			}
			if wrote {
				rep.Promoted = append(rep.Promoted, row.Path)
			}
		}
	}
	return rep, nil
}

// utilityDocExists reports whether a scored path is a real document.
// Telemetry and citations can outlive their documents; ghosts never
// score, prune, or promote.
func utilityDocExists(kbRoot, path string) bool {
	if !utilityStatePathOK(path) {
		return false
	}
	_, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(path)))
	return err == nil
}

// RecordInjections appends one telemetry record per distinct path for
// this delivery. Best-effort by contract: a ledger outside git, an
// unwritable state, or a malformed state file silently skips
// recording, because context delivery must never fail on telemetry.
// A malformed state is never appended to: the corruption stays exactly
// as found for review utility to report.
func RecordInjections(kbRoot string, paths []string) {
	gitDir, err := sync.GitDirOf(kbRoot)
	if err != nil {
		return
	}
	statePath := filepath.Join(gitDir, UtilityStateFile)
	existing, _ := os.ReadFile(statePath)
	for _, line := range strings.Split(strings.TrimSpace(string(existing)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var probe struct {
			Date string `json:"date"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return
		}
		// A state file holding an invalid record is corrupt: never grow
		// it. Path shape and future dates both count, so a locked file
		// does not keep collecting lines underneath the error.
		if !utilityStatePathOK(probe.Path) {
			return
		}
		if probe.Date > time.Now().UTC().Format("2006-01-02") {
			return
		}
	}
	root := resolve.EvalRoot(kbRoot)
	today := time.Now().UTC().Format("2006-01-02")
	var fresh []string
	added := map[string]bool{}
	for _, p := range paths {
		p = filepath.ToSlash(filepath.Clean(p))
		if !strings.HasSuffix(p, ".md") || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil {
			continue
		}
		rec := fmt.Sprintf("{\"date\":%q,\"path\":%q}", today, p)
		if added[rec] {
			continue
		}
		added[rec] = true
		fresh = append(fresh, rec)
	}
	if len(fresh) == 0 {
		return
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteString("\n")
	}
	for _, rec := range fresh {
		b.WriteString(rec)
		b.WriteString("\n")
	}
	tmp, err := os.CreateTemp(gitDir, "pylon-utility-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, statePath); err != nil {
		os.Remove(name)
	}
}

// utilityCitations scans the corpus for citation markers. The product
// and reference trees are excluded: marker examples in the manual,
// protocols, and templates are documentation, not evidence. Archive
// subtrees count: a citation inside a shipped plan still proves use.
func utilityCitations(kbRoot string) (map[string]utilityCite, error) {
	out := map[string]utilityCite{}
	err := filepath.WalkDir(kbRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if utilitySkipDir(kbRoot, p, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if utilityCiteDocsSkipped[d.Name()] {
			return nil
		}
		for _, m := range utilityMarker.FindAllStringSubmatch(utilityStripFences(string(data)), -1) {
			target := filepath.ToSlash(filepath.Clean(m[1]))
			if !strings.HasSuffix(target, ".md") {
				continue
			}
			c := out[target]
			c.count++
			if !slices.Contains(c.files, rel) {
				c.files = append(c.files, rel)
			}
			out[target] = c
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for t, c := range out {
		slices.Sort(c.files)
		out[t] = c
	}
	return out, nil
}

// utilitySkipDir excludes the trees whose markdown exists to document
// the marker, not to cite documents.
func utilitySkipDir(kbRoot, p, name string) bool {
	if strings.HasPrefix(name, "_") || name == ".git" {
		return true
	}
	if p != filepath.Join(kbRoot, name) {
		return false
	}
	switch name {
	case "templates", "protocols", "pylons", "cli", "bin", "manifests",
		"scripts", "extensions", ".github", "dist":
		return true
	}
	return false
}

// utilityStripFences removes closed fenced code blocks. Fence lines
// are matched per delimiter (backtick vs tilde) and opener length, so
// a tilde line inside a backtick block does not close it. A file left
// inside an open fence strips nothing: losing real citations to a
// malformed document could push a cited quirk into prune, and missing
// evidence is the unsafe direction.
func utilityStripFences(content string) string {
	lines := strings.Split(content, "\n")
	inFence, openChar, openLen := false, byte(0), 0
	for _, line := range lines {
		ch, n := fenceRun(line)
		if !inFence {
			if ch != 0 {
				inFence, openChar, openLen = true, ch, n
			}
			continue
		}
		if ch == openChar && n >= openLen {
			inFence = false
		}
	}
	if inFence {
		return content
	}
	var out strings.Builder
	inFence = false
	for _, line := range lines {
		ch, n := fenceRun(line)
		if !inFence {
			if ch != 0 {
				inFence, openChar, openLen = true, ch, n
			} else {
				out.WriteString(line)
				out.WriteString("\n")
			}
			continue
		}
		if ch == openChar && n >= openLen {
			inFence = false
		}
	}
	return out.String()
}

// fenceRun reports the fence delimiter and run length of a line, or
// zeros when the line does not open or close a fence.
func fenceRun(line string) (byte, int) {
	t := strings.TrimLeft(line, " ")
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return 0, 0
	}
	i := 0
	for i < len(t) && t[i] == t[0] {
		i++
	}
	if i < 3 {
		return 0, 0
	}
	return t[0], i
}

// Root and zone README/AGENTS/CONTEXT documents teach the marker;
// none of them cite anything.
var utilityCiteDocsSkipped = map[string]bool{
	"AGENTS.md":  true,
	"README.md":  true,
	"CONTEXT.md": true,
}

type utilityCite struct {
	count int
	files []string
}

// utilityInjections reads the telemetry state: one record per line,
// {"date":"YYYY-MM-DD","path":"quirks/x.md"}. A record dated after the
// run's UTC date is corruption or clock skew and fails the command:
// future mass would be pure fiction.
func utilityInjections(kbRoot string, now time.Time) (map[string][]time.Time, bool, error) {
	gitDir, err := sync.GitDirOf(kbRoot)
	if err != nil {
		return nil, false, nil
	}
	data, err := os.ReadFile(filepath.Join(gitDir, UtilityStateFile))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	today := now.UTC().Format("2006-01-02")
	out := map[string][]time.Time{}
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			Date string `json:"date"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, true, fmt.Errorf("utility state %s line %d: %w", UtilityStateFile, i+1, err)
		}
		day, err := time.Parse("2006-01-02", rec.Date)
		if err != nil {
			return nil, true, fmt.Errorf("utility state %s line %d: record date %q: %w", UtilityStateFile, i+1, rec.Date, err)
		}
		if rec.Date > today {
			return nil, true, fmt.Errorf("utility state %s line %d: record date %q is in the future", UtilityStateFile, i+1, rec.Date)
		}
		if !utilityStatePathOK(rec.Path) {
			return nil, true, fmt.Errorf("utility state %s line %d: record path %q is not a ledger-relative .md path", UtilityStateFile, i+1, rec.Path)
		}
		out[rec.Path] = append(out[rec.Path], day)
	}
	return out, true, nil
}

// utilityStatePathOK rejects anything that is not a clean
// ledger-relative .md path: the state file is machine-written, so a
// violating record means tampering or corruption and fails closed.
func utilityStatePathOK(p string) bool {
	if p == "" || filepath.IsAbs(p) || !strings.HasSuffix(p, ".md") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	return clean == p && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}

// utilityScore divides citations by the weighted injection mass: every
// injection decays with the half-life, and a document never delivered
// carries no score at all.
func utilityScore(inj []time.Time, citations int, now time.Time) float64 {
	mass := 0.0
	for _, t := range inj {
		mass += utilityWeight(t, now)
	}
	if mass == 0 {
		return 0
	}
	return float64(citations) / mass
}

func utilityWeight(t, now time.Time) float64 {
	// Day-granular decay: telemetry dates carry no time of day, so the
	// hour of the run must not change which side of a threshold a
	// document lands on.
	midnight := func(x time.Time) time.Time {
		u := x.UTC()
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	}
	days := midnight(now).Sub(midnight(t)).Hours() / 24
	if days < 0 {
		days = 0
	}
	return math.Pow(2, -days/utilityHalfLifeDays)
}

// utilityPrunable reports an uncited quirk delivered on at least two
// distinct days whose weighted injection mass clears the floor:
// repeated surfacing across sessions, never one heavy day.
func utilityPrunable(kbRoot string, row UtilityRow, inj []time.Time, now time.Time) bool {
	if row.Citations != 0 || row.Injections == 0 {
		return false
	}
	if !strings.HasPrefix(row.Path, "quirks/") || strings.Contains(row.Path, "/archive/") {
		return false
	}
	days := map[string]bool{}
	mass := 0.0
	for _, t := range inj {
		days[t.Format("2006-01-02")] = true
		mass += utilityWeight(t, now)
	}
	if len(days) < 2 || mass < utilityPruneFloor {
		return false
	}
	data, err := os.ReadFile(filepath.Join(kbRoot, filepath.FromSlash(row.Path)))
	if err != nil {
		return false
	}
	var meta resolve.DocMeta
	return resolve.ParseDocMeta(string(data), &meta) && meta.Type == "quirk"
}

func utilityPromoteCandidates(rows []UtilityRow) []UtilityRow {
	var out []UtilityRow
	for _, row := range rows {
		if row.Citations >= utilityPromoteCites && len(row.CitingFiles) >= utilityPromoteFiles {
			out = append(out, row)
		}
	}
	return out
}

// utilityArchiveQuirk moves an uncited quirk to quirks/archive/. An
// existing destination is never overwritten: the move skips and stays
// reportable.
func utilityArchiveQuirk(kbRoot, path string) (bool, error) {
	src := filepath.Join(kbRoot, filepath.FromSlash(path))
	dstDir := filepath.Join(kbRoot, "quirks", "archive")
	dst := filepath.Join(dstDir, filepath.Base(path))
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(src, dst); err != nil {
		return false, err
	}
	return true, nil
}

// utilityWriteStub writes one convention proposal per cited document.
// An existing stub is never rewritten: idempotent across runs. The
// stub name derives from the whole ledger path, sanitized, so two
// same-stem documents never fight over one file and a hostile stem
// cannot shape the filename.
func utilityWriteStub(kbRoot string, row UtilityRow, today string) (bool, error) {
	slug := utilityStubSlug(strings.TrimSuffix(row.Path, ".md"))
	dir := filepath.Join(kbRoot, "conventions", "proposals")
	dst := filepath.Join(dir, fmt.Sprintf("conv-utility-%s.md", slug))
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	doc := strings.Join([]string{
		"---",
		fmt.Sprintf("title: %q", "Utility: promote "+row.Path),
		"type: convention",
		fmt.Sprintf("slug: conv.utility.%s", slug),
		"level: SHOULD",
		"status: proposed",
		"created: " + today,
		"updated: " + today,
		"owners:",
		"  - Pylons",
		"ticket: []",
		`applies_to: ["all"]`,
		"supersedes: []",
		`artifact: ""`,
		`description: ""`,
		"tags: [conventions]",
		"---",
		"",
		"## Rule",
		"",
		fmt.Sprintf("Promote the knowledge in `%s` to a formal convention.", row.Path), "",
		"## Rationale",
		"",
		fmt.Sprintf("The document collected %d citations across %s, so sessions keep using it.", row.Citations, strings.Join(row.CitingFiles, ", ")),
		"",
	}, "\n")
	if err := os.WriteFile(dst, []byte(doc+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// utilityStubSlug folds a ledger path into [a-z0-9-] plus a 64-bit
// hash of the raw path. The fold is not injective (a-b, a_b, a/b all
// collapse) and the hash only makes collisions improbable (~2^-64 per
// pair), but a collision costs one silently skipped stub instead of
// two documents fighting over one file.
func utilityStubSlug(path string) string {
	lower := strings.ToLower(path)
	var b strings.Builder
	dash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "doc"
	}
	h := fnv.New64a()
	h.Write([]byte(path))
	return fmt.Sprintf("%s-%016x", out, h.Sum64())
}
