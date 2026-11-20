// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

var (
	// Branch grammar matches ParseURI: no slashes in branch names.
	branchMentionRe = regexp.MustCompile(`repo://([a-zA-Z0-9._-]+)@([a-zA-Z0-9._-]+)/`)
	quirkGuards     = map[string]bool{
		"lint-rule":     true,
		"convention":    true,
		"test":          true,
		"accepted-risk": true,
	}
)

func scanPlanAndQuirkRules(kbRoot string, docs []corpusDoc, cfg config.Config, opts LintOptions) []report.Finding {
	var out []report.Finding
	for _, d := range docs {
		if strings.Contains(d.rel, "/archive/") {
			continue
		}
		if opts.HasCode {
			out = append(out, checkStaleBranchMentions(kbRoot, d, cfg)...)
		}

		var pf resolve.PlanFrontMatter
		if resolve.ParsePlanFrontMatter(d.content, &pf) && pf.Type == "plan" {
			out = append(out, checkPlanEvidenceAndFalsifiers(kbRoot, d.rel, pf, cfg, opts)...)
			out = append(out, checkSlicePlan(pf, d.rel)...)
			out = append(out, checkScenarioCoverage(pf, d.rel)...)
		} else if strings.HasPrefix(d.content, "---\n") {
			// A plan doc whose plan frontmatter cannot parse must
			// fail loudly: silently dropping it would skip every
			// plan rule, collision check, and ripple.
			var probe resolve.DocMeta
			if resolve.ParseDocMeta(d.content, &probe) && probe.Type == "plan" {
				out = append(out, errFinding("PYLON001", d.rel, 1, "plan frontmatter cannot parse; plan rules, collisions, and ripple are skipped for this document"))
			}
		}

		var pkg resolve.PackageFrontMatter
		if resolve.ParsePackageFrontMatter(d.content, &pkg) && pkg.Type == "package" {
			out = append(out, checkPackageExtended(kbRoot, d.rel, pkg, cfg, opts)...)
		}

		if strings.HasPrefix(d.rel, "quirks/") {
			out = append(out, checkQuirkRules(d)...)
		}
	}
	return out
}

func checkPlanEvidenceAndFalsifiers(kbRoot, rel string, pf resolve.PlanFrontMatter, cfg config.Config, opts LintOptions) []report.Finding {
	var out []report.Finding
	if pf.Status == "accepted" && len(pf.Ticket) == 0 {
		out = append(out, errFinding("PYLON016", rel, 1, "accepted plan must cite at least one ticket: entry"))
	}

	if pf.Status == "accepted" || pf.Status == "in-progress" {
		for _, entry := range pf.Contract.Stable {
			wrongIf, _ := entry["wrong_if"].(string)
			if strings.TrimSpace(wrongIf) == "" {
				id, _ := entry["id"].(string)
				out = append(out, errFinding("PYLON014", rel, 1, "contract.stable entry "+id+" missing wrong_if: falsifier"))
			}
			if provableContractKind(entry) && !hasProofPointer(entry) {
				id, _ := entry["id"].(string)
				out = append(out, advFinding("PYLON040", rel, "contract.stable entry "+id+" carries no verified_by: or falsified_by: proof pointer"))
			}
			if opts.HasCode {
				for _, field := range []string{"verified_by", "falsified_by"} {
					if ref, _ := entry[field].(string); strings.TrimSpace(ref) != "" {
						id, _ := entry["id"].(string)
						// A repo:// test ref names its own project; a bare
						// file::symbol falls back to the contract's project.
						targetID := id
						if u, ok := resolve.ParseURI(ref); ok && u.Scheme == "repo" {
							targetID = "repo://" + u.Alias
						}
						_, projRoot, _, baseRef, _ := resolveProjectTarget(kbRoot, targetID, pf.Components, cfg)
						if projRoot != "" {
							reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{Ref: baseRef})
							if v := reg.Resolve(projRoot, resolve.KindSymbol, ref); !v.Resolved {
								out = append(out, errFinding("PYLON035", rel, 1, field+" target did not resolve: "+ref))
							}
						}
					}
				}
			}
		}

		if opts.HasCode {
			for _, list := range [][]any{pf.Contract.Editing, pf.Contract.Unstable} {
				for _, raw := range list {
					g, ok := extractGlobString(raw)
					if !ok {
						continue
					}
					_, projRoot, _, baseRef, _ := resolveProjectTarget(kbRoot, g, pf.Components, cfg)
					if projRoot != "" {
						reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{Ref: baseRef})
						if v := reg.Resolve(projRoot, resolve.KindGlob, g); !v.Resolved {
							out = append(out, errFinding("PYLON032", rel, 1, "inert contract glob matches zero files: "+g))
						}
					}
				}
			}
		}
	}

	if pf.Status == "in-progress" && !pf.Artifact.NonEmpty() {
		slug := strings.TrimSuffix(filepath.Base(rel), ".md")
		if !hasMatchingBranch(kbRoot, slug, pf.Components, cfg) {
			out = append(out, report.Finding{
				RuleID:   "PYLON027",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     rel,
				Line:     1,
				Message:  "in-progress plan has no artifact: and no matching branch in registered projects",
			})
		}
	}
	return out
}

