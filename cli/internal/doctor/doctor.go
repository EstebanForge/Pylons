// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Package doctor audits the structural health of a ledger: the files
// the shipped layout expects, manifest parsability and freshness, gate
// hook wiring, and the record self-update needs. It checks the
// structure lint assumes; content rules stay with lint.
package doctor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/install"
	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

// Finding is one structural health report line.
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // error, warning, info
	File     string `json:"file,omitempty"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
	Fixable  bool   `json:"fixable"`
}

// generatedManifests are the manifests the engine can rebuild from
// ledger state; parity.json and the example files ship verbatim.
var generatedManifests = []string{"rules", "packages", "vocabulary"}

// zoneBlurbs seed synthesized entry docs; anything unmapped gets the
// generic line.
var zoneBlurbs = map[string]string{
	"packages":       "Package docs: what each project does and what to build on",
	"proposals":      "Design proposals awaiting or recording a decision",
	"plans":          "Implementation plans and their milestone state",
	"decisions":      "Accepted decisions with their rationale",
	"conventions":    "Conventions the ledger enforces",
	"quirks":         "Non-obvious behavior worth remembering",
	"domain":         "Domain entities and their vocabulary",
	"investigations": "Point-in-time investigations",
	"references":     "Reference entries",
	"tickets":        "Ticket notes",
	"prompts":        "Agent prompts",
	"skills":         "Agent skills",
}

// Run inspects kbRoot and returns findings. It never writes. A missing
// or invalid pylons.yaml is reported, not fatal: the other structural
// checks still have something to say.
func Run(kbRoot string) ([]Finding, error) {
	var fs []Finding
	cfg, cfgErr := config.Load(kbRoot)
	if cfgErr != nil {
		fs = append(fs, Finding{
			ID: "DOCTOR005", Severity: "error", File: "manifests/pylons.yaml",
			Message: fmt.Sprintf("pylons.yaml unreadable or invalid: %v", cfgErr),
			Fix:     "repair the yaml by hand; doctor cannot guess your configuration",
		})
	}
	fs = append(fs, checkExactFiles(kbRoot)...)
	fs = append(fs, checkZoneDocs(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkKeepers(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkProductTrees(kbRoot)...)
	fs = append(fs, checkProjects(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkHooks(kbRoot)...)
	fs = append(fs, checkBinary(kbRoot)...)
	fs = append(fs, checkGeneratedManifests(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkProductRecord(kbRoot)...)
	fs = append(fs, checkBaseline(kbRoot)...)
	fs = append(fs, checkCorpusSize(kbRoot, cfg, cfgErr)...)
	return fs, nil
}

// Fix applies every fixable finding and returns one line per repair.
// Repair failures are collected, not fatal: one broken repair never
// silently skips the rest. Findings that stay unfixable are the
// user's calls by design: legal text, semantic paths, a real binary
// build, and the product record, which only a real release applied by
// self-update can establish.
func Fix(kbRoot string) ([]string, error) {
	fs, err := Run(kbRoot)
	if err != nil {
		return nil, err
	}
	// Only cfgErr matters here: the DOCTOR010 repair gate reads the
	// generate opt-out through it.
	_, cfgErr := config.Load(kbRoot)
	var fixed []string
	var failures []string
	byID := map[string][]Finding{}
	for _, f := range fs {
		byID[f.ID] = append(byID[f.ID], f)
	}
	fail := func(what string, err error) {
		failures = append(failures, fmt.Sprintf("repair %s failed: %v", what, err))
	}
	for _, f := range byID["DOCTOR002"] {
		if err := writeZoneDoc(kbRoot, f.File); err != nil {
			fail(f.File, err)
			continue
		}
		fixed = append(fixed, fmt.Sprintf("%s: wrote %s", f.ID, f.File))
	}
	for _, f := range byID["DOCTOR003"] {
		path := filepath.Join(kbRoot, filepath.FromSlash(f.File))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fail(f.File, err)
			continue
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			fail(f.File, err)
			continue
		}
		fixed = append(fixed, fmt.Sprintf("%s: created %s", f.ID, f.File))
	}
	if hookRepairDue(byID) {
		if _, err := install.EnsureHooks(kbRoot, len(byID["DOCTOR008"]) > 0); err != nil {
			fail("hook wiring", err)
		} else {
			fixed = append(fixed, "hooks wired or repaired via pylon install")
		}
	}
	for _, f := range byID["DOCTOR009"] {
		if !f.Fixable {
			continue
		}
		path := filepath.Join(kbRoot, "bin", "pylon")
		if err := os.Chmod(path, 0o755); err != nil {
			fail(f.File, err)
			continue
		}
		fixed = append(fixed, fmt.Sprintf("%s: made %s executable", f.ID, f.File))
	}
	// With an unreadable pylons.yaml the generate opt-out cannot be
	// read, so regenerating could stomp manifests handed to foreign
	// tooling.
	if len(byID["DOCTOR010"]) > 0 && cfgErr != nil {
		failures = append(failures, "repair manifests skipped: pylons.yaml is unreadable, so the generate opt-out cannot be checked; fix the yaml first")
	} else {
		for _, f := range byID["DOCTOR010"] {
			if err := regenerateManifest(kbRoot, f.File); err != nil {
				fail(f.File, err)
				continue
			}
			fixed = append(fixed, fmt.Sprintf("%s: regenerated %s", f.ID, f.File))
		}
	}
	for _, f := range byID["DOCTOR012"] {
		if !f.Fixable {
			continue
		}
		if err := rewriteBaseline(kbRoot); err != nil {
			fail("baseline rewrite", err)
			continue
		}
		fixed = append(fixed, "DOCTOR012: rewrote manifests/baseline.json from a fresh lint run (old copy kept as .bak; every FORCED finding lint reports now is accepted)")
	}
	if len(failures) > 0 {
		return fixed, errors.New(strings.Join(failures, "; "))
	}
	return fixed, nil
}

// hookRepairDue reports whether any hook finding is doctor-fixable;
// foreign-owned paths and nested ledgers are hands-on by design.
func hookRepairDue(byID map[string][]Finding) bool {
	for _, id := range []string{"DOCTOR007", "DOCTOR008"} {
		for _, f := range byID[id] {
			if f.Fixable {
				return true
			}
		}
	}
	return false
}

// --- checks ---

func checkExactFiles(kbRoot string) []Finding {
	var fs []Finding
	for _, rel := range install.ExactFiles() {
		// rules.json and vocabulary.json are generated; DOCTOR010 owns them.
		if rel == "manifests/rules.json" || rel == "manifests/vocabulary.json" {
			continue
		}
		if _, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(rel))); err != nil {
			fs = append(fs, Finding{
				ID: "DOCTOR001", Severity: "error", File: rel,
				Message: fmt.Sprintf("missing product file %s", rel),
				Fix:     "re-run the installer or copy the file from a release; doctor cannot invent legal or example text",
			})
		}
	}
	return fs
}

func checkZoneDocs(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	var fs []Finding
	for _, zone := range install.DocZones() {
		rel := zone + "/AGENTS.md"
		if cfgErr == nil && pathExcluded(cfg, rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(kbRoot, zone, "AGENTS.md")); statSaysMissing(err) {
			fs = append(fs, Finding{
				ID: "DOCTOR002", Severity: "error", File: rel, Fixable: true,
				Message: fmt.Sprintf("zone %s has no AGENTS.md entry doc", zone),
				Fix:     "doctor --fix writes a minimal entry doc",
			})
		}
	}
	return fs
}

func checkKeepers(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	var fs []Finding
	for _, keeper := range install.Keepers() {
		rel := keeper + "/.gitkeep"
		if cfgErr == nil && pathExcluded(cfg, rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(rel))); statSaysMissing(err) {
			fs = append(fs, Finding{
				ID: "DOCTOR003", Severity: "warning", File: rel, Fixable: true,
				Message: fmt.Sprintf("archive directory %s has no .gitkeep; clones lose it", keeper),
				Fix:     "doctor --fix creates the placeholder",
			})
		}
	}
	return fs
}

// pathExcluded reports whether a pylons.yaml exclude glob covers rel;
// excluded paths are silent by design on every surface, including
// doctor's structural expectations.
func pathExcluded(cfg config.Config, rel string) bool {
	for _, pattern := range cfg.Exclude {
		if resolve.MatchGlob(pattern, rel) {
			return true
		}
	}
	return false
}

// statSaysMissing reports whether a Stat error means the path cannot
// be read as it should be. Permission errors stay silent: doctor must
// not propose writes it cannot attempt. Everything else (absent path,
// a parent occupied by a regular file) is structural and repairable
// or honestly reported as a failed repair.
func statSaysMissing(err error) bool {
	return err != nil && !errors.Is(err, iofs.ErrPermission)
}

func checkProductTrees(kbRoot string) []Finding {
	var fs []Finding
	for _, tree := range install.ProductTrees() {
		base := filepath.Join(kbRoot, filepath.FromSlash(tree))
		empty := true
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				empty = false
			}
			return nil
		})
		if empty {
			fs = append(fs, Finding{
				ID: "DOCTOR004", Severity: "warning", File: tree,
				Message: fmt.Sprintf("product tree %s is missing or empty", tree),
				Fix:     "copy the tree from a release; its contents are versioned payload",
			})
			continue
		}
		for _, rel := range install.RequiredTreeFiles() {
			if !strings.HasPrefix(rel, tree+"/") {
				continue
			}
			if _, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(rel))); err != nil {
				fs = append(fs, Finding{
					ID: "DOCTOR004", Severity: "warning", File: rel,
					Message: fmt.Sprintf("product tree %s is missing required file %s", tree, rel),
					Fix:     "copy the file from a release",
				})
			}
		}
	}
	return fs
}

func checkProjects(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	if cfgErr != nil {
		return nil
	}
	var fs []Finding
	for name, pc := range cfg.Projects {
		path := pc.Path
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(kbRoot, path)
		}
		if _, err := os.Stat(path); err != nil {
			fs = append(fs, Finding{
				ID: "DOCTOR006", Severity: "warning", File: "manifests/pylons.yaml",
				Message: fmt.Sprintf("project %s registers %q, which does not exist on disk; repo:// links into it cannot resolve", name, pc.Path),
				Fix:     "check out the project or fix the registered path; doctor cannot decide for you",
			})
		}
	}
	return fs
}

func checkHooks(kbRoot string) []Finding {
	st := install.CheckHooks(kbRoot)
	if !st.InGitRepo {
		return nil
	}
	if kbAbs, err := filepath.Abs(kbRoot); err == nil && filepath.Clean(st.RepoRoot) != filepath.Clean(kbAbs) {
		return []Finding{{
			ID: "DOCTOR008", Severity: "info", File: st.RepoRoot,
			Message: "ledger sits inside the repo rooted at " + st.RepoRoot + "; gate hooks belong to that repo",
			Fix:     "run pylon install at the repository root, or leave the gates to the parent checkout",
		}}
	}
	if st.HooksPath != "" && st.HooksPath != ".githooks" {
		return []Finding{{
			ID: "DOCTOR008", Severity: "warning",
			Message: fmt.Sprintf("core.hooksPath points at %q, owned by another tool; the gates may not run", st.HooksPath),
			Fix:     "decide which owner wins, then run pylon install --force to take it back",
		}}
	}
	var fs []Finding
	if len(st.Missing) > 0 {
		fs = append(fs, Finding{
			ID: "DOCTOR007", Severity: "error", Fixable: true,
			Message: fmt.Sprintf("gate hooks missing from .githooks: %s", strings.Join(st.Missing, ", ")),
			Fix:     "doctor --fix runs pylon install to write them",
		})
	}
	if len(st.Drifted) > 0 {
		fs = append(fs, Finding{
			ID: "DOCTOR008", Severity: "warning", Fixable: true,
			Message: fmt.Sprintf("hooks locally edited: %s", strings.Join(st.Drifted, ", ")),
			Fix:     "doctor --fix rewrites them from the shipped scripts, keeping .bak copies",
		})
	}
	return fs
}

func checkBinary(kbRoot string) []Finding {
	path := filepath.Join(kbRoot, "bin", "pylon")
	st, err := os.Stat(path)
	switch {
	case err != nil:
		return []Finding{{
			ID: "DOCTOR009", Severity: "warning", File: "bin/pylon",
			Message: "bin/pylon is missing; gate hooks fall back to whatever pylon is on PATH, which has no version check",
			Fix:     "run the installer with --bin-only, or copy a release binary in",
		}}
	case !st.Mode().IsRegular():
		return []Finding{{
			ID: "DOCTOR009", Severity: "warning", File: "bin/pylon",
			Message: "bin/pylon is not a regular file; the gates cannot trust it",
		}}
	case st.Mode().Perm()&0o100 == 0:
		return []Finding{{
			ID: "DOCTOR009", Severity: "warning", File: "bin/pylon", Fixable: true,
			Message: "bin/pylon is not executable; hooks cannot run it",
			Fix:     "doctor --fix sets the executable bit",
		}}
	}
	return nil
}

// manifestPayload returns the generator payload for one manifest kind;
// manifestBytes renders it exactly as the manifest command writes it.
func manifestPayload(kind, kbRoot string) any {
	switch kind {
	case "rules":
		return rules.AllRules()
	case "packages":
		return rules.BuildPackagesManifest(kbRoot)
	default:
		return rules.BuildVocabularyManifest()
	}
}

func manifestBytes(payload any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func checkGeneratedManifests(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	var fs []Finding
	for _, kind := range generatedManifests {
		if cfgErr == nil && !cfg.GeneratesManifest(kind) {
			continue
		}
		rel := "manifests/" + kind + ".json"
		path := filepath.Join(kbRoot, filepath.FromSlash(rel))
		payload := manifestPayload(kind, kbRoot)
		want, err := manifestBytes(payload)
		if err != nil {
			continue
		}
		disk, readErr := os.ReadFile(path)
		if readErr != nil {
			fs = append(fs, Finding{
				ID: "DOCTOR010", Severity: "error", File: rel, Fixable: true,
				Message: fmt.Sprintf("generated manifest %s is missing", rel),
				Fix:     "doctor --fix regenerates it from ledger state",
			})
			continue
		}
		normalized := strings.ReplaceAll(string(disk), "\r\n", "\n")
		if !json.Valid([]byte(normalized)) {
			fs = append(fs, Finding{
				ID: "DOCTOR010", Severity: "error", File: rel, Fixable: true,
				Message: fmt.Sprintf("generated manifest %s is not valid JSON", rel),
				Fix:     "doctor --fix regenerates it",
			})
			continue
		}
		if normalized != string(want) {
			fs = append(fs, Finding{
				ID: "DOCTOR010", Severity: "warning", File: rel, Fixable: true,
				Message: fmt.Sprintf("generated manifest %s is stale against current ledger state", rel),
				Fix:     "doctor --fix regenerates it, or run pylon manifest " + kind,
			})
		}
	}
	return fs
}

func checkProductRecord(kbRoot string) []Finding {
	path := filepath.Join(kbRoot, "manifests", "product.json")
	if _, err := os.Stat(path); err != nil {
		return []Finding{{
			ID: "DOCTOR011", Severity: "warning", File: "manifests/product.json",
			Message: "no product record; until one exists, self-update treats every product file as drifted",
			Fix:     "run pylon self-update once (or reinstall); bootstrapping the record from disk would mislabel your edits as release state",
		}}
	}
	if _, err := install.ReadProductManifest(path); err != nil {
		return []Finding{{
			ID: "DOCTOR011", Severity: "warning", File: "manifests/product.json",
			Message: fmt.Sprintf("product record unreadable: %v", err),
			Fix:     "delete it and run pylon self-update once, or reinstall; the next update rewrites the record from the applied release",
		}}
	}
	return nil
}

func checkBaseline(kbRoot string) []Finding {
	path := filepath.Join(kbRoot, "manifests", "baseline.json")
	if _, err := os.Stat(path); err != nil {
		return []Finding{{
			ID: "DOCTOR012", Severity: "info", File: "manifests/baseline.json",
			Message: "no baseline; accepted debt is not recorded (nothing to do unless lint reports FORCED findings you accept)",
		}}
	}
	data, err := os.ReadFile(path)
	var m struct {
		Version int `json:"version"`
	}
	if err != nil || json.Unmarshal(data, &m) != nil {
		return []Finding{{
			ID: "DOCTOR012", Severity: "error", File: "manifests/baseline.json", Fixable: true,
			Message: "baseline.json is unreadable; lint rejects it",
			Fix:     "doctor --fix rewrites it from a fresh lint run; every FORCED finding lint reports now becomes accepted debt, and the old copy is kept as .bak",
		}}
	}
	if m.Version > 1 {
		return []Finding{{
			ID: "DOCTOR012", Severity: "error", File: "manifests/baseline.json",
			Message: fmt.Sprintf("baseline.json is version %d, written by a newer engine; this pylon cannot interpret it", m.Version),
			Fix:     "update pylon to at least that version; doctor must not downgrade the record",
		}}
	}
	if m.Version != 1 {
		return []Finding{{
			ID: "DOCTOR012", Severity: "error", File: "manifests/baseline.json", Fixable: true,
			Message: fmt.Sprintf("baseline.json is version %d, not 1; lint rejects it", m.Version),
			Fix:     "doctor --fix rewrites it from a fresh lint run; every FORCED finding lint reports now becomes accepted debt, and the old copy is kept as .bak",
		}}
	}
	return nil
}

// --- repairs ---

func writeZoneDoc(kbRoot, rel string) error {
	zone := strings.TrimSuffix(rel, "/AGENTS.md")
	blurb, ok := zoneBlurbs[zone]
	if !ok {
		blurb = "Zone docs"
	}
	doc := fmt.Sprintf(`---
