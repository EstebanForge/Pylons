// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

type corpusDoc struct {
	rel     string
	content string
	meta    resolve.DocMeta
}

func scanGraphRules(kbRoot string, docs []corpusDoc) []report.Finding {
	var findings []report.Finding
	findings = append(findings, checkOrphanDocs(kbRoot, docs)...)
	findings = append(findings, checkUnlinkedDomainTerms(docs)...)
	findings = append(findings, checkPackageDependencies(docs)...)
	findings = append(findings, checkSupersessionLinks(kbRoot, docs)...)
	findings = append(findings, checkFolderEntryREADMEs(kbRoot, docs)...)
	return findings
}

func checkOrphanDocs(kbRoot string, docs []corpusDoc) []report.Finding {
	linked := make(map[string]bool)
	// Seed links from root README.md and AGENTS.md plus all docs.
	scanLinksFrom := func(baseDir, content string) {
		for _, m := range mdLinkRe.FindAllStringSubmatch(content, -1) {
			target := m[1]
			if strings.Contains(target, "://") {
				continue
			}
			abs := filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(target)))
			if rel, err := filepath.Rel(kbRoot, abs); err == nil {
				linked[filepath.ToSlash(rel)] = true
			}
		}
	}
	for _, rootDoc := range []string{"README.md", "AGENTS.md"} {
		if raw, err := os.ReadFile(filepath.Join(kbRoot, rootDoc)); err == nil {
			scanLinksFrom(kbRoot, string(raw))
		}
	}
	for _, d := range docs {
		scanLinksFrom(filepath.Dir(filepath.Join(kbRoot, filepath.FromSlash(d.rel))), d.content)
		for _, l := range d.meta.Links {
			if u, ok := resolve.ParseURI(l); ok && u.Scheme == "pylon" {
				linked[u.Path] = true
			} else {
				linked[strings.TrimPrefix(l, "./")] = true
			}
		}
	}

	var out []report.Finding
	for _, d := range docs {
		base := filepath.Base(d.rel)
		if base == "README.md" || base == "AGENTS.md" || strings.Contains(d.rel, "/archive/") {
			continue
		}
		if strings.HasPrefix(d.rel, "packages/") || strings.HasPrefix(d.rel, "conventions/") || strings.HasPrefix(d.rel, "domain/") {
			continue
		}
		if !linked[d.rel] {
			out = append(out, report.Finding{
				RuleID:   "PYLON004",
				Severity: report.SeverityError,
				Grade:    report.GradeForced,
				File:     d.rel,
				Line:     1,
				Message:  "orphan document: no incoming markdown or frontmatter link",
			})
		}
	}
	return out
}

func checkUnlinkedDomainTerms(docs []corpusDoc) []report.Finding {
	var terms []struct {
		title string
		rel   string
	}
	for _, d := range docs {
		if strings.HasPrefix(d.rel, "domain/") && d.meta.Title != "" {
			terms = append(terms, struct {
				title string
				rel   string
			}{title: d.meta.Title, rel: d.rel})
		}
	}
	if len(terms) == 0 {
		return nil
	}

	var out []report.Finding
	for _, d := range docs {
		if strings.HasPrefix(d.rel, "domain/") || strings.Contains(d.rel, "/archive/") {
			continue
		}
		body := resolve.WithoutFrontMatter(d.content)
		for _, term := range terms {
			if len(term.title) < 4 {
				continue
			}
			if strings.Contains(body, term.title) && !strings.Contains(body, "["+term.title+"]") && !strings.Contains(body, filepath.Base(term.rel)) {
				out = append(out, report.Finding{
					RuleID:   "PYLON005",
					Severity: report.SeverityError,
					Grade:    report.GradeForced,
					File:     d.rel,
					Line:     1,
					Message:  "unlinked domain term " + term.title + " (link to " + term.rel + ")",
				})
			}
		}
	}
	return out
}

func checkPackageDependencies(docs []corpusDoc) []report.Finding {
	known := make(map[string]bool)
	var pkgs []struct {
		rel string
		fm  resolve.PackageFrontMatter
	}
	for _, d := range docs {
		if !strings.HasPrefix(d.rel, "packages/") || strings.Contains(d.rel, "/archive/") {
			continue
		}
		var fm resolve.PackageFrontMatter
		if resolve.ParsePackageFrontMatter(d.content, &fm) && fm.Type == "package" {
			name := fm.Package
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(d.rel), ".md")
			}
			known[name] = true
			pkgs = append(pkgs, struct {
				rel string
				fm  resolve.PackageFrontMatter
			}{rel: d.rel, fm: fm})
		}
	}

	var out []report.Finding
	for _, p := range pkgs {
		for _, dep := range p.fm.DependsOn {
			if !known[dep] {
				out = append(out, errFinding("PYLON013", p.rel, 1, "depends_on references unknown package: "+dep))
			}
		}
	}
	return out
}

func checkSupersessionLinks(kbRoot string, docs []corpusDoc) []report.Finding {
	var out []report.Finding
	for _, d := range docs {
		dir := filepath.Dir(filepath.Join(kbRoot, filepath.FromSlash(d.rel)))
		for _, sup := range d.meta.Supersedes {
			sup = strings.TrimSpace(sup)
			if sup == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sup))); err == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, "archive", filepath.FromSlash(sup))); err == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(kbRoot, filepath.FromSlash(sup))); err == nil {
				continue
			}
			out = append(out, errFinding("PYLON019", d.rel, 1, "supersedes target not found: "+sup))
		}
	}
	return out
}

func checkFolderEntryREADMEs(kbRoot string, docs []corpusDoc) []report.Finding {
	zones := make(map[string]bool)
	for _, d := range docs {
		zone, _, ok := strings.Cut(d.rel, "/")
		if ok && !strings.Contains(d.rel, "/archive/") {
			zones[zone] = true
		}
	}
	var out []report.Finding
	for zone := range zones {
		if _, err := os.Stat(filepath.Join(kbRoot, zone, "AGENTS.md")); err != nil {
			out = append(out, report.Finding{
				RuleID:   "PYLON034",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     zone + "/",
				Message:  "zone folder has documents but no AGENTS.md entry point",
			})
		}
	}
	return out
}
