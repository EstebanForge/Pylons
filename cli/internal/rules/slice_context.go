// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// SliceContract is one plan contract with its resolution state.
type SliceContract struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Status   string `json:"status"` // stable | unstable | editing
	Resolved bool   `json:"resolved"`
	Detail   string `json:"detail,omitempty"`
}

// SliceReport is the smallest context that completes one slice: the
// plan's contracts with resolution state, plus linked quirks and domain
// terms. Nothing else loads.
type SliceReport struct {
	Plan       string          `json:"plan"`
	Title      string          `json:"title"`
	Status     string          `json:"status"`
	Components []string        `json:"components"`
	Contracts  []SliceContract `json:"contracts"`
	Quirks     []string        `json:"quirks"`
	Terms      []string        `json:"terms"`
}

// SliceContext builds a SliceReport for one plan document.
func SliceContext(kbRoot, planArg string, allowExternal bool) (SliceReport, error) {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return SliceReport{}, err
	}
	abs := planArg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(kbRoot, planArg)
	}
	data, err := readManifest(abs)
	if err != nil {
		return SliceReport{}, fmt.Errorf("read plan: %w", err)
	}
	rel, err := filepath.Rel(kbRoot, abs)
	if err != nil {
		rel = planArg
	}
	rel = filepath.ToSlash(rel)
	content := string(data)
	var pf resolve.PlanFrontMatter
	if !resolve.ParsePlanFrontMatter(content, &pf) || pf.Type != "plan" {
		return SliceReport{}, fmt.Errorf("%s is not a valid plan document", rel)
	}
	rep := SliceReport{Plan: rel, Title: pf.Title, Status: pf.Status, Components: pf.Components}
	rep.Contracts = sliceContracts(kbRoot, rel, content, pf, cfg, allowExternal)
	rep.Quirks = sliceQuirks(kbRoot, pf.Components)
	rep.Terms = sliceTerms(kbRoot, content, rep.Contracts)
	// Empty stays [] in JSON: nil slices marshal as null and break
	// consumers of --json.
	if rep.Components == nil {
		rep.Components = []string{}
	}
	if rep.Contracts == nil {
		rep.Contracts = []SliceContract{}
	}
	if rep.Quirks == nil {
		rep.Quirks = []string{}
	}
	if rep.Terms == nil {
		rep.Terms = []string{}
	}
	return rep, nil
}

// sliceContracts resolves the plan's stable contracts and records the
// declared unstable and editing entries.
func sliceContracts(kbRoot, rel, content string, pf resolve.PlanFrontMatter, cfg config.Config, allowExternal bool) []SliceContract {
	var out []SliceContract
	extDir := filepath.Join(kbRoot, "extensions")
	for _, entry := range pf.Contract.Stable {
		if isBoolTrue(entry["external"]) {
			kindStr, _ := entry["kind"].(string)
			idStr, _ := entry["id"].(string)
			out = append(out, SliceContract{Kind: kindStr, ID: idStr, Status: "stable", Detail: "external: true"})
			continue
		}
		kindStr, _ := entry["kind"].(string)
		idStr, _ := entry["id"].(string)
		c := SliceContract{Kind: kindStr, ID: idStr, Status: "stable"}
		_, projRoot, exts, ref, reason := resolveProjectTarget(kbRoot, idStr, pf.Components, cfg)
		if projRoot == "" {
			c.Detail = reason
			out = append(out, c)
			continue
		}
		reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{
			Extensions:    exts,
			ExtDir:        extDir,
			Ref:           ref,
			AllowExternal: allowExternal,
		})
		v := reg.Resolve(projRoot, resolve.Kind(kindStr), idStr)
		c.Resolved = v.Resolved
		c.Detail = v.Detail
		out = append(out, c)
	}
	for _, list := range []struct {
		status string
		raw    []any
	}{{"unstable", pf.Contract.Unstable}, {"editing", pf.Contract.Editing}} {
		for _, raw := range list.raw {
			m, ok := raw.(map[string]any)
			if !ok {
				if gs, glob := extractGlobString(raw); glob {
					out = append(out, SliceContract{Kind: "glob", ID: gs, Status: list.status})
				}
				continue
			}
			kindStr, _ := m["kind"].(string)
			idStr, _ := m["id"].(string)
			out = append(out, SliceContract{Kind: kindStr, ID: idStr, Status: list.status})
		}
	}
	return out
}

// sliceQuirks lists quirk documents whose repo:// sources name one of
// the plan's component projects. Matching is alias-scoped by design:
// the slice orients on the whole component, while quirk guardrails
// are path-scoped per target. A source pinned with alias@branch
// lists only on that branch, the same rule guardrails apply.
func sliceQuirks(kbRoot string, components []string) []string {
	if len(components) == 0 {
		return nil
	}
	comps := make(map[string]bool, len(components))
	for _, c := range components {
		comps[c] = true
	}
	branch := ledgerBranch(kbRoot)
	var out []string
	for _, q := range quirkDocs(kbRoot) {
		for _, src := range q.sources {
			if u, ok := resolve.ParseURI(src); ok && u.Scheme == "repo" && comps[u.Alias] {
				if u.Branch != "" && u.Branch != branch {
					continue
				}
				out = append(out, q.path)
				break
			}
		}
	}
	return out
}

// sliceTerms lists domain entities whose title appears as a word in the
// plan body or its contract ids.
func sliceTerms(kbRoot, planContent string, contracts []SliceContract) []string {
	needles := resolve.WithoutFrontMatter(planContent)
	for _, c := range contracts {
		needles += "\n" + c.ID
	}
	domainDir := filepath.Join(kbRoot, "domain")
	var out []string
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
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(meta.Title) + `\b`)
		if re.MatchString(needles) {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}
