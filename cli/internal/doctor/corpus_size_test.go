// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus-size advisory keeps ledger growth visible: total bytes
// and the largest documents, counted over the same corpus the lint
// walk reads (excludes honored). It stays silent on small ledgers
// and fires past the growth threshold. It is information only —
// doctor never deletes or trims anything to change it.
func TestDoctorReportsCorpusSize(t *testing.T) {
	dir := brokenLedger(t)
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Cross the doc-count threshold; one document carries the bulk so
	// the largest-name assertion has a stable winner.
	write("references/heavy.md", strings.Repeat("a", 6000)+"\n")
	write("manifests/pylons.yaml", "version: 1\nschemes: []\nexclude:\n  - references/light.md\n")
	write("references/light.md", strings.Repeat("b", 300)+"\n")
	for i := 0; i < 120; i++ {
		write(fmt.Sprintf("references/n%03d.md", i), "x\n")
	}

	fs, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found []Finding
	for _, f := range fs {
		if f.ID == "DOCTOR013" {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one DOCTOR013, got %d: %+v", len(found), found)
	}
	f := found[0]
	if f.Fixable {
		t.Fatal("corpus size is a fact, not something doctor fixes")
	}
	// kept.md + heavy + n000..n119; light.md is excluded.
	if !strings.Contains(f.Message, "122 docs") {
		t.Fatalf("count must cover the corpus minus excluded docs, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "references/heavy.md") {
		t.Fatalf("the largest doc must be named first, got: %s", f.Message)
	}
	if strings.Contains(f.Message, "references/light.md") {
		t.Fatalf("excluded docs must not count, got: %s", f.Message)
	}

	// --fix leaves the advisory: nothing about size is repairable.
	if _, err := Fix(dir); err != nil {
		t.Fatal(err)
	}
	fs, _ = Run(dir)
	still := false
	for _, g := range fs {
		if g.ID == "DOCTOR013" {
			still = true
		}
	}
	if !still {
		t.Fatal("fix must not make the corpus-size advisory disappear")
	}
}

// Below the growth threshold the advisory stays silent so a healthy
// small ledger keeps doctor's empty-report contract.
func TestDoctorCorpusSizeStaysSilentBelowThreshold(t *testing.T) {
	dir := brokenLedger(t)
	path := filepath.Join(dir, "references", "one.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 900)), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.ID == "DOCTOR013" {
			t.Fatalf("small ledger must stay silent, got: %+v", f)
		}
	}
}
