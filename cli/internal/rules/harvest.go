// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// SymbolCandidate is one exported symbol added by a merged PR.
type SymbolCandidate struct {
	Path     string // diff path as written in the patch
	Alias    string // owning project alias, empty when unattributed
	Package  string // owning package doc name, empty when unattributed
	Kind     string // contract kind, e.g. go-symbol
	Name     string
	ID       string // repo://alias/relpath::Name, empty when unattributed
	Proposed bool   // false when the package already declares the symbol
}

// UnattributedSymbol is an exported symbol under no registered project.
type UnattributedSymbol struct {
	Path string
	Name string
}

// QuirkCandidate is a scaffolded post-mortem proposal for one Footgun
// marker found in the PR metadata.
type QuirkCandidate struct {
	Slug   string
	Title  string
	Text   string // the full Footgun marker text
	Body   string
	Source string // e.g. "commit fix: replay storm"
}

// HarvestReport is the full candidate set for one PR.
type HarvestReport struct {
	Symbols      []SymbolCandidate
	Unattributed []UnattributedSymbol
	Quirks       []QuirkCandidate
	Warnings     []string
}

type prMetadata struct {
	Title   string `json:"title"`
	Labels  []string
	Commits []struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	Reviews []struct {
		Body string `json:"body"`
	}
}

