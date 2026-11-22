// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/rules"
)

// cmdRetro renders the review-retro promotion artifact to stdout.
// Report-only: candidates are proposals a maintainer applies by hand.
func cmdRetro(findingsPath, window string, min int, shape string) int {
	if findingsPath == "" {
		fmt.Fprintln(os.Stderr, "pylon: review retro requires --findings <file|->")
		return 2
	}
	data, err := readAnnotationInput(findingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: review retro: %v\n", err)
		return 2
	}
	rep, err := rules.ReviewRetro(rules.RetroOptions{Findings: data, Window: window, Min: min, Shape: shape})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: review retro: %v\n", err)
		return 2
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Review retro\n\n%d findings, %d signatures", rep.Examined, rep.Signatures)
	if len(rep.Candidates) == 0 {
		b.WriteString(", no repeats past the threshold.\n")
		fmt.Print(b.String())
		return 0
	}
	fmt.Fprintf(&b, ", %d promotion candidates:\n\n", len(rep.Candidates))
	for _, c := range rep.Candidates {
		fmt.Fprintf(&b, "## %s (%d repeats)\n\nShape: %s\n\n%s\n\n%s\n", c.Signature, c.Count, c.Shape, c.Summary, strings.TrimRight(c.Scaffold, "\n"))
		b.WriteString("\n")
	}
	fmt.Print(b.String())
	return 0
}
