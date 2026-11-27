// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package doctor

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestDoctorFixLeavesRecordToSelfUpdate(t *testing.T) {
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
	// Complete the product layer the way a real release ships it, plus
	// a ledger-local template the ledger-templates feature encourages,
	// but leave no product record: the legacy layout self-update warns
	// on. Bootstrapping the record from disk would mark both as
	// release state, and the next update would delete the local
	// template as release-dropped.
	for _, rel := range install.ExactFiles() {
		write(rel, rel+"\n")
	}
	write("bin/pylon", "#!/bin/sh\necho 0\n")
	write("templates/local-schema.md", "---\ntype: reference\n---\n\nLocal extension.\n")

	fixed, err := Fix(dir)
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "manifests", "product.json")); !os.IsNotExist(statErr) {
		t.Fatal("doctor --fix wrote manifests/product.json; only a real release may establish the record")
	}
	for _, line := range fixed {
		if strings.Contains(line, "DOCTOR011") {
			t.Fatalf("fix reported a DOCTOR011 repair: %q", line)
		}
	}
	fs, err := Run(dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f := finding(t, fs, "DOCTOR011"); f.Fixable {
		t.Fatal("DOCTOR011 must not be doctor-fixable; the record needs a real release")
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
	// call, bin/pylon needs a real build, and the product record needs
	// a real release applied by self-update.
	for _, id := range []string{"DOCTOR001", "DOCTOR004", "DOCTOR006", "DOCTOR009", "DOCTOR011"} {
		if !hasID(fs, id) {
			t.Fatalf("findings %v lost %s", ids(fs), id)
		}
	}
}

func TestDoctorFixContinuesPastRepairErrors(t *testing.T) {
	dir := brokenLedger(t)
	// A regular file occupies the plans/archive path, so the keeper
	// repair cannot mkdir there; every other repair must still happen.
	occupied := filepath.Join(dir, "plans", "archive")
	if err := os.MkdirAll(filepath.Dir(occupied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occupied, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixed, err := Fix(dir)
	if err == nil {
		t.Fatal("Fix must surface the failed repair")
	}
	if !strings.Contains(err.Error(), "plans/archive/.gitkeep") {
		t.Fatalf("error does not name the failed repair: %v", err)
	}
	if len(fixed) == 0 {
		t.Fatal("failed keeper repair must not skip the other repairs")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "packages", "AGENTS.md")); statErr != nil {
		t.Fatalf("zone docs skipped after a failed repair: %v", statErr)
	}
}

func TestDoctorRewriteBaselineKeepsBackupAndRefusesNewer(t *testing.T) {
	dir := brokenLedger(t)
	// The corrupt baseline rewrite must keep the old bytes.
	old, err := os.ReadFile(filepath.Join(dir, "manifests", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Fix(dir); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "manifests", "baseline.json.bak.*"))
	if len(matches) != 1 {
		t.Fatalf("rewrite left %d .bak copies, want 1", len(matches))
	}
	kept, err := os.ReadFile(matches[0])
	if err != nil || string(kept) != string(old) {
		t.Fatal("the .bak copy does not carry the pre-rewrite baseline")
	}
	// A baseline from a newer engine must not be downgraded.
	newer := filepath.Join(dir, "manifests", "baseline.json")
	if err := os.WriteFile(newer, []byte(`{"version": 2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, fs, "DOCTOR012")
	if f.Fixable {
		t.Fatal("a newer-version baseline must not be doctor-fixable")
	}
	if _, err := Fix(dir); err != nil {
		t.Fatalf("fixing a ledger with a newer baseline must be a clean no-op for it: %v", err)
	}
	body, _ := os.ReadFile(newer)
	if string(body) != `{"version": 2}`+"\n" {
		t.Fatal("the newer baseline was overwritten")
	}
}

func TestDoctorHonorsExcludeAndConfigErr(t *testing.T) {
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
	// Excluded zones are silent on every surface; doctor must not
	// demand their entry docs or recreate their keepers.
	write("manifests/pylons.yaml", "version: 1\nschemes: []\nexclude:\n  - packages/**\nprojects:\n  gone:\n    path: ../nowhere\n")
	fs, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.ID == "DOCTOR002" && strings.HasPrefix(f.File, "packages/") {
			t.Fatal("doctor demanded an entry doc for an excluded zone")
		}
	}
	// An unreadable pylons.yaml blocks manifest regeneration: the
	// generate opt-out cannot be read, so writing could stomp a
	// manifest handed to foreign tooling.
	write("manifests/pylons.yaml", "version: 1\n\t\tno valid yaml here: [\n")
	_, err = Fix(dir)
	if err == nil || !strings.Contains(err.Error(), "generate opt-out") {
		t.Fatalf("fix must refuse manifest regen on a broken config, got: %v", err)
	}
	var onDisk map[string]any
	body, _ := os.ReadFile(filepath.Join(dir, "manifests", "rules.json"))
	if err := json.Unmarshal(body, &onDisk); err == nil {
		t.Fatal("the garbage rules manifest was regenerated despite the unreadable config")
	}
}

func TestDoctorHooksOwnershipBoundaries(t *testing.T) {
	dir := brokenLedger(t)
	// Foreign hooksPath: one non-fixable DOCTOR008, nothing fixable —
	// the takeover must stay a human decision.
	if out, err := exec.Command("git", "init", "-q", "-b", "main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "core.hooksPath", ".husky").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	fs, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := finding(t, fs, "DOCTOR008")
	if f.Fixable || f.Severity != "warning" {
		t.Fatalf("foreign hooksPath finding = %+v; must be a non-fixable warning", f)
	}
	if _, err := Fix(dir); err != nil {
		t.Fatalf("fix must not fail on a foreign hooksPath: %v", err)
	}
	cur, err := exec.Command("git", "-C", dir, "config", "--get", "core.hooksPath").Output()
	if err != nil || strings.TrimSpace(string(cur)) != ".husky" {
		t.Fatalf("fix took over the foreign hooksPath: %q (err %v)", strings.TrimSpace(string(cur)), err)
	}
	// Nested ledger: hooks belong to the parent repo; an info finding
	// replaces the unfixable DOCTOR007 error.
	nested := filepath.Join(dir, "ledger-nested")
	if err := os.MkdirAll(filepath.Join(nested, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "manifests", "pylons.yaml"), []byte("version: 1\nschemes: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err = Run(nested)
	if err != nil {
		t.Fatal(err)
	}
	if hasID(fs, "DOCTOR007") {
		t.Fatal("a nested ledger must not be flagged DOCTOR007")
	}
	f = finding(t, fs, "DOCTOR008")
	if f.Severity != "info" || f.Fixable {
		t.Fatalf("nested-ledger finding = %+v; must be non-fixable info", f)
	}
}
