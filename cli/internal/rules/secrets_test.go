// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
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
	if len(ids["PYLON061"]) != 3 {
		t.Fatalf("expected 3 PYLON061 findings, got %v", ids)
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
