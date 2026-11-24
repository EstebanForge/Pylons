// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/sync"
)

type quirkDoc struct {
	path    string
	sources []string
}

// quirkDocs lists the top-level active quirk documents with parsed
// frontmatter. Archive subtrees are excluded: an archived quirk no
// longer guards anything.
func quirkDocs(kbRoot string) []quirkDoc {
	quirkDir := filepath.Join(kbRoot, "quirks")
	entries, err := os.ReadDir(quirkDir)
	if err != nil {
		return nil
	}
	var out []quirkDoc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" || strings.HasPrefix(e.Name(), "._") {
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
		out = append(out, quirkDoc{path: "quirks/" + e.Name(), sources: meta.Sources})
	}
	return out
}

// QuirkGuardrails returns the distinct, sorted quirk documents whose
// repo:// sources cover any context target. A source covers a target
// when the alias matches and the paths are equal, or the source path
// is a directory ancestor of the target path. A source pinned with
// alias@branch fires only on that branch. Only targets with project
// attribution (repo:// URIs and absolute paths inside tracked
// projects) can match: plain ledger-relative targets carry no alias
// by design, the same rule the collision walk applies.
func QuirkGuardrails(kbRoot string, targets []string) []string {
	kbRoot = resolve.EvalRoot(kbRoot)
	cfg, _ := config.Load(kbRoot)
	branch := ledgerBranch(kbRoot)
	norm := make([]normalizedTarget, 0, len(targets))
	for _, t := range targets {
		// A directory-shaped target with a trailing slash must parse as
		// the same path it names without the slash.
		norm = append(norm, normalizeCheckTarget(kbRoot, strings.TrimRight(t, "/"), cfg))
	}
	if len(norm) == 0 {
		return nil
	}
	var out []string
	for _, q := range quirkDocs(kbRoot) {
		if quirkCovers(q.sources, norm, branch) {
			out = append(out, q.path)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// ledgerBranch reads the checked-out branch from the git HEAD ref.
// No repository, a detached HEAD, or an unreadable HEAD yields "": a
// branch-pinned source can then never match, which fails closed.
func ledgerBranch(kbRoot string) string {
	gitDir, err := sync.GitDirOf(kbRoot)
	if err != nil {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(head))
	if b, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
		return b
	}
	return ""
}

func quirkCovers(sources []string, norm []normalizedTarget, branch string) bool {
	for _, raw := range sources {
		// Directory sources are written with a trailing slash more often
		// than not; the URI parser rejects it, so trim before parsing.
		u, ok := resolve.ParseURI(strings.TrimRight(raw, "/"))
		if !ok || u.Scheme != "repo" {
			continue
		}
		// A branch-pinned source is knowledge about one branch only.
		if u.Branch != "" && u.Branch != branch {
			continue
		}
		sp := strings.TrimSuffix(u.Path, "/")
		if sp == "" {
			continue
		}
		for _, nt := range norm {
			if nt.alias == "" || nt.alias != u.Alias {
				continue
			}
			if sp == nt.path || strings.HasPrefix(nt.path, sp+"/") {
				return true
			}
		}
	}
	return false
}
