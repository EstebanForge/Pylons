// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/rules"
)

// cmdHarvest renders the pr-harvest candidate artifact to stdout. The
// engine never writes to main: a CI workflow captures this output as
// the reviewable patch and a maintainer applies it by hand.
func cmdHarvest(kbRoot, diffPath, metaPath string) int {
	if diffPath == "" {
		fmt.Fprintln(os.Stderr, "pylon: harvest pr requires --diff <file|->")
		return 2
	}
	diff, err := readAnnotationInput(diffPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: harvest: %v\n", err)
		return 2
	}
	var meta []byte
	if metaPath != "" {
		meta, err = readAnnotationInput(metaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: harvest: %v\n", err)
			return 2
		}
	}
	rep, err := rules.HarvestPR(kbRoot, diff, meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: harvest: %v\n", err)
		return 2
	}
	fmt.Print(renderHarvest(rep))
	return 0
}

func renderHarvest(rep rules.HarvestReport) string {
	var b strings.Builder
	b.WriteString("# Harvest candidates\n\n")
	if len(rep.Symbols) == 0 && len(rep.Quirks) == 0 {
		b.WriteString("No candidates: the diff added no exported symbols and no footgun markers.\n")
		return b.String()
	}
	byDoc := map[string][]rules.SymbolCandidate{}
	var docs []string
	for _, s := range rep.Symbols {
		if s.Package == "" {
			continue
		}
		doc := "packages/" + s.Package + ".md"
		if _, ok := byDoc[doc]; !ok {
			docs = append(docs, doc)
		}
		byDoc[doc] = append(byDoc[doc], s)
	}
	if len(docs) > 0 {
		b.WriteString("## exposes entries\n\n")
		for _, doc := range docs {
			b.WriteString("- " + doc + ":\n")
			for _, s := range byDoc[doc] {
				if s.Proposed {
					fmt.Fprintf(&b, "  - {kind: %s, id: %s}  # new in %s\n", s.Kind, s.ID, s.Path)
				}
			}
		}
		b.WriteString("\n")
	}
	if len(rep.Quirks) > 0 {
		b.WriteString("## quirk proposals\n\n")
		for _, q := range rep.Quirks {
			fmt.Fprintf(&b, "### quirks/proposals/%s.md\n(from %s)\n\n%s\n", q.Slug, q.Source, q.Body)
		}
	}
	if len(rep.Unattributed) > 0 {
		b.WriteString("## Unattributed\n\n")
		for _, u := range rep.Unattributed {
			fmt.Fprintf(&b, "- %s :: %s\n", u.Path, u.Name)
		}
		b.WriteString("\n")
	}
	if len(rep.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, w := range rep.Warnings {
			b.WriteString("- " + w + "\n")
		}
	}
	return b.String()
}
