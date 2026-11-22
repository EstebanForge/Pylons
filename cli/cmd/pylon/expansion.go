// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

// Expansion adds at most this many terms to a query so rare component
// and neighbor names cannot flood the BM25 ranking.
const expansionTermCap = 8

var expansionExts = map[string]bool{
	".go": true, ".rs": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".php": true, ".py": true, ".rb": true, ".cs": true, ".md": true,
	".yaml": true, ".yml": true, ".json": true, ".sql": true, ".toml": true,
}

// pathLikeToken reports whether a raw query field names a file or
// directory rather than a word.
func pathLikeToken(field string) bool {
	if strings.ContainsRune(field, '/') {
		return true
	}
	return expansionExts[strings.ToLower(filepath.Ext(field))]
}

// expandIndexTerms appends one-hop domain vocabulary to each document's
// index terms: entity titles and tags cited by an exact frontmatter
// link or named in the doc's prose. Titles shorter than 4 characters
// are exempt, mirroring the term-link rule, and entity docs never
// grant vocabulary to each other. No recursion: only declared entities
// feed the pass, never terms discovered from the appended text.
func expandIndexTerms(docs []docEntry) {
	type entity struct {
		path   string
		title  string
		tags   []string
		phrase []string
	}
	byPath := map[string]entity{}
	var entities []entity
	for _, d := range docs {
		if d.Type != "domain-entity" || len(d.Title) < 4 {
			continue
		}
		e := entity{path: d.Path, title: d.Title, tags: d.Tags, phrase: docTokens(d.Title)}
		if len(e.phrase) == 0 {
			continue
		}
		byPath[d.Path] = e
		entities = append(entities, e)
	}
	if len(entities) == 0 {
		return
	}
	for i := range docs {
		d := &docs[i]
		linked := map[string]entity{}
		for _, l := range d.Links {
			target := l
			if _, rest, ok := strings.Cut(target, "://"); ok {
				target = rest
			}
			if head, _, ok := strings.Cut(target, "::"); ok {
				target = head
			}
			if head, _, ok := strings.Cut(target, "#"); ok {
				target = head
			}
			if e, ok := byPath[strings.TrimSuffix(target, "/")]; ok {
				linked[e.title] = e
			}
		}
		if d.Type != "domain-entity" {
			bodyTokens := docTokens(resolve.WithoutFrontMatter(d.Body))
			for _, e := range entities {
				if e.path == d.Path {
					continue
				}
				if containsTokenSeq(bodyTokens, e.phrase) {
					linked[e.title] = e
				}
			}
		}
		if len(linked) == 0 {
			continue
		}
		for _, e := range linked {
			d.IndexTerms = append(d.IndexTerms, e.title)
			d.IndexTerms = append(d.IndexTerms, e.tags...)
		}
		slices.Sort(d.IndexTerms)
		d.IndexTerms = slices.Compact(d.IndexTerms)
	}
}

// containsTokenSeq reports whether the phrase appears as a contiguous
// token run in the body tokens (word-boundary phrase match).
func containsTokenSeq(body, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(body) {
		return false
	}
	for i := 0; i+len(phrase) <= len(body); i++ {
		match := true
		for j, w := range phrase {
			if body[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// expandQueryTerms resolves raw query fields against the ledger and
// returns the deterministic expansion set: owning project aliases,
// owning package names with their components, and dependency-graph
// neighbor packages at depth one. Sorted, deduped, capped.
func expandQueryTerms(kbRoot string, cfg config.Config, docs []docEntry, fields []string) []string {
	terms := map[string]bool{}
	owningPkgs := map[string]bool{}
	pkgByName := map[string]docEntry{}
	for _, d := range docs {
		if d.Type == "package" {
			pkgByName[d.Package] = d
		}
	}
	for _, field := range fields {
		alias := ""
		if pathLikeToken(field) {
			alias = resolvePathAlias(kbRoot, cfg, field)
		} else if _, ok := cfg.Projects[field]; ok {
			alias = field
		}
		if alias != "" {
			terms[alias] = true
		}
		// A field naming a package directly expands that package.
		if pd, ok := pkgByName[field]; ok {
			terms[pd.Package] = true
			owningPkgs[pd.Package] = true
		}
		for _, d := range docs {
			if d.Type != "package" {
				continue
			}
			if alias != "" && (slices.Contains(d.Components, alias) || d.Package == alias) {
				terms[d.Package] = true
				owningPkgs[d.Package] = true
			}
			if alias != "" || d.Package == field {
				for _, c := range d.Components {
					terms[c] = true
				}
			}
		}
	}
	for pkg := range owningPkgs {
		report, err := rules.DependencyGraph(kbRoot, pkg, 1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: dependency graph for %s unavailable: %v\n", pkg, err)
			continue
		}
		for _, n := range report.Nodes {
			if n.Package != pkg && n.Package != "" {
				terms[n.Package] = true
			}
		}
	}
	out := slices.Sorted(maps.Keys(terms))
	if len(out) > expansionTermCap {
		out = out[:expansionTermCap]
	}
	return out
}

// resolvePathAlias tries a query path in the caller's resolution order:
// repo:// URIs raw (a join would mangle the scheme), then absolute from
// the current directory (query paths are relative to the caller, not
// the ledger), then KB-root-relative.
func resolvePathAlias(kbRoot string, cfg config.Config, field string) string {
	if strings.HasPrefix(field, "repo://") {
		if a, _, ok := rules.ResolveCheckTarget(kbRoot, field, cfg); ok {
			return a
		}
		return ""
	}
	candidates := make([]string, 0, 2)
	if abs, err := filepath.Abs(field); err == nil {
		candidates = append(candidates, abs)
	}
	if !filepath.IsAbs(field) {
		candidates = append(candidates, filepath.Join(kbRoot, field))
	}
	for _, cand := range candidates {
		if a, _, ok := rules.ResolveCheckTarget(kbRoot, cand, cfg); ok {
			return a
		}
	}
	return ""
}
