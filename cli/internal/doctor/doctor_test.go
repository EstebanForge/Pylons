// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package doctor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/install"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// brokenLedger builds a ledger with pylons.yaml present but the product
// layer gutted: no exact files, no zone docs, no keepers, garbage
// generated manifests, no product record, and one dead project path.
func brokenLedger(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
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
	write("manifests/pylons.yaml", "version: 1\nschemes: []\nprojects:\n  gone:\n    path: ../nowhere\n")
	write("manifests/rules.json", "{not json\n")
	write("manifests/baseline.json", "version: 9\n")
	write("packages/kept.md", "---\ntitle: Kept\ntype: package\nstatus: active\ncreated: 2026-11-27\nupdated: 2026-11-27\n---\n\nA package.\n")
	return dir
}

func finding(t *testing.T, fs []Finding, id string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no %s in findings %v", id, ids(fs))
	return Finding{}
}

func ids(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

func hasID(fs []Finding, id string) bool {
	for _, f := range fs {
		if f.ID == id {
			return true
		}
	}
	return false
}

func TestDoctorFlagsBrokenStructure(t *testing.T) {
	dir := brokenLedger(t)
	fs, err := Run(dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	f := finding(t, fs, "DOCTOR002")
	if f.Severity != "error" || !f.Fixable {
		t.Fatalf("DOCTOR002 = %+v; a missing zone doc is a fixable error", f)
	}
	if f := finding(t, fs, "DOCTOR003"); f.Severity != "warning" || !f.Fixable {
		t.Fatalf("DOCTOR003 = %+v; a missing keeper is a fixable warning", f)
	}
	if f := finding(t, fs, "DOCTOR010"); f.Severity != "error" || !f.Fixable {
		t.Fatalf("DOCTOR010 = %+v; an unparseable generated manifest is a fixable error", f)
	}
	if f := finding(t, fs, "DOCTOR012"); f.Severity != "error" {
		t.Fatalf("DOCTOR012 = %+v; an unparseable baseline is an error", f)
	}
	if f := finding(t, fs, "DOCTOR006"); f.Fixable {
		t.Fatal("DOCTOR006 must not be fixable; the path is the user's call")
	}
	if f := finding(t, fs, "DOCTOR011"); f.Severity != "warning" {
		t.Fatalf("DOCTOR011 = %+v; a legacy layout is a warning", f)
	}
	if !hasID(fs, "DOCTOR001") || !hasID(fs, "DOCTOR009") {
		t.Fatalf("findings %v miss DOCTOR001/DOCTOR009", ids(fs))
	}
}

func TestDoctorSkipsHooksOutsideGit(t *testing.T) {
	dir := brokenLedger(t)
	fs, err := Run(dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, f := range fs {
		if f.ID == "DOCTOR007" || f.ID == "DOCTOR008" {
			t.Fatalf("non-git ledger reported %s: %s", f.ID, f.Message)
		}
	}
}

func TestDoctorFixRepairsAndRecords(t *testing.T) {
	dir := brokenLedger(t)
	fixed, err := Fix(dir)
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) == 0 {
		t.Fatal("Fix must report what it repaired")
	}
	// Synthesized entry docs must pass the strict frontmatter parser.
	for _, zone := range install.DocZones() {
		body, err := os.ReadFile(filepath.Join(dir, zone, "AGENTS.md"))
		if err != nil {
			t.Fatalf("%s/AGENTS.md missing after fix: %v", zone, err)
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(body), &meta) {
			t.Fatalf("%s/AGENTS.md does not strict-parse", zone)
		}
	}
	for _, keeper := range install.Keepers() {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(keeper), ".gitkeep")); err != nil {
			t.Fatalf("keeper %s missing after fix", keeper)
		}
	}
	// The regenerated rules manifest must equal what the command writes.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifestPayload("rules", dir)); err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(dir, "manifests", "rules.json"))
	if err != nil || string(disk) != buf.String() {
		t.Fatalf("rules.json not regenerated to command bytes: %v", err)
	}
	// A second fix run must converge: nothing left it can repair.
	fixed2, err := Fix(dir)
	if err != nil {
		t.Fatalf("second Fix: %v", err)
	}
	if len(fixed2) != 0 {
		t.Fatalf("second Fix repaired %v; fix must converge", fixed2)
	}
}

func TestDoctorFixRecordsProductManifest(t *testing.T) {
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
	// Complete the product layer the way a real release ships it, but
	// leave no product record: the legacy layout self-update warns on.
	for _, rel := range install.ExactFiles() {
		write(rel, rel+"\n")
	}
	write("bin/pylon", "#!/bin/sh\necho 0\n")
	if _, err := Fix(dir); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	rec, err := install.ReadProductManifest(filepath.Join(dir, "manifests", "product.json"))
	if err != nil {
		t.Fatalf("product.json does not parse: %v", err)
	}
	if rec.Version == "" || len(rec.Files) == 0 {
		t.Fatal("product.json carries no version or file hashes")
	}
	if _, pinned := rec.Binary[runtime.GOOS+"-"+runtime.GOARCH]; !pinned {
		t.Fatal("product.json does not pin the host binary")
	}
}

func TestDoctorAfterFixDropsRepairedIDs(t *testing.T) {
	dir := brokenLedger(t)
	if _, err := Fix(dir); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	fs, err := Run(dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, id := range []string{"DOCTOR002", "DOCTOR003", "DOCTOR010", "DOCTOR012"} {
		if hasID(fs, id) {
			t.Fatalf("%s survived the fix run", id)
		}
	}
	// Hands-on gaps stay honest: exact files need content, product
	// trees ship with releases, the dead project path is the user's
	// call, and bin/pylon needs a real build. DOCTOR011 cleared: the
	// zone docs the fix wrote are product-owned, so the record could
	// bootstrap from them.
	for _, id := range []string{"DOCTOR001", "DOCTOR004", "DOCTOR006", "DOCTOR009"} {
		if !hasID(fs, id) {
			t.Fatalf("findings %v lost %s", ids(fs), id)
		}
	}
}
