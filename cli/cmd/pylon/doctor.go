// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/doctor"
	"github.com/estebanforge/pylons/cli/internal/report"
)

// cmdDoctor audits ledger structure; exit 1 means error-grade findings
// remain (after --fix if given), mirroring the findings exit contract.
func cmdDoctor(kbRoot string, fix bool, format report.Format) int {
	if format != report.FormatHuman && format != report.FormatJSON {
		fmt.Fprintf(os.Stderr, "pylon: doctor accepts --format human|json\n")
		return 2
	}
	if fix {
		fixed, err := doctor.Fix(kbRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: doctor --fix: %v\n", err)
		}
		for _, line := range fixed {
			fmt.Println(line)
		}
		if len(fixed) == 0 {
			fmt.Println("doctor: nothing fixable to repair")
		}
	}
	fs, err := doctor.Run(kbRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: doctor: %v\n", err)
		return 2
	}
	switch format {
	case report.FormatJSON:
		out, err := json.MarshalIndent(fs, "", "  ")
		if err != nil {
			return 2
		}
		os.Stdout.Write(out)
		os.Stdout.Write([]byte("\n"))
	default:
		renderDoctorHuman(fs)
	}
	for _, f := range fs {
		if f.Severity == "error" {
			fmt.Fprintln(os.Stderr, "pylon doctor: error-grade findings remain; see above")
			return 1
		}
	}
	if len(fs) == 0 {
		fmt.Println("doctor: ledger structure is healthy")
	}
	return 0
}

// renderDoctorHuman groups findings worst-first with the fix hint.
func renderDoctorHuman(fs []doctor.Finding) {
	if len(fs) == 0 {
		return
	}
	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sorted := append([]doctor.Finding(nil), fs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if rank[sorted[i].Severity] != rank[sorted[j].Severity] {
			return rank[sorted[i].Severity] < rank[sorted[j].Severity]
		}
		return sorted[i].ID < sorted[j].ID
	})
	severity := ""
	for _, f := range sorted {
		if f.Severity != severity {
			severity = f.Severity
			fmt.Printf("\n== %s ==\n", strings.ToUpper(severity))
		}
		loc := f.ID
		if f.File != "" {
			loc += " " + f.File
		}
		fmt.Printf("%s: %s\n", loc, f.Message)
		if f.Fixable {
			fmt.Printf("  fixable: %s\n", f.Fix)
		} else if f.Fix != "" {
			fmt.Printf("  by hand: %s\n", f.Fix)
		}
	}
	fmt.Printf("\ndoctor: %d finding(s); run pylon doctor --fix to apply the fixable ones\n", len(fs))
}
