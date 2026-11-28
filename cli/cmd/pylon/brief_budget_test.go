// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brief is the session-start surface: it must stay bounded no matter
// how the ledger grows. The budget caps the output lines, and the
// sections that fit are deterministic (fixed priority, no sampling).
func TestBriefBoundedSections(t *testing.T) {
	kb := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(kb, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	doc := func(title, typ, status string) string {
		return "---\ntitle: " + title + "\ntype: " + typ + "\nstatus: " + status + "\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# " + title + "\n\nBody.\n"
	}
	write(filepath.Join("manifests", "pylons.yaml"), "version: 1\n")
	write(filepath.Join("references", "auth.md"), doc("Auth", "reference", "active"))
	write(filepath.Join("references", "billing.md"), doc("Billing", "reference", "active"))
	write(filepath.Join("packages", "orm.md"), doc("ORM", "package", "active"))
	write(filepath.Join("plans", "feature-one.md"), doc("One", "plan", "in-progress"))
	write(filepath.Join("plans", "feature-two.md"), "---\ntitle: Two\ntype: plan\nstatus: paused\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [Pylons]\ntags: []\n---\n\n# Two\n\n## State\n\nNext step is the retry path.\n")
	write(filepath.Join("proposals", "docs-three.md"), doc("Three", "proposal", "proposed"))

	_, out, _ := runPylon(t, kb, "brief")
	lines := nonEmptyLines(out)
	if len(lines) < 3 {
		t.Fatalf("brief should print header, zones, and in-flight, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "docs: 6") {
		t.Fatalf("header must keep the counts line, got: %s", lines[0])
	}
	if !strings.Contains(out, "references 2") || !strings.Contains(out, "packages 1") {
		t.Fatalf("zones line must count docs per zone, got:\n%s", out)
	}
	if !strings.Contains(out, "1 in progress") || !strings.Contains(out, "1 paused") || !strings.Contains(out, "1 in review") {
		t.Fatalf("in-flight line must report plan and proposal states, got:\n%s", out)
	}

	// The budget caps lines; the truncation marker announces what a
	// higher cap would restore. Header and in-flight outrank zones.
	_, out, _ = runPylon(t, kb, "brief", "--budget", "2")
	lines = nonEmptyLines(out)
	if strings.Contains(out, "references 2") {
		t.Fatalf("budget 2 must drop the zones line, got:\n%s", out)
	}
	if !strings.Contains(out, "1 paused") {
		t.Fatalf("budget 2 must keep the in-flight line, got:\n%s", out)
	}
	if !strings.Contains(out, "hidden") {
		t.Fatalf("a dropped section must be announced, got:\n%s", out)
	}

	// JSON stays complete: budget is a human-display concern.
	_, jout, _ := runPylon(t, kb, "brief", "--format", "json", "--budget", "2")
	if !strings.Contains(jout, `"in_flight"`) || !strings.Contains(jout, `"zones"`) {
		t.Fatalf("json brief must carry zones and in_flight:\n%s", jout)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
