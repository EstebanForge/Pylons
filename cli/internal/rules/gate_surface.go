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

// templates/ is the declared frontmatter schema (TemplateKeys derives
// from it), so a diff touching it changes what the checker accepts.
const gateTemplatesPrefix = "templates/"

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
	// Per-commit union, not a tree diff: a file added and then moved
	// away within the range nets out of base...HEAD and would evade a
	// net-only check.
	// --relative prints paths KB-relative even when the ledger is a
	// subdirectory of its repository, so the surface list still matches.
	out, err := resolve.GitRun(kbRoot, nil, "log", "--name-only", "--no-renames", "--relative", "--format=", base+"..HEAD")
	if err != nil {
		return []report.Finding{advisoryFinding("PYLON063", "manifests/pylons.yaml", 1,
			"the gate surface check could not run: base "+base+" is unreadable; pin --base to a reachable ref")}
	}
	changed := changedPaths(string(out))
	var touched []string
	for _, p := range changed {
		if slices.Contains(gateSurfaces, p) || strings.HasPrefix(p, gateHooksPrefix) || strings.HasPrefix(p, gateTemplatesPrefix) {
			touched = append(touched, p)
		}
	}
	slices.Sort(touched)
	touched = slices.Compact(touched)
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
// gate-weakening knob line with its real line number. The patch is the
// net range diff on purpose: a knob that is added and removed again
// within the range leaves no widening behind.
func gateKnobFindings(kbRoot, base string) []report.Finding {
	patch, err := resolve.GitRun(kbRoot, nil, "diff", "-U3", "--no-ext-diff", "--no-color", base+"...HEAD", "--", "manifests/pylons.yaml")
	if err != nil {
		return nil
	}
	var findings []report.Finding
	for _, h := range parseKnobPatch(string(patch)) {
		findings = append(findings, errFinding("PYLON062", "manifests/pylons.yaml", h.line,
			"this diff widens a gate knob ("+h.knob+"); walk the change through the ledger owners"))
	}
	return findings
}

type knobHit struct {
	line int
	knob string
}

// parseKnobPatch flags every ADDED line inside an exclude/generate/
// schemes block: the key line itself and any deeper-indented line
// under it. Context lines carry the knob state across, so items added
// under an existing key fire too. A column-0 key ends the block; a
// blank line keeps it. Lines starting with a backslash ("\\ No
// newline at end of file") never advance the line.
func parseKnobPatch(patch string) []knobHit {
	var findings []knobHit
	line := 0
	knob := ""
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			line = hunkNewStart(l)
			knob = ""
		case strings.HasPrefix(l, "\\"):
		case strings.HasPrefix(l, "-"):
		case strings.HasPrefix(l, "+"), strings.HasPrefix(l, " "):
			added := strings.HasPrefix(l, "+")
			body := l[1:]
			switch {
			case body == "":
				line++
				continue
			case gateKnobRe.MatchString(body):
				knob = gateKnobRe.FindStringSubmatch(body)[1]
				if added {
					findings = append(findings, knobHit{line, knob})
				}
			case knob != "" && (strings.HasPrefix(body, " ") || strings.HasPrefix(body, "\t")):
				if added {
					findings = append(findings, knobHit{line, knob})
				}
			default:
				knob = ""
			}
			line++
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
