// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// RetroRecord is one structured review finding. The signature is the
// lens plus rule/contract/path quadruple; text, shape, and source are
// context. Producers append one JSON object per line.
type RetroRecord struct {
	Date     string `json:"date"`
	Lens     string `json:"lens"`
	Rule     string `json:"rule"`
	Contract string `json:"contract"`
	Path     string `json:"path"`
	Shape    string `json:"shape"`
	Text     string `json:"text"`
	Source   string `json:"source"`
}

// RetroCandidate is one promotion proposal: a signature seen Min or
// more times inside the window opens exactly one candidate.
type RetroCandidate struct {
	Signature string
	Count     int
	Shape     string
	Summary   string
	Scaffold  string
	Sources   []string
}

// RetroReport is the full retro outcome.
type RetroReport struct {
	Examined   int
	Signatures int
	Candidates []RetroCandidate
}

// RetroOptions carries the retro inputs. Window uses the rework window
// grammar; Min defaults to 2.
type RetroOptions struct {
	Findings []byte
	Window   string
	Min      int
	Shape    string
}

var retroShapes = map[string]bool{"convention": true, "falsifier": true, "lint-rule": true}

// ReviewRetro groups structured review findings by signature and opens
// one promotion candidate per signature that repeats Min or more times
// inside the window. Report-only: the artifact is a candidate patch a
// maintainer applies by commit; the engine never writes to main.
func ReviewRetro(opts RetroOptions) (RetroReport, error) {
	if opts.Shape != "" && !retroShapes[opts.Shape] {
		return RetroReport{}, fmt.Errorf("shape %q must be convention, falsifier, or lint-rule", opts.Shape)
	}
	window := opts.Window
	if strings.TrimSpace(window) == "" {
		window = "90d"
	}
	days, err := parseReworkWindow(window)
	if err != nil {
		return RetroReport{}, err
	}
	minCount := opts.Min
	if minCount <= 0 {
		minCount = 2
	}
	cutoff := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	rep := RetroReport{}
	type group struct {
		records []RetroRecord
		shapes  map[string]bool
	}
	groups := map[string]*group{}
	for _, line := range strings.Split(string(opts.Findings), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r RetroRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return RetroReport{}, fmt.Errorf("findings record: %w", err)
		}
		if r.Shape != "" && !retroShapes[r.Shape] {
			return RetroReport{}, fmt.Errorf("findings record shape %q must be convention, falsifier, or lint-rule", r.Shape)
		}
		day, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			return RetroReport{}, fmt.Errorf("findings record date %q: %w", r.Date, err)
		}
		if day.Before(cutoff) {
			continue
		}
		rep.Examined++
		key := r.Lens + "\x00" + r.Rule + "\x00" + r.Contract + "\x00" + r.Path
		// Free-text findings carry no structured identity; they
		// accumulate on a normalized-text slug instead, so two
		// different markers never share a signature.
		if r.Rule == "" && r.Contract == "" && r.Path == "" {
			key += "\x00" + harvestSlug(r.Text)
		}
		g := groups[key]
		if g == nil {
			g = &group{shapes: map[string]bool{}}
			groups[key] = g
		}
		g.records = append(g.records, r)
		g.shapes[r.Shape] = true
	}
	rep.Signatures = len(groups)

	type counted struct {
		key   string
		g     *group
		count int
	}
	var countedSorted []counted
	for k, g := range groups {
		if len(g.records) >= minCount {
			countedSorted = append(countedSorted, counted{k, g, len(g.records)})
		}
	}
	slices.SortFunc(countedSorted, func(a, b counted) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return strings.Compare(a.key, b.key)
	})
	today := time.Now().Format("2006-01-02")
	usedSlugs := map[string]int{}
	for _, c := range countedSorted {
		records := c.g.records
		slices.SortFunc(records, func(a, b RetroRecord) int { return strings.Compare(a.Date, b.Date) })
		latest := records[len(records)-1]
		shape := opts.Shape
		if shape == "" {
			shape = "convention"
			if c.g.shapes != nil && len(c.g.shapes) == 1 {
				for s := range c.g.shapes {
					if s != "" {
						shape = s
					}
				}
			}
		}
		// A falsifier with no contract has nothing to anchor the
		// wrong_if entry to; it falls back to a convention stub.
		if shape == "falsifier" && latest.Contract == "" {
			shape = "convention"
		}
		cand := RetroCandidate{
			Signature: retroOneLine(retroSignatureLabel(c.key)),
			Count:     c.count,
			Shape:     shape,
			Sources:   retroSources(records),
		}
		latest.Text = retroOneLine(latest.Text)
		cleanContract := retroOneLine(latest.Contract)
		cleanRule := retroOneLine(ruleOrNone(latest.Rule))
		cleanPath := retroOneLine(pathOrNone(latest.Path))
		cleanLens := retroOneLine(latest.Lens)
		switch shape {
		case "falsifier":
			cand.Summary = fmt.Sprintf("Append a wrong_if entry to contract %s:", cleanContract)
			cand.Scaffold = fmt.Sprintf("wrong_if: %q\n", latest.Text)
		case "lint-rule":
			cand.Summary = "Proposed lint rule for the catalog:"
			cand.Scaffold = fmt.Sprintf("Trigger: lens %s on %s %s\nFalsifier: %q\n", cleanLens, cleanRule, cleanPath, latest.Text)
		default:
			slug := "retrospective"
			if s := harvestSlug(latest.Text); s != "footgun" {
				slug = s
			}
			usedSlugs[slug]++
			if n := usedSlugs[slug]; n > 1 {
				slug = fmt.Sprintf("%s-%d", slug, n)
			}
			doc := strings.Join([]string{
				"---",
				fmt.Sprintf("title: %q", retroTitle(latest)),
				"type: convention",
				fmt.Sprintf("slug: conv.retrospective.%s", slug),
				"level: SHOULD",
				"status: proposed",
				"created: " + today,
				"updated: " + today,
				"owners:",
				"  - Pylons",
				"ticket: []",
				`applies_to: ["all"]`,
				"supersedes: []",
				`artifact: ""`,
				`description: ""`,
				"tags: [conventions]",
				"---",
				"",
				"## Rule",
				"",
				retroSentence(latest.Text),
				"",
				"## Rationale",
				"",
				fmt.Sprintf("A recurring review finding: the same signature appeared %d times across %s.", c.count, strings.Join(cand.Sources, ", ")),
				"",
			}, "\n")
			cand.Summary = fmt.Sprintf("conventions/proposals/conv-retrospective-%s.md", slug)
			cand.Scaffold = doc
		}
		rep.Candidates = append(rep.Candidates, cand)
	}
	return rep, nil
}

