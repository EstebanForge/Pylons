// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

// cmdUtility renders the knowledge-utility report to stdout. Default is
// a read-only score view; --prune archives uncited quirks past the
// weighted injection floor and --promote writes convention proposal
// stubs for repeatedly cited documents.
func cmdUtility(kbRoot string, prune, promote bool) int {
	rep, err := rules.ReviewUtility(rules.UtilityOptions{
		KBRoot:  kbRoot,
		Now:     time.Now(),
		Prune:   prune,
		Promote: promote,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: review utility: %v\n", err)
		return 2
	}
	var b strings.Builder
	if !rep.Telemetry {
		b.WriteString("# Knowledge utility\n\nNo injection telemetry yet: run `pylon context` or `pylon context --slice` and the delivery record starts.\n")
	}
	if len(rep.Scored) > 0 {
		fmt.Fprintf(&b, "# Knowledge utility (%d scored documents)\n\n", len(rep.Scored))
		fmt.Fprintf(&b, "%-40s %11s %10s %8s\n", "document", "injections", "citations", "score")
		for _, row := range rep.Scored {
			fmt.Fprintf(&b, "%-40s %11d %10d %8.2f\n", row.Path, row.Injections, row.Citations, row.Score)
		}
		b.WriteString("\n")
	} else if rep.Telemetry {
		b.WriteString("# Knowledge utility\n\nTelemetry present, nothing scored.\n")
	}
	if len(rep.PruneCandidates) > 0 {
		b.WriteString("## Prune candidates (uncited past the injection floor)\n\n")
		for _, p := range rep.PruneCandidates {
			fmt.Fprintf(&b, "- %s (run with --prune to move it to quirks/archive/)\n", p)
		}
		b.WriteString("\n")
	}
	if len(rep.PromoteCandidates) > 0 {
		b.WriteString("## Promote candidates (cited repeatedly, from more than one place)\n\n")
		for _, row := range rep.PromoteCandidates {
			fmt.Fprintf(&b, "- %s: %d citations across %s (run with --promote for a convention proposal)\n", row.Path, row.Citations, strings.Join(row.CitingFiles, ", "))
		}
		b.WriteString("\n")
	}
	for _, p := range rep.Pruned {
		fmt.Fprintf(&b, "pruned: %s -> quirks/archive/\n", p)
	}
	for _, p := range rep.Promoted {
		fmt.Fprintf(&b, "promoted: %s -> conventions/proposals/\n", p)
	}
	fmt.Print(b.String())
	return 0
}

// injectedFiles picks the distinct ledger documents a context run
// delivered, so the utility telemetry records exactly what reached the
// caller.
func injectedFiles(findings []report.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range findings {
		if !strings.HasSuffix(f.File, ".md") || seen[f.File] {
			continue
		}
		seen[f.File] = true
		out = append(out, f.File)
	}
	return out
}
