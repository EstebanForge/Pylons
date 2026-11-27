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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/install"
	"github.com/estebanforge/pylons/cli/internal/rules"
	"github.com/estebanforge/pylons/cli/internal/version"
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
	fs = append(fs, checkZoneDocs(kbRoot)...)
	fs = append(fs, checkKeepers(kbRoot)...)
	fs = append(fs, checkProductTrees(kbRoot)...)
	fs = append(fs, checkProjects(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkHooks(kbRoot)...)
	fs = append(fs, checkBinary(kbRoot)...)
	fs = append(fs, checkGeneratedManifests(kbRoot, cfg, cfgErr)...)
	fs = append(fs, checkProductRecord(kbRoot)...)
	fs = append(fs, checkBaseline(kbRoot)...)
	return fs, nil
}

// Fix applies every fixable finding and returns one line per repair.
// Findings that stay unfixable are the user's calls by design: legal
// text, semantic paths, and real binary builds.
func Fix(kbRoot string) ([]string, error) {
	fs, err := Run(kbRoot)
	if err != nil {
		return nil, err
	}
	var fixed []string
	byID := map[string][]Finding{}
	for _, f := range fs {
		byID[f.ID] = append(byID[f.ID], f)
	}
	if _, ok := byID["DOCTOR002"]; ok {
		for _, f := range byID["DOCTOR002"] {
			if err := writeZoneDoc(kbRoot, f.File); err != nil {
				return fixed, err
			}
			fixed = append(fixed, fmt.Sprintf("%s: wrote %s", f.ID, f.File))
		}
	}
	for _, f := range byID["DOCTOR003"] {
		path := filepath.Join(kbRoot, filepath.FromSlash(f.File))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fixed, err
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return fixed, err
		}
		fixed = append(fixed, fmt.Sprintf("%s: created %s", f.ID, f.File))
	}
	if _, ok := byID["DOCTOR007"]; ok {
		if _, err := install.EnsureHooks(kbRoot, false); err != nil {
			return fixed, fmt.Errorf("hook wiring: %w", err)
		}
		fixed = append(fixed, "DOCTOR007: wired gate hooks via pylon install")
	}
	for _, f := range byID["DOCTOR008"] {
		if !strings.Contains(f.Message, "locally edited") {
			continue
		}
		if _, err := install.EnsureHooks(kbRoot, true); err != nil {
			return fixed, fmt.Errorf("hook repair: %w", err)
		}
		fixed = append(fixed, "DOCTOR008: rewrote drifted hooks (old copies kept as .bak)")
	}
	for _, f := range byID["DOCTOR009"] {
		if !f.Fixable {
			continue
		}
		path := filepath.Join(kbRoot, "bin", "pylon")
		if err := os.Chmod(path, 0o755); err != nil {
			return fixed, err
		}
		fixed = append(fixed, fmt.Sprintf("%s: made %s executable", f.ID, f.File))
	}
	for _, f := range byID["DOCTOR010"] {
		if err := regenerateManifest(kbRoot, f.File); err != nil {
			return fixed, err
		}
		fixed = append(fixed, fmt.Sprintf("%s: regenerated %s", f.ID, f.File))
	}
	if _, ok := byID["DOCTOR011"]; ok {
		line, err := recordProductState(kbRoot)
		if err != nil {
			return fixed, err
		}
		if line != "" {
			fixed = append(fixed, line)
		}
	}
	if _, ok := byID["DOCTOR012"]; ok {
		if err := rewriteBaseline(kbRoot); err != nil {
			return fixed, err
		}
		fixed = append(fixed, "DOCTOR012: rewrote manifests/baseline.json from a fresh lint run")
	}
	return fixed, nil
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

func checkZoneDocs(kbRoot string) []Finding {
	var fs []Finding
	for _, zone := range install.DocZones() {
		rel := zone + "/AGENTS.md"
		if _, err := os.Stat(filepath.Join(kbRoot, zone, "AGENTS.md")); err != nil {
			fs = append(fs, Finding{
				ID: "DOCTOR002", Severity: "error", File: rel, Fixable: true,
				Message: fmt.Sprintf("zone %s has no AGENTS.md entry doc", zone),
				Fix:     "doctor --fix writes a minimal entry doc",
			})
		}
	}
	return fs
}