// retroSentence ends a rule line with one period, never two: text
// already carrying terminal punctuation passes through unchanged.
func retroSentence(s string) string {
	if s == "" {
		return "Unspecified finding."
	}
	if r := []rune(s); r[len(r)-1] == '.' || r[len(r)-1] == '!' || r[len(r)-1] == '?' {
		return s
	}
	return s + "."
}

// retroOneLine flattens a field to one artifact line: control
// characters dropped, every whitespace run collapsed to one space, so
// record fields cannot inject headings into the rendered candidate.
func retroOneLine(s string) string {
	return strings.Join(strings.Fields(report.CleanText(s)), " ")
}

func retroSignatureLabel(key string) string {
	parts := strings.Split(key, "\x00")
	labels := []string{"lens " + parts[0]}
	if parts[1] != "" {
		labels = append(labels, "rule "+parts[1])
	}
	if parts[2] != "" {
		labels = append(labels, "contract "+parts[2])
	}
	if parts[3] != "" {
		labels = append(labels, "path "+parts[3])
	}
	return strings.Join(labels, ", ")
}

func retroTitle(r RetroRecord) string {
	words := strings.Fields(r.Text)
	if len(words) > 8 {
		words = words[:8]
	}
	if len(words) == 0 {
		return "Recurring review finding"
	}
	return "Review: " + strings.Join(words, " ")
}

func retroSources(records []RetroRecord) []string {
	var out []string
	for _, r := range records {
		if r.Source != "" {
			out = append(out, retroOneLine(r.Source))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func ruleOrNone(rule string) string {
	if rule == "" {
		return "(no rule id)"
	}
	return rule
}

func pathOrNone(path string) string {
	if path == "" {
		return "(no path)"
	}
	return path
}