title: "%s entry"
type: reference
status: active
created: %s
updated: %s
---

%s. This entry doc was written by pylon doctor --fix; describe the zone here.
`, strings.ToUpper(zone[:1])+zone[1:], time.Now().Format("2006-01-02"), time.Now().Format("2006-01-02"), blurb)
	path := filepath.Join(kbRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(doc), 0o644)
}

func regenerateManifest(kbRoot, rel string) error {
	kind := strings.TrimSuffix(strings.TrimPrefix(rel, "manifests/"), ".json")
	data, err := manifestBytes(manifestPayload(kind, kbRoot))
	if err != nil {
		return err
	}
	path := filepath.Join(kbRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func rewriteBaseline(kbRoot string) error {
	path := filepath.Join(kbRoot, "manifests", "baseline.json")
	if data, err := os.ReadFile(path); err == nil {
		backup := path + ".bak." + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return err
		}
	}
	findings := rules.Lint(kbRoot, rules.LintOptions{HasCode: true})
	return rules.UpdateBaseline(kbRoot, findings)
}

// checkCorpusSize keeps ledger growth visible: how many documents the
// corpus walk would read, how many bytes they weigh, and which carry
// the most. It stays silent while a ledger is small — the number only
// matters once the corpus is big enough that latency and recall
// budgets feel it. It counts the same corpus lint reads (excludes
// honored); archive folders count because they still sit on disk.
// Information only — doctor never trims anything to improve the number.
const (
	corpusSizeAdvisoryDocs  = 120
	corpusSizeAdvisoryBytes = 1 << 20
)

func checkCorpusSize(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	var exclude []string
	if cfgErr == nil {
		exclude = cfg.Exclude
	}
	var total int64
	docs := 0
	type weight struct {
		rel  string
		size int64
	}
	var sizes []weight
	_ = filepath.WalkDir(kbRoot, func(p string, d iofs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			relDir, rerr := filepath.Rel(kbRoot, p)
			if rerr == nil {
				relDir = filepath.ToSlash(relDir)
				if !strings.Contains(relDir, "/") {
					switch relDir {
					case ".git", "cli", "bin", "templates", "manifests", "protocols", "extensions", "pylons-docs":
						return filepath.SkipDir
					}
				}
			}
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(kbRoot, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, pat := range exclude {
			if resolve.MatchGlob(pat, rel) {
				return nil
			}
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		docs++
		total += info.Size()
		sizes = append(sizes, weight{rel: rel, size: info.Size()})
		return nil
	})
	if docs == 0 || (docs < corpusSizeAdvisoryDocs && total < corpusSizeAdvisoryBytes) {
		return nil
	}
	slices.SortStableFunc(sizes, func(a, b weight) int {
		if a.size != b.size {
			return int(b.size - a.size)
		}
		return strings.Compare(a.rel, b.rel)
	})
	shown := sizes
	if len(shown) > 3 {
		shown = shown[:3]
	}
	parts := make([]string, 0, len(shown))
	for _, w := range shown {
		parts = append(parts, fmt.Sprintf("%s (%s)", w.rel, humanBytes(w.size)))
	}
	return []Finding{{
		ID: "DOCTOR013", Severity: "info",
		Message: fmt.Sprintf("corpus: %d docs, %s total; largest: %s", docs, humanBytes(total), strings.Join(parts, ", ")),
	}}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
