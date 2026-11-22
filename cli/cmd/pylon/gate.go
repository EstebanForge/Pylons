// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

// runGate is the engine-owned gate entry the hook shims call. All
// gate logic lives here so hook files stay thin enough to survive
// external tooling that regenerates them.
func runGate(kbRoot, stage string) int {
	switch stage {
	case "pre-commit":
		return gatePreCommit(kbRoot)
	case "pre-push":
		return gatePrePush(kbRoot)
	default:
		fmt.Fprintln(os.Stderr, "pylon: gate requires 'pre-commit' or 'pre-push'")
		return 2
	}
}

func gatePreCommit(kbRoot string) int {
	// Commit-time convenience: auto-stage manifest drift. Push time blocks instead.
	for _, kind := range []string{"rules", "packages"} {
		if !generatesManifest(kbRoot, kind) {
			continue
		}
		path := filepath.Join(kbRoot, "manifests", kind+".json")
		if kind == "rules" {
			cmdManifestJSON(path, rules.AllRules(), false)
		} else {
			cmdManifestJSON(path, rules.BuildPackagesManifest(kbRoot), false)
		}
	}
	if _, err := gitOut(kbRoot, "diff", "--exit-code", "manifests/rules.json", "manifests/packages.json"); err != nil {
		// Non-zero exit means drift; stage it.
		if _, err := gitOut(kbRoot, "add", "manifests/rules.json", "manifests/packages.json"); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: stage manifests: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "pylon: auto-staged regenerated manifests")
		}
	}

	code := lintGate(kbRoot, true)
	if code == 3 {
		fmt.Fprintln(os.Stderr, "pylon: no manifests/pylons.yaml; skipping lint gate")
	} else if code != 0 {
		fmt.Fprintln(os.Stderr, "pylon: lint findings on changed docs; fix before committing")
		return 1
	}

	for _, f := range stagedFiles(kbRoot) {
		if f == "proposals/README.md" || f == "proposals/AGENTS.md" {
			continue
		}
		if strings.HasPrefix(f, "proposals/") && strings.HasSuffix(f, ".md") {
			findings := rules.ReviewPlan(kbRoot, f)
			if report.HasBlocking(findings) {
				renderHuman(findings)
				fmt.Fprintf(os.Stderr, "pylon: plan review findings in %s; fix structure before committing\n", f)
				return 1
			}
		}
	}
	for _, f := range stagedFiles(kbRoot) {
		if f == "quirks/README.md" || f == "quirks/AGENTS.md" {
			continue
		}
		if strings.HasPrefix(f, "quirks/") && strings.HasSuffix(f, ".md") {
			findings := rules.ReviewPostmortem(kbRoot, f, true)
			if report.HasBlocking(findings) {
				renderHuman(findings)
				fmt.Fprintf(os.Stderr, "pylon: postmortem review findings in %s; verify source paths before committing\n", f)
				return 1
			}
		}
	}
	return 0
}

func gatePrePush(kbRoot string) int {
	fail := false
	for _, kind := range []string{"rules", "packages"} {
		if !generatesManifest(kbRoot, kind) {
			continue
		}
		path := filepath.Join(kbRoot, "manifests", kind+".json")
		if kind == "rules" {
			if cmdManifestJSON(path, rules.AllRules(), true) != 0 {
				fail = true
			}
		} else if cmdManifestJSON(path, rules.BuildPackagesManifest(kbRoot), true) != 0 {
			fail = true
		}
	}
	if fail {
		fmt.Fprintln(os.Stderr, "pylon: manifests stale with frontmatter; run: bin/pylon manifest rules && bin/pylon manifest packages")
		return 1
	}

	code := lintGate(kbRoot, false)
	if code != 0 {
		if code == 3 {
			fmt.Fprintln(os.Stderr, "pylon: no manifests/pylons.yaml; the KB is broken; fix before pushing")
		} else {
			fmt.Fprintln(os.Stderr, "pylon: corpus lint findings; fix before pushing")
		}
		return 1
	}

	findings := rules.ReviewPostmortem(kbRoot, "", true)
	if report.HasBlocking(findings) {
		renderHuman(findings)
		fmt.Fprintln(os.Stderr, "pylon: quirk source validation failed; declared code paths missing")
		return 1
	}
	return 0
}

func lintGate(kbRoot string, changedOnly bool) int {
	findings, _, _ := lintCorpus(kbRoot, rules.LintOptions{HasCode: true, ChangedOnly: changedOnly})
	rules.Remediate(findings)
	if err := report.Render(os.Stdout, report.FormatHuman, findings); err != nil {
		return 1
	}
	if report.HasBlocking(findings) {
		return 1
	}
	// Lint exits 3 only through the no-KB-root path, which gate cannot
	// reach; mirror the bash exit contract for broken configs instead.
	for _, f := range findings {
		if f.File == "manifests/pylons.yaml" && f.Grade == report.GradeForced {
			return 3
		}
	}
	return 0
}

// lintCorpus is the one lint entry every gate consumer shares: it runs
// the corpus lint and applies the accepted-debt baseline, so gates and
// the lint command suppress exactly the same findings. Keep lint entry
// points on this helper; a second unbaselined path resurrects accepted
// debt as blocking.
func lintCorpus(kbRoot string, opts rules.LintOptions) ([]report.Finding, int, int) {
	findings := rules.Lint(kbRoot, opts)
	return rules.ApplyBaseline(kbRoot, findings)
}

func stagedFiles(kbRoot string) []string {
	out, err := gitOut(kbRoot, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if err != nil && len(out) == 0 {
		return nil
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files
}

func renderHuman(findings []report.Finding) {
	rules.Remediate(findings)
	_ = report.Render(os.Stdout, report.FormatHuman, findings)
}

// gitOut runs one git command in the KB root and returns stdout.
func gitOut(kbRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", kbRoot}, args...)...)
	return cmd.Output()
}
