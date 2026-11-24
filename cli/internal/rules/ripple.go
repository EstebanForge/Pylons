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

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// rippleRuleID is PYLON044: everything the ledger knows about a surface
// the caller is about to touch. It informs; it never gates.
const rippleRuleID = "PYLON044"

// rippleArtifacts collects, for one target path, the ledger documents
// that declare or consume the same surface.
type rippleArtifacts struct {
	contracts []string
	quirks    []string
	terms     []string
}

// rippleFindings emits one PYLON044 advisory per touched target, listing
// the plans and packages that declare the same contract id, the quirks
// anchored to the same file, and the domain terms named in the source.
func rippleFindings(kbRoot string, cfg config.Config, targets []normalizedTarget) []report.Finding {
	decls, globs := rippleIndex(kbRoot)
	terms := domainTermList(kbRoot)
	branch := ledgerBranch(kbRoot)
	var out []report.Finding
	for _, nt := range targets {
		if nt.alias == "" || nt.path == "" {
			continue
		}
		arts := rippleArtifactsFor(kbRoot, cfg, nt, decls, globs, terms, branch)
		items := slices.Concat(arts.contracts, arts.quirks, arts.terms)
		if len(items) == 0 {
			continue
		}
		if len(items) > 8 {
			items = append(items[:8], fmt.Sprintf("... and %d more", len(items)-8))
		}
		// The finding anchors to a real KB doc: the first declaring
		// contract doc, else the project manifest.
		file := "manifests/pylons.yaml"
		if len(arts.contracts) > 0 {
			file = strings.TrimSuffix(arts.contracts[0], " (contract)")
		}
		out = append(out, report.Finding{
			RuleID:   rippleRuleID,
			Severity: report.SeverityAdvisory,
			Grade:    report.GradeAdvisory,
			File:     file,
			Line:     1,
			Message:  fmt.Sprintf("RIPPLE_OPEN: %s/%s moves with %d artifact(s): %s", nt.alias, nt.path, len(arts.contracts)+len(arts.quirks)+len(arts.terms), strings.Join(items, ", ")),
		})
	}
	return out
}

// rippleDecls indexes contract ids by their surface path, naming the
// declaring document: plans (contract.stable) and packages (exposes).
type rippleDecls map[contractKey][]string

type contractKey struct {
	alias, path string
}

// rippleGlob is one glob-kind contract declaration awaiting a
// gitignore-style match against a target path.
type rippleGlob struct {
	alias, pattern, doc string
}

