// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// gateSurfaces is the closed set of files that decide what the checker
// may silence. The list is deliberately hard-coded: a configurable
// list could exclude itself.
var gateSurfaces = []string{
	"manifests/baseline.json",
	"manifests/ratchet.json",
	"manifests/vocabulary.json",
	"manifests/rules.json",
	"manifests/packages.json",
	"manifests/product.json",
	"manifests/extensions.json",
	"manifests/parity.json",
}

const gateHooksPrefix = ".githooks/"

// gateKnobRe matches added lines that widen a gate-weakening knob in
// manifests/pylons.yaml: exclude skips linting, generate disables
// gate-maintained manifests, schemes relaxes link checks.
var gateKnobRe = regexp.MustCompile(`^\s*(exclude|generate|schemes):`)

// GateSurface reports diffs against base that touch the surfaces the
// checker itself relies on: the manifests that carry suppression and
// closed sets, the shipped hooks, and the gate-weakening knobs in
// manifests/pylons.yaml.
func GateSurface(kbRoot, base string) []report.Finding {
	if strings.TrimSpace(base) == "" {
		base = "origin/main"
	}
	out, err := resolve.GitRun(kbRoot, nil, "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return []report.Finding{advisoryFinding("PYLON063", "manifests/pylons.yaml", 1,
			"the gate surface check could not run: base "+base+" is unreadable; pin --base to a reachable ref")}
	}
	changed := changedPaths(string(out))
	var touched []string
	for _, p := range changed {
		if slices.Contains(gateSurfaces, p) || strings.HasPrefix(p, gateHooksPrefix) {
			touched = append(touched, p)
		}
	}
	slices.Sort(touched)
	var findings []report.Finding
	for _, p := range touched {
		findings = append(findings, errFinding("PYLON062", p, 1,
			"gate surface changed in this diff; walk the change through the ledger owners"))
	}
	if slices.Contains(changed, "manifests/pylons.yaml") {
		findings = append(findings, gateKnobFindings(kbRoot, base)...)
	}
	return findings
}

// gateKnobFindings reads the pylons.yaml patch and reports each added
// gate-weakening knob line with its real line number.
func gateKnobFindings(kbRoot, base string) []report.Finding {
	patch, err := resolve.GitRun(kbRoot, nil, "diff", "-U0", base+"...HEAD", "--", "manifests/pylons.yaml")
	if err != nil {
		return nil
	}
	var findings []report.Finding
	line := 0
	for _, l := range strings.Split(string(patch), "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			line = hunkNewStart(l)
		case strings.HasPrefix(l, "+"):
			body := l[1:]
			if m := gateKnobRe.FindStringSubmatch(body); m != nil {
				findings = append(findings, errFinding("PYLON062", "manifests/pylons.yaml", line,
					"this diff widens a gate knob ("+m[1]+"); walk the change through the ledger owners"))
			}
			line++
		case strings.HasPrefix(l, "-"):
		default:
			line++
		}
	}
	return findings
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func hunkNewStart(header string) int {
	m := hunkHeaderRe.FindStringSubmatch(header)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

func changedPaths(diffOut string) []string {
	var out []string
	for _, p := range strings.Split(strings.TrimSpace(diffOut), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
