// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/rules"
)

// cmdHarvest renders the pr-harvest candidate artifact to stdout. The
// engine never writes to main: a CI workflow captures this output as
// the reviewable patch and a maintainer applies it by hand. With
// --record, each Footgun marker is also appended to a JSONL findings
// file that `review retro` consumes.
func cmdHarvest(kbRoot, diffPath, metaPath, recordPath string) int {
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
	if recordPath != "" && len(rep.Quirks) > 0 {
		if err := appendRetroRecords(recordPath, rep.Quirks); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: harvest: %v\n", err)
			return 2
		}
	}
	fmt.Print(renderHarvest(rep))
	return 0
}

// appendRetroRecords appends one JSONL finding per Footgun marker so a
// recurring marker crosses the retro threshold over later merges.
func appendRetroRecords(path string, quirks []rules.QuirkCandidate) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	today := time.Now().Format("2006-01-02")
	for _, q := range quirks {
		rec := rules.RetroRecord{Date: today, Lens: "review", Text: q.Text, Source: q.Source}
		line, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
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