var harvestDeclRules = []struct {
	exts []string
	re   *regexp.Regexp
	kind string
}{
	{[]string{".go"}, regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)\s*\(|^(?:type|var|const)\s+([A-Z]\w*)`), "go-symbol"},
	{[]string{".rs"}, regexp.MustCompile(`^pub\s+(?:const|static|fn|struct|enum|trait|type)\s+(\w+)`), "rust-item"},
	{[]string{".ts", ".tsx", ".js", ".jsx"}, regexp.MustCompile(`^export\s+(?:default\s+)?(?:async\s+)?(?:function\s*\*?\s*|class\s+|const\s+|let\s+|var\s+|interface\s+|type\s+|enum\s+)(\w+)`), "ts-export"},
	{[]string{".php"}, regexp.MustCompile(`^public\s+(?:static\s+)?(?:function\s+([A-Za-z]\w*)|const\s+([A-Za-z]\w*)|class\s+(\w+)|interface\s+(\w+))`), "php-symbol"},
}

var harvestExts = func() map[string]int {
	m := map[string]int{}
	for i, r := range harvestDeclRules {
		for _, e := range r.exts {
			m[e] = i
		}
	}
	return m
}()

// HarvestPR parses a merged PR diff plus optional PR metadata into the
// candidate set: exported symbols as package exposes proposals and
// scaffolded quirk post-mortems for Footgun markers. Report-only: the
// caller publishes the artifact; a maintainer applies it by hand.
func HarvestPR(kbRoot string, diff []byte, meta []byte) (HarvestReport, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return HarvestReport{}, err
	}
	if abs, aerr := filepath.Abs(kbRoot); aerr == nil {
		kbRoot = abs
	}
	if resolved, serr := filepath.EvalSymlinks(kbRoot); serr == nil {
		kbRoot = resolved
	}
	rep := HarvestReport{}
	pkgs := harvestPackages(kbRoot, cfg)
	claimed := map[string]bool{}
	for _, p := range pkgs {
		for id := range p.exposing {
			claimed[id] = true
		}
	}

	type fileSym struct {
		path  string
		kind  string
		name  string
		rule  int
		match []string
	}
	var syms []fileSym
	current := ""
	binary := false
	for _, line := range strings.Split(string(diff), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			current, binary = "", false
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			current = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "Binary files ") {
			binary = true
			rep.Warnings = append(rep.Warnings, "binary file skipped: "+current)
			continue
		}
		if current == "" || binary || !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		if isHarvestTestFile(current) || strings.Contains(current, "/vendor/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(current))
		ruleIdx, ok := harvestExts[ext]
		if !ok {
			continue
		}
		m := harvestDeclRules[ruleIdx].re.FindStringSubmatch(line[1:])
		if m == nil {
			continue
		}
		name := ""
		for _, g := range m[1:] {
			if g != "" {
				name = g
				break
			}
		}
		if name == "" {
			continue
		}
		syms = append(syms, fileSym{path: current, kind: harvestDeclRules[ruleIdx].kind, name: name})
	}

	seen := map[string]bool{}
	for _, s := range syms {
		alias, absPath := harvestAlias(kbRoot, cfg, s.path)
		cand := SymbolCandidate{Path: s.path, Alias: alias, Kind: s.kind, Name: s.name}
		if alias == "" {
			rep.Unattributed = append(rep.Unattributed, UnattributedSymbol{Path: s.path, Name: s.name})
			rep.Symbols = append(rep.Symbols, cand)
			continue
		}
		projPath := cfg.ProjectPath(kbRoot, alias)
		rel, err := filepath.Rel(projPath, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		cand.Package = harvestOwningPackage(pkgs, alias)
		if cand.Package == "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("no package doc claims alias %s", alias))
		}
		cand.ID = fmt.Sprintf("repo://%s/%s::%s", alias, filepath.ToSlash(rel), s.name)
		cand.Proposed = cand.Package != "" && !claimed[cand.ID]
		key := cand.Kind + "\x00" + cand.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		rep.Symbols = append(rep.Symbols, cand)
	}

	if len(meta) > 0 {
		var pm prMetadata
		if err := json.Unmarshal(meta, &pm); err != nil {
			return HarvestReport{}, fmt.Errorf("metadata: %w", err)
		}
		rep.Quirks = harvestFootguns(&pm)
	}
	slices.SortFunc(rep.Warnings, strings.Compare)
	rep.Warnings = slices.Compact(rep.Warnings)
	return rep, nil
}

// harvestAlias tries a diff path absolute from the caller's working
// directory first (CI checkout root), then ledger-relative, resolving
// symlinks on both sides; relative targets never carry an alias by
// design, so only these two forms attribute.
func harvestAlias(kbRoot string, cfg config.Config, path string) (string, string) {
	cands := make([]string, 0, 2)
	if abs, err := filepath.Abs(path); err == nil {
		cands = append(cands, abs)
	}
	if !filepath.IsAbs(path) {
		cands = append(cands, filepath.Join(kbRoot, path))
	}
	for _, c := range cands {
		p := c
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if t := normalizeCheckTarget(kbRoot, p, cfg); t.alias != "" {
			return t.alias, p
		}
	}
	return "", path
}

type harvestPackage struct {
	doc      string
	name     string
	comp     []string
	exposing map[string]bool
}

func harvestPackages(kbRoot string, cfg config.Config) []harvestPackage {
	var out []harvestPackage
	dir := filepath.Join(kbRoot, "packages")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "README.md" || name == "AGENTS.md" || strings.HasPrefix(name, "._") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var fm resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(string(data), &fm) || fm.Type != "package" {
			continue
		}
		p := harvestPackage{doc: "packages/" + name, name: fm.Package, comp: fm.Components, exposing: map[string]bool{}}
		for _, x := range fm.Exposes {
			if id, _ := x["id"].(string); id != "" {
				p.exposing[id] = true
			}
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b harvestPackage) int { return strings.Compare(a.doc, b.doc) })
	return out
}

func harvestOwningPackage(pkgs []harvestPackage, alias string) string {
	for _, p := range pkgs {
		if p.name == alias || slices.Contains(p.comp, alias) {
			return p.name
		}
	}
	return ""
}

func isHarvestTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_test.rs") ||
		base == "Test.php" || strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx")
}

// Footgun markers follow the closed prose-marker precedent: a
// line-anchored prefix in commit or review bodies.
var footgunRe = regexp.MustCompile(`^Footgun:\s*(.+)$`)

func harvestFootguns(pm *prMetadata) []QuirkCandidate {
	type marker struct{ text, source string }
	var markers []marker
	for _, c := range pm.Commits {
		for _, line := range strings.Split(c.Body, "\n") {
			if m := footgunRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				markers = append(markers, marker{strings.TrimSpace(m[1]), "commit " + c.Subject})
			}
		}
	}
	for i, r := range pm.Reviews {
		for _, line := range strings.Split(r.Body, "\n") {
			if m := footgunRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				markers = append(markers, marker{strings.TrimSpace(m[1]), fmt.Sprintf("review comment %d", i+1)})
			}
		}
	}
	today := time.Now().Format("2006-01-02")
	slugs := map[string]int{}
	var out []QuirkCandidate
	for _, mk := range markers {
		slug := harvestSlug(mk.text)
		if n := slugs[slug]; n > 0 {
			slug = fmt.Sprintf("%s-%d", slug, n+1)
		}
		slugs[slug]++
		words := strings.Fields(mk.text)
		if len(words) > 8 {
			words = words[:8]
		}
		q := QuirkCandidate{
			Slug:   slug,
			Title:  "Footgun: " + strings.Join(words, " "),
			Text:   mk.text,
			Source: mk.source,
		}
		q.Body = strings.Join([]string{
			"---",
			fmt.Sprintf("title: %q", q.Title),
			"type: quirk",
			"status: active",
			"created: " + today,
			"updated: " + today,
			"owners:",
			"  - Pylons",
			`package: ""`,
			"guard: convention",
			`guardrail: ""`,
			"---",
			"",
			"## Symptom",
			"",
			mk.text,
			"",
			"## Cause",
			"",
			"Fill in from the PR context.",
			"",
			"## Resolution",
			"",
			"Fill in from the PR context.",
			"",
		}, "\n")
		out = append(out, q)
	}
	return out
}

func harvestSlug(text string) string {
	var parts []string
	for _, w := range strings.Fields(strings.ToLower(text)) {
		var b strings.Builder
		for _, r := range w {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		if b.Len() > 0 {
			parts = append(parts, b.String())
		}
		if len(parts) == 5 {
			break
		}
	}
	if len(parts) == 0 {
		return "footgun"
	}
	return strings.Join(parts, "-")
}
