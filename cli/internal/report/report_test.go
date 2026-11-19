// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderFormatsAndExitCode(t *testing.T) {
	findings := []Finding{
		{RuleID: "PYLON001", Severity: SeverityError, Grade: GradeForced, File: "plans/a.md", Message: "missing status"},
		{RuleID: "PYLON015", Severity: SeverityAdvisory, Grade: GradeAdvisory, File: "packages/x.md", Message: "stale doc"},
	}

	if !HasBlocking(findings) {
		t.Fatal("expected HasBlocking=true when error finding exists")
	}
	if HasBlocking(findings[1:]) {
		t.Fatal("expected HasBlocking=false for advisory-only")
	}

	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, findings); err != nil {
		t.Fatal(err)
	}
	var decoded []Finding
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("got %d findings, want 2", len(decoded))
	}

	buf.Reset()
	if err := Render(&buf, FormatHuman, findings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "FORCED") || !strings.Contains(buf.String(), "PYLON001") {
		t.Fatalf("unexpected human output: %s", buf.String())
	}

	buf.Reset()
	if err := Render(&buf, FormatSARIF, findings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"version": "2.1.0"`) {
		t.Fatalf("unexpected SARIF output: %s", buf.String())
	}
}
