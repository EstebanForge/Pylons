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
	regexp.MustCompile(`(^|[^A-Za-z0-9])sk-ant-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY( BLOCK)?-----`),
}

// The bridge class covers the shapes real files use: a JSON closing
// quote ("password":), an ENV suffix (SECRET_KEY=), and an optional
// bearer scheme before the value.
var secretAssignmentRe = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|authorization)[^:=\n]{0,4}[=:][[:space:]]*(bearer[[:space:]]+)?["']?[A-Za-z0-9/+_.-]{12,}`)

// secretAllowMarker exempts exactly the line it sits on. Reference
// docs quote real provider example keys (the AWS docs key is a known
// one), and a FORCED rule the baseline cannot suppress needs its own
// escape hatch.
const secretAllowMarker = "pylons-allow-secret"

const secretFindingsPerFile = 8

func scanSecrets(docs []corpusDoc) []report.Finding {
	var findings []report.Finding
	for _, d := range docs {
		provider := 0
		assignment := 0
		for i, line := range strings.Split(d.content, "\n") {
			if strings.Contains(line, secretAllowMarker) {
				continue
			}
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
			if assignment < secretFindingsPerFile {
				if m := secretAssignmentRe.FindString(line); m != "" && strings.ContainsAny(m, "0123456789") {
					findings = append(findings, advisoryFinding("PYLON061", d.rel, i+1, "possible secret assignment: rotate it if real, or reword the example"))
					assignment++
				}
			}
		}
	}
	return findings
}
