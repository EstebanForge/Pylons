// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// StaleSurface is one declared surface the diff touched while its
// declaring document did not move in the same commit set.
type StaleSurface struct {
	Alias string `json:"alias"`
	Path  string `json:"path"`
	Doc   string `json:"doc"`
	Kind  string `json:"kind"`
}

// CaptureManifest is the session-close manifest: touched surfaces whose
// docs lagged, plus the projects that could not be judged. The engine
// reports drift; the prose stays with the agent.
type CaptureManifest struct {
	Since    string         `json:"since"`
	Examined int            `json:"examined"`
	Surfaces []StaleSurface `json:"surfaces"`
	Warnings []string       `json:"warnings"`
}

// SessionCapture compares git history since ref against the surfaces
// declared in packages/ and plans/: a surface's code path touched in
// the diff (committed or not, ref against the working tree) whose
// declaring KB document did not move in the same range lands in the
// manifest. Projects whose repository cannot be read are skipped and
// reported in Warnings.
func SessionCapture(kbRoot, since string) (CaptureManifest, error) {
	manifest := CaptureManifest{Since: since, Surfaces: []StaleSurface{}, Warnings: []string{}}
	if strings.TrimSpace(since) == "" {
		return manifest, fmt.Errorf("an empty ref names no history")
	}
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return manifest, err
	}
	kbChanged, err := diffSincePaths(kbRoot, since)
	if err != nil {
		manifest.Warnings = append(manifest.Warnings, "the pylon repository could not be diffed; every declared doc reads as not updated")
		kbChanged = nil
	}
	declared := captureSurfaces(kbRoot)
	manifest.Examined = len(declared)
	aliases := slices.Sorted(maps.Keys(declared))
	for _, alias := range aliases {
		projPath := cfg.ProjectPath(kbRoot, alias)
		if projPath == "" {
			continue
		}
		paths, err := diffSincePaths(projPath, since)
		if err != nil {
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("project %q could not be diffed; its surfaces are unjudged", alias))
			continue
		}
		touched := make(map[string]bool, len(paths))
		for _, p := range paths {
			touched[p] = true
		}
		seen := map[string]bool{}
		for _, surface := range declared[alias] {
			// One manifest entry per surface path: an expose and a
			// source naming the same file drift together.
			if seen[surface.Path] || !touched[surface.Path] || slices.Contains(kbChanged, surface.Doc) {
				continue
			}
			seen[surface.Path] = true
			manifest.Surfaces = append(manifest.Surfaces, StaleSurface{
				Alias: alias,
				Path:  surface.Path,
				Doc:   surface.Doc,
				Kind:  surface.Kind,
			})
		}
	}
	return manifest, nil
}

// diffSincePaths lists the files changed between ref and the working
// tree, relative to the given directory. Uncommitted edits count: at
// session close both code and doc changes are usually unstaged.
func diffSincePaths(dir, since string) ([]string, error) {
	out, err := resolve.GitRun(dir, nil, "diff", "--name-only", "--relative", since)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, filepath.ToSlash(line))
		}
	}
	return paths, nil
}

type captureSurface struct {
	Path string
	Doc  string
	Kind string
}

// captureSurfaces indexes the declared surfaces by project alias:
// package exposes and sources, plus accepted and in-progress plan
// contracts. Glob ids name whole trees, so no single path proves them
// stale and they are out of scope here. Doc paths are real KB-relative
// paths, never reconstructed from the walk.
func captureSurfaces(kbRoot string) map[string][]captureSurface {
	out := map[string][]captureSurface{}
	seen := map[string]bool{}
	add := func(alias, path, doc, kind string) {
		if alias == "" || path == "" {
			return
		}
		if seen[alias+":"+path] {
			return
		}
		seen[alias+":"+path] = true
		out[alias] = append(out[alias], captureSurface{Path: path, Doc: doc, Kind: kind})
	}
	walk := func(dir string, fn func(content, doc string) error) {
		_ = filepath.WalkDir(filepath.Join(kbRoot, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
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
			rel, err := filepath.Rel(kbRoot, path)
			if err != nil {
				return nil
			}
			return fn(string(data), filepath.ToSlash(rel))
		})
	}
	walk("packages", func(content, doc string) error {
		var pkg resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(content, &pkg) || pkg.Type != "package" {
			return nil
		}
		for _, entry := range pkg.Exposes {
			kindStr, _ := entry["kind"].(string)
			idStr, _ := entry["id"].(string)
			if u, ok := resolve.ParseURI(idStr); ok && u.Scheme == "repo" && !strings.ContainsAny(u.Path, "*?[") {
				add(u.Alias, uriSurfacePath(u.Path), doc, kindOr(kindStr))
			}
		}
		for _, raw := range pkg.Sources {
			if u, ok := resolve.ParseURI(raw); ok && u.Scheme == "repo" && !strings.ContainsAny(u.Path, "*?[") {
				add(u.Alias, uriSurfacePath(u.Path), doc, "source")
			}
		}
		return nil
	})
	walk("plans", func(content, doc string) error {
		var pf resolve.PlanFrontMatter
		if !resolve.ParsePlanFrontMatter(content, &pf) || pf.Type != "plan" {
			return nil
		}
		if pf.Status != "accepted" && pf.Status != "in-progress" {
			return nil
		}
		for _, entry := range pf.Contract.Stable {
			kindStr, _ := entry["kind"].(string)
			idStr, _ := entry["id"].(string)
			if u, ok := resolve.ParseURI(idStr); ok && u.Scheme == "repo" && !strings.ContainsAny(u.Path, "*?[") {
				add(u.Alias, uriSurfacePath(u.Path), doc, kindOr(kindStr))
			}
		}
		return nil
	})
	return out
}

// kindOr reduces a language-prefixed kind to its base kind.
func kindOr(kind string) string {
	if i := strings.LastIndex(kind, "-"); i >= 0 {
		return kind[i+1:]
	}
	return kind
}
