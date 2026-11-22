// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"regexp"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// Secret patterns are deliberately small and provider-shaped so the
// FORCED rule stays quiet on reference docs. Keep this list closed;
// broad shapes belong to dedicated scanners.
var secretProviderRes = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`),
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

var secretAssignmentRe = regexp.MustCompile(`(?i)(password|passwd|secret|token|api_?key)[[:space:]]*[=:][[:space:]]*["']?[A-Za-z0-9/+_.-]{12,}`)

const secretFindingsPerFile = 8

func scanSecrets(docs []corpusDoc) []report.Finding {
	var findings []report.Finding
	for _, d := range docs {
		provider := 0
		assignment := 0
		for i, line := range strings.Split(d.content, "\n") {
			if provider >= secretFindingsPerFile && assignment >= secretFindingsPerFile {
				break
			}
			if provider < secretFindingsPerFile {
				for _, re := range secretProviderRes {
					if re.MatchString(line) {
						findings = append(findings, errFinding("PYLON060", d.rel, i+1, "possible credential: rotate it; deleting the line does not remove it from git history"))
						provider++
						break
					}
				}
			}
			if assignment < secretFindingsPerFile && secretAssignmentRe.MatchString(line) {
				findings = append(findings, advisoryFinding("PYLON061", d.rel, i+1, "possible secret assignment: rotate it if real, or reword the example"))
				assignment++
			}
		}
	}
	return findings
}