func checkKeepers(kbRoot string) []Finding {
	var fs []Finding
	for _, keeper := range install.Keepers() {
		rel := keeper + "/.gitkeep"
		if _, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(rel))); err != nil {
			fs = append(fs, Finding{
				ID: "DOCTOR003", Severity: "warning", File: rel, Fixable: true,
				Message: fmt.Sprintf("archive directory %s has no .gitkeep; clones lose it", keeper),
				Fix:     "doctor --fix creates the placeholder",
			})
		}
	}
	return fs
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
	var fs []Finding
	if len(st.Missing) > 0 {
		fs = append(fs, Finding{
			ID: "DOCTOR007", Severity: "error",
			File: st.HooksPath, Fixable: true,
			Message: fmt.Sprintf("gate hooks missing from .githooks: %s", strings.Join(st.Missing, ", ")),
			Fix:     "doctor --fix runs pylon install to write them",
		})
	}
	switch st.HooksPath {
	case "":
		fs = append(fs, Finding{
			ID: "DOCTOR007", Severity: "error", Fixable: true,
			Message: "core.hooksPath is not set; the gates never run",
			Fix:     "doctor --fix sets it to .githooks",
		})
	case ".githooks":
		// wired
	default:
		fs = append(fs, Finding{
			ID: "DOCTOR008", Severity: "warning",
			Message: fmt.Sprintf("core.hooksPath points at %q, owned by another tool", st.HooksPath),
			Fix:     "decide which owner wins, then run pylon install --force to take it back",
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

// generates reports whether a kind is enabled under manifests.generate;
// a nil list means the default pair ships enabled.
func generates(cfg config.Config, kind string) bool {
	if cfg.Generate == nil {
		return kind == "rules" || kind == "packages"
	}
	for _, g := range *cfg.Generate {
		if g == kind {
			return true
		}
	}
	return false
}

func checkGeneratedManifests(kbRoot string, cfg config.Config, cfgErr error) []Finding {
	var fs []Finding
	for _, kind := range generatedManifests {
		if cfgErr == nil && !generates(cfg, kind) {
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
			ID: "DOCTOR011", Severity: "warning", File: "manifests/product.json", Fixable: true,
			Message: "no product record; a legacy layout where the first self-update reports every product file as drifted",
			Fix:     "doctor --fix records the current state as applied",
		}}
	}
	if _, err := install.ReadProductManifest(path); err != nil {
		return []Finding{{
			ID: "DOCTOR011", Severity: "warning", File: "manifests/product.json", Fixable: true,
			Message: fmt.Sprintf("product record unreadable: %v", err),
			Fix:     "doctor --fix re-records the current state",
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
	var m struct {
		Version int `json:"version"`
	}
	data, err := os.ReadFile(path)
	if err == nil && json.Unmarshal(data, &m) == nil && m.Version == 1 {
		return nil
	}
	return []Finding{{
		ID: "DOCTOR012", Severity: "error", File: "manifests/baseline.json", Fixable: true,
		Message: "baseline.json is unreadable or not version 1; lint rejects it",
		Fix:     "doctor --fix rewrites it from a fresh lint run (accepted debt not currently matching lint is dropped)",
	}}
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

func recordProductState(kbRoot string) (string, error) {
	m, err := install.RecordCurrentState(kbRoot, version.Version)
	if err != nil {
		// Nothing product-owned exists yet; leave the warning standing.
		return "", nil
	}
	pin := filepath.Join(kbRoot, "bin", "pylon")
	if st, statErr := os.Stat(pin); statErr == nil && st.Mode().IsRegular() {
		if err := install.PinBinary(&m, runtime.GOOS+"-"+runtime.GOARCH, pin); err != nil {
			return "", err
		}
	}
	data, err := install.WriteProductManifest(m)
	if err != nil {
		return "", err
	}
	path := filepath.Join(kbRoot, "manifests", "product.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	note := ""
	if _, statErr := os.Stat(pin); statErr != nil {
		note = " (no binary pinned; bin/pylon absent)"
	}
	return fmt.Sprintf("DOCTOR011: recorded manifests/product.json as version %s%s", version.Version, note), nil
}

func rewriteBaseline(kbRoot string) error {
	findings := rules.Lint(kbRoot, rules.LintOptions{HasCode: true})
	return rules.UpdateBaseline(kbRoot, findings)
}
