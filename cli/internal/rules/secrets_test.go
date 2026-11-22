// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

func secretDoc(content string) []corpusDoc {
	return []corpusDoc{{rel: "investigations/2026-11-01-leak.md", content: content, meta: resolve.DocMeta{Title: "leak", Type: "investigation"}}}
}

func secretIDs(fs []report.Finding) map[string][]int {
	out := map[string][]int{}
	for _, f := range fs {
		out[f.RuleID] = append(out[f.RuleID], f.Line)
	}
	return out
}

func TestSecretProviderShapeForces(t *testing.T) {
	src := "---\ntitle: leak\ntype: investigation\nstatus: resolved\n---\n\nLogs:\n\n```\nAKIAIOSFODNN7EXAMPLE\nghp_abcdefghijklmnopqrstuvwxyz0123456789ABCD\ngithub_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZabcd1234\nsk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij\nxoxb-123456789012-1234567890123-abcdefghijklmnop\n-----BEGIN RSA PRIVATE KEY-----\n```\n"
	fs := scanSecrets(secretDoc(src))
	ids := secretIDs(fs)
	if len(ids["PYLON060"]) != 6 {
		t.Fatalf("expected 6 PYLON060 findings, got %v", ids)
	}
	for _, f := range fs {
		if f.Severity != report.SeverityError || f.Grade != report.GradeForced {
			t.Fatalf("PYLON060 must be error/FORCED, got %v/%v", f.Severity, f.Grade)
		}
		if f.Line < 10 || f.Line > 15 {
			t.Fatalf("finding line %d outside the fenced block", f.Line)
		}
		if strings.Contains(f.Message, "AKIA") || strings.Contains(f.Message, "ghp_") {
			t.Fatalf("message must not echo the matched value: %s", f.Message)
		}
	}
}

func TestSecretAssignmentAdvisory(t *testing.T) {
	src := "---\ntitle: cfg\ntype: investigation\nstatus: resolved\n---\n\npassword: hunter2hunter2\ntoken=abcdef0123456789\napi_key: your-api-key-here\nshort: abc\nHOST=localhost\n"
	fs := scanSecrets(secretDoc(src))
	ids := secretIDs(fs)
	if len(ids["PYLON061"]) != 2 {
		t.Fatalf("digit-bearing assignments fire, word-only values stay prose: %v", ids)
	}
	for _, f := range fs {
		if f.Severity != report.SeverityAdvisory || f.Grade != report.GradeAdvisory {
			t.Fatalf("PYLON061 must be advisory, got %v/%v", f.Severity, f.Grade)
		}
	}
	if len(ids["PYLON060"]) != 0 {
		t.Fatalf("assignments must not fire the provider rule: %v", ids)
	}
}

func TestSecretCleanDocSilent(t *testing.T) {
	src := "---\ntitle: fine\ntype: investigation\nstatus: resolved\n---\n\nThe token endpoint rotates hourly.\napi_key = read from the vault\npassword: see the vault\n"
	fs := scanSecrets(secretDoc(src))
	if len(fs) != 0 {
		t.Fatalf("clean doc must stay silent, got %v", fs)
	}
}

func TestSecretFindingsCapped(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("token=0123456789abcdef\n")
	}
	src := "---\ntitle: flood\ntype: investigation\nstatus: resolved\n---\n\n" + b.String()
	fs := scanSecrets(secretDoc(src))
	if len(fs) > 8 {
		t.Fatalf("findings must cap at 8 per file, got %d", len(fs))
	}
}

func TestSecretScanRunsOnUnparsableDocs(t *testing.T) {
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\n")
	mustWrite(t, filepath.Join(kb, "references/creds.md"), "---\ntitle: creds\n&bad anchors: yes\ntype: reference\n---\n\nAKIAIOSFODNN7EXAMPLE\n")
	fs := Lint(kb, LintOptions{})
	ids := findingRules(fs)
	joined := strings.Join(ids, "\n")
	if !strings.Contains(joined, "PYLON001") {
		t.Fatalf("the broken frontmatter must still fail, got %v", ids)
	}
	if !strings.Contains(joined, "PYLON060") {
		t.Fatalf("a doc that fails strict parsing must still be scanned for secrets, got %v", ids)
	}
}

func TestSecretAssignmentRequiresDigit(t *testing.T) {
	fs := scanSecrets(secretDoc("password = correct-horse-battery-staple\npassword = correct-horse-12345\n"))
	ids := secretIDs(fs)
	if len(ids["PYLON061"]) != 1 {
		t.Fatalf("a word-only assignment is prose, a digit-bearing one is a finding: %v", ids)
	}
}

func TestSecretPrefixInsideWordStaysQuiet(t *testing.T) {
	fs := scanSecrets(secretDoc("the task-ant-api-key-registry-service-handling layer tokens\n"))
	if ids := secretIDs(fs); len(ids["PYLON060"]) != 0 {
		t.Fatalf("sk-ant- inside a word must not fire: %v", ids)
	}
}

func TestSecretAllowMarkerSkipsLine(t *testing.T) {
	fs := scanSecrets(secretDoc("AKIAIOSFODNN7EXAMPLE pylons-allow-secret\nAKIAIOSFODNN7EXAMPLE\n"))
	ids := secretIDs(fs)
	if len(ids["PYLON060"]) != 1 || ids["PYLON060"][0] != 2 {
		t.Fatalf("the allow marker must exempt only its own line: %v", ids)
	}
}

func TestSecretProviderPgpBlockForces(t *testing.T) {
	src := "```\n-----BEGIN PGP PRIVATE KEY BLOCK-----\n-----BEGIN OPENSSH PRIVATE KEY-----\n```\n"
	fs := scanSecrets(secretDoc(src))
	ids := secretIDs(fs)
	if len(ids["PYLON060"]) != 2 {
		t.Fatalf("PGP and OpenSSH private-key headers must force, got %v", ids)
	}
}

func TestSecretAssignmentShapesAdvisory(t *testing.T) {
	src := strings.Join([]string{
		"```json",
		`"password": "Hunter2Hunter2Hunter2"`,
		"```",
		"",
		"SECRET_KEY=abcdef123456abcdef",
		"api-key: gh-tokens-1234567890ab",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"",
	}, "\n")
	fs := scanSecrets(secretDoc(src))
	ids := secretIDs(fs)
	if len(ids["PYLON061"]) != 4 {
		t.Fatalf("JSON, ENV, dash-key, and bearer shapes must advise, got %v", ids)
	}
	for _, f := range fs {
		if f.Severity != report.SeverityAdvisory || f.Grade != report.GradeAdvisory {
			t.Fatalf("PYLON061 must stay advisory, got %v/%v", f.Severity, f.Grade)
		}
	}
}
