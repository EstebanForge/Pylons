// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// Declared schemes are recognized: well-formed links pass, malformed
// ones fire PYLON012. Undeclared schemes stay outside the engine.
func TestDeclaredSchemeLinks(t *testing.T) {
	doc := func(body string) []report.Finding {
		t.Helper()
		return checkMarkdownLinks([]string{"legacy"}, t.TempDir(), "plans/x.md", body)
	}
	if fs := doc("[pkg](legacy://pkg/file.md)"); len(fs) != 0 {
		t.Fatalf("well-formed declared-scheme link must pass, got %v", fs)
	}
	if fs := doc("[pkg](legacy://)"); len(fs) != 1 || fs[0].RuleID != "PYLON012" {
		t.Fatalf("empty declared-scheme path must fire PYLON012, got %v", fs)
	}
	if fs := doc("[pkg](legacy://../escape.md)"); len(fs) != 1 {
		t.Fatalf("traversal in a declared scheme must fire PYLON012, got %v", fs)
	}
	if fs := doc("[pkg](unknown://whatever)"); len(fs) != 0 {
		t.Fatalf("undeclared schemes stay silent, got %v", fs)
	}
}