// rippleIndex walks the KB once and indexes plan contracts and package
// exposes, splitting exact surface paths from glob declarations.
func rippleIndex(kbRoot string) (rippleDecls, []rippleGlob) {
	decls := rippleDecls{}
	var globs []rippleGlob
	_ = filepath.WalkDir(kbRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (isRootSkippedDir(kbRoot, p) || strings.HasPrefix(d.Name(), "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" || strings.HasPrefix(d.Name(), "._") || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		content := string(data)
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(content, &meta) {
			return nil
		}
		switch meta.Type {
		case "plan":
			var pf resolve.PlanFrontMatter
			if resolve.ParsePlanFrontMatter(content, &pf) && (meta.Status == "accepted" || meta.Status == "in-progress") {
				for _, entry := range pf.Contract.Stable {
					globs = append(globs, indexContractID(decls, entry, rel)...)
				}
			}
		case "package":
			// Deprecated and retired packages keep no live ripple
			// surface, matching the import-boundary rule's filter.
			if meta.Status == "deprecated" || meta.Status == "retired" {
				return nil
			}
			var pkg resolve.PackageFrontMatter
			if resolve.ParsePackageFrontMatter(content, &pkg) {
				for _, entry := range pkg.Exposes {
					globs = append(globs, indexContractID(decls, entry, rel)...)
				}
			}
		}
		return nil
	})
	return decls, globs
}

// indexContractID records one contract entry's surface path under its
// declaring document; glob-kind ids return as pending globs instead.
func indexContractID(decls rippleDecls, entry map[string]any, rel string) []rippleGlob {
	idStr, _ := entry["id"].(string)
	if idStr == "" {
		return nil
	}
	u, ok := resolve.ParseURI(idStr)
	if !ok || u.Scheme != "repo" {
		return nil
	}
	if strings.ContainsAny(u.Path, "*?[") {
		return []rippleGlob{{alias: u.Alias, pattern: u.Path, doc: rel}}
	}
	key := contractKey{alias: u.Alias, path: uriSurfacePath(u.Path)}
	decls[key] = append(decls[key], rel)
	return nil
}

// uriSurfacePath strips a ::symbol fragment from a contract path.
func uriSurfacePath(path string) string {
	if i := strings.Index(path, "::"); i >= 0 {
		return path[:i]
	}
	return path
}

// rippleArtifactsFor resolves the artifacts for one target: declaring
// docs, quirks anchored to the same file, and domain terms found in the
// target's source.
func rippleArtifactsFor(kbRoot string, cfg config.Config, nt normalizedTarget, decls rippleDecls, globs []rippleGlob, terms []domainTerm, branch string) rippleArtifacts {
	var arts rippleArtifacts
	seen := map[string]bool{}
	addContract := func(doc string) {
		if !seen[doc] {
			seen[doc] = true
			arts.contracts = append(arts.contracts, doc+" (contract)")
		}
	}
	for _, doc := range decls[contractKey{alias: nt.alias, path: nt.path}] {
		addContract(doc)
	}
	// Glob contracts ripple through gitignore-style matching.
	for _, g := range globs {
		if g.alias == nt.alias && resolve.MatchGlob(g.pattern, nt.path) {
			addContract(g.doc)
		}
	}
	// Quirks anchored to the same file by repo:// source.
	root := cfg.ProjectPath(kbRoot, nt.alias)
	for _, q := range quirkSourcesFor(kbRoot, nt.alias, nt.path, branch) {
		if !seen[q] {
			seen[q] = true
			arts.quirks = append(arts.quirks, q+" (quirk)")
		}
	}
	// Domain terms whose name appears in the target source.
	if root != "" {
		data := readTargetBounded(filepath.Join(root, filepath.FromSlash(nt.path)))
		for _, t := range domainTermsIn(terms, data) {
			if !seen[t.rel] {
				seen[t.rel] = true
				arts.terms = append(arts.terms, t.rel+" (domain term)")
			}
		}
	}
	return arts
}

// quirkSourcesFor returns quirk documents whose repo:// sources name
// the same alias and file path. A source pinned with alias@branch
// matches only on that branch, the same rule quirk guardrails apply.
// branch arrives resolved once per run: resolving it per target would
// shell out to git for every path in a multi-target context call.
func quirkSourcesFor(kbRoot, alias, targetPath, branch string) []string {
	if alias == "" {
		return nil
	}
	quirkDir := filepath.Join(kbRoot, "quirks")
	entries, err := os.ReadDir(quirkDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" {
			continue
		}
		data, err := readBounded(filepath.Join(quirkDir, e.Name()))
		if err != nil {
			continue
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(data), &meta) || meta.Type != "quirk" {
			continue
		}
		for _, src := range meta.Sources {
			if u, ok := resolve.ParseURI(src); ok && u.Scheme == "repo" && u.Alias == alias && uriSurfacePath(u.Path) == targetPath {
				if u.Branch != "" && u.Branch != branch {
					continue
				}
				out = append(out, "quirks/"+e.Name())
			}
		}
	}
	return out
}

// domainTerm is one domain entity title with its document.
type domainTerm struct {
	title, rel string
}

// domainTermList loads every domain entity once so multi-target runs
// do not re-walk the domain directory per target.
func domainTermList(kbRoot string) []domainTerm {
	domainDir := filepath.Join(kbRoot, "domain")
	var out []domainTerm
	_ = filepath.WalkDir(domainDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), "._") || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(data), &meta) || meta.Type != "domain-entity" || meta.Title == "" {
			return nil
		}
		out = append(out, domainTerm{title: meta.Title, rel: filepath.ToSlash(rel)})
		return nil
	})
	return out
}

// domainTermsIn lists cached domain terms whose title appears as a
// word in the given source text.
func domainTermsIn(terms []domainTerm, source []byte) []domainTerm {
	if len(source) == 0 {
		return nil
	}
	var out []domainTerm
	for _, t := range terms {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(t.title) + `\b`)
		if re.Match(source) {
			out = append(out, t)
		}
	}
	return out
}

// readTargetBounded reads a regular source file with the shared 4MB
// cap; symlinks and FIFOs are never opened.
func readTargetBounded(p string) []byte {
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := readBounded(p)
	if err != nil {
		return nil
	}
	return []byte(data)
}
