// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

var expansionExts = map[string]bool{
	".go": true, ".rs": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".php": true, ".py": true, ".rb": true, ".cs": true, ".md": true,
	".yaml": true, ".yml": true, ".json": true, ".sql": true, ".toml": true,
}

// pathLikeToken reports whether a query token names a file or directory
// rather than a word.
func pathLikeToken(token string) bool {
	if strings.ContainsRune(token, '/') {
		return true
	}
	return expansionExts[strings.ToLower(filepath.Ext(token))]
}

// expandIndexTerms appends one-hop domain vocabulary to each document's
// index terms: entity titles and tags linked from the doc's frontmatter
// or named in its prose. No recursion: only declared entities feed the
// pass, never terms discovered from the appended text.
func expandIndexTerms(docs []docEntry) {
	type entity struct {
		path  string
		title string
		tags  []string
	}
	byPath := map[string]entity{}
	byTitle := map[string]entity{}
	for _, d := range docs {
		if d.Type != "domain-entity" {
			continue
		}
		e := entity{path: d.Path, title: d.Title, tags: d.Tags}
		byPath[d.Path] = e
		byTitle[strings.ToLower(d.Title)] = e
	}
	if len(byTitle) == 0 {
		return
	}
	for i := range docs {
		d := &docs[i]
		bodyTokens := docTokens(resolve.WithoutFrontMatter(d.Body))
		linked := map[string]entity{}
		for _, l := range d.Links {
			target := l
			if _, rest, ok := strings.Cut(target, "://"); ok {
				target = rest
			}
			if head, _, ok := strings.Cut(target, "::"); ok {
				target = head
			}
			if e, ok := byPath[target]; ok {
				linked[e.title] = e
				continue
			}
			if _, base := splitPathTail(target); base != "" {
				if e, ok := byPath["domain/"+base]; ok {
					linked[e.title] = e
				}
			}
		}
		for _, e := range byTitle {
			if e.path == d.Path {
				continue
			}
			if containsTokenSeq(bodyTokens, docTokens(e.title)) {
				linked[e.title] = e
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

func splitPathTail(p string) (dir, base string) {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

// containsTokenSeq reports whether the token sequence appears as a
// contiguous run in the body tokens (word-boundary phrase match).
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

// expandQueryTerms resolves path and package tokens against the ledger
// and returns the deterministic expansion set: owning project aliases,
// owning package names with their components, and dependency-graph
// neighbor packages at depth one. Sorted and deduped.
func expandQueryTerms(kbRoot string, cfg config.Config, docs []docEntry, tokens []string) []string {
	terms := map[string]bool{}
	owningPkgs := map[string]bool{}
	for _, t := range tokens {
		alias := ""
		if pathLikeToken(t) {
			abs := t
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(kbRoot, t)
			}
			if a, _, ok := rules.ResolveCheckTarget(kbRoot, abs, cfg); ok {
				alias = a
			}
		} else if cfg.Projects[t].Path != "" || cfg.Projects[t].Repo != "" {
			alias = t
		}
		if alias == "" {
			continue
		}
		terms[alias] = true
		for _, d := range docs {
			if d.Type != "package" {
				continue
			}
			if slices.Contains(d.Components, alias) || d.Package == alias {
				terms[d.Package] = true
				owningPkgs[d.Package] = true
				for _, c := range d.Components {
					terms[c] = true
				}
			}
		}
	}
	for pkg := range owningPkgs {
		report, err := rules.DependencyGraph(kbRoot, pkg, 1)
		if err != nil {
			continue
		}
		for _, n := range report.Nodes {
			if n.Package != pkg && n.Package != "" {
				terms[n.Package] = true
			}
		}
	}
	out := slices.Sorted(maps.Keys(terms))
	return out
}