// provableKinds are the contract kinds whose proof is a test in the
// mapped repo; language-prefixed external kinds (go-symbol, ts-export)
// count by their suffix after the last dash, so two-dash kinds split
// correctly.
var provableKinds = map[string]bool{
	"symbol": true, "route": true, "cli": true, "event": true, "schema": true,
	// Built-in extension kinds: ts-export, js-export, rust-item.
	"export": true, "item": true,
}

// provableContractKind reports whether the entry's kind belongs to the
// provable set, ignoring a language-extension prefix.
func provableContractKind(entry map[string]any) bool {
	kind, _ := entry["kind"].(string)
	if dash := strings.LastIndex(kind, "-"); dash >= 0 {
		return provableKinds[kind[dash+1:]]
	}
	return provableKinds[kind]
}

// hasProofPointer reports whether the entry names a proving or
// falsifying test. Resolution of the pointer itself is PYLON035.
func hasProofPointer(entry map[string]any) bool {
	for _, field := range []string{"verified_by", "falsified_by"} {
		if ref, _ := entry[field].(string); strings.TrimSpace(ref) != "" {
			return true
		}
	}
	return false
}

func checkPackageExtended(kbRoot, rel string, pkg resolve.PackageFrontMatter, cfg config.Config, opts LintOptions) []report.Finding {
	if !opts.HasCode {
		return nil
	}
	var out []report.Finding
	extDir := filepath.Join(kbRoot, "extensions")
	for _, entry := range pkg.Exposes {
		if !isBoolTrue(entry["pending"]) {
			continue
		}
		kindStr, _ := entry["kind"].(string)
		idStr, _ := entry["id"].(string)
		_, projRoot, exts, baseRef, _ := resolveProjectTarget(kbRoot, idStr, []string{pkg.Package}, cfg)
		if projRoot == "" {
			continue
		}
		if pkg.Ref != "" {
			baseRef = pkg.Ref
		}
		reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{
			Extensions:    exts,
			ExtDir:        extDir,
			Ref:           baseRef,
			AllowExternal: opts.AllowExternal,
		})
		if v := reg.Resolve(projRoot, resolve.Kind(kindStr), idStr); v.Resolved {
			out = append(out, report.Finding{
				RuleID:   "PYLON024",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     rel,
				Line:     1,
				Message:  "pending exposes entry already resolves in code; remove pending: true (" + idStr + ")",
			})
		}
	}
	return out
}

func checkQuirkRules(d corpusDoc) []report.Finding {
	var out []report.Finding
	guard := strings.TrimSpace(d.meta.Guard)
	if guard != "" && !quirkGuards[guard] {
		out = append(out, errFinding("PYLON028", d.rel, 1, "invalid quirk guard: "+guard+" (use lint-rule|convention|test|accepted-risk)"))
	} else if guard == "" && (d.meta.Status == "" || d.meta.Status == "active") {
		if created, err := time.Parse("2006-01-02", strings.TrimSpace(d.meta.Created)); err == nil {
			if time.Since(created) > 30*24*time.Hour {
				out = append(out, report.Finding{
					RuleID:   "PYLON028",
					Severity: report.SeverityAdvisory,
					Grade:    report.GradeAdvisory,
					File:     d.rel,
					Line:     1,
					Message:  fmt.Sprintf("active quirk created %s has no guard: after 30 days", d.meta.Created),
				})
			}
		}
	}

	var qm struct {
		Contracts []map[string]any `yaml:"contracts"`
	}
	if parseYAMLFrontMatter(d.content, &qm) {
		for _, c := range qm.Contracts {
			k, _ := c["kind"].(string)
			id, _ := c["id"].(string)
			if strings.TrimSpace(k) == "" || strings.TrimSpace(id) == "" {
				out = append(out, errFinding("PYLON036", d.rel, 1, "quirk contracts: entry requires non-empty kind and id"))
			}
		}
	}
	return out
}

func checkStaleBranchMentions(kbRoot string, d corpusDoc, cfg config.Config) []report.Finding {
	var out []report.Finding
	for _, m := range branchMentionRe.FindAllStringSubmatch(d.content, -1) {
		alias, branch := m[1], m[2]
		projRoot := cfg.ProjectPath(kbRoot, alias)
		if projRoot == "" {
			continue
		}
		_, err := resolve.GitRun(projRoot, nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
		if err != nil {
			out = append(out, report.Finding{
				RuleID:   "PYLON037",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     d.rel,
				Line:     1,
				Message:  fmt.Sprintf("branch %s in repo://%s@%s not found locally", branch, alias, branch),
			})
		}
	}
	return out
}

func hasMatchingBranch(kbRoot, slug string, components []string, cfg config.Config) bool {
	for _, c := range components {
		projRoot := cfg.ProjectPath(kbRoot, c)
		if projRoot == "" {
			continue
		}
		out, err := resolve.GitRun(projRoot, nil, "branch", "--list", "*"+slug+"*")
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return true
		}
	}
	return false
}
