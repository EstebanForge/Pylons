// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newSandboxKB builds a ledger skeleton with the repo's real templates
// copied in, because pylon new reads the ledger's own templates/ as the
// single schema source.
func newSandboxKB(t *testing.T) string {
	t.Helper()
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	templates := filepath.Join(kb, "templates")
	if err := os.MkdirAll(templates, 0o755); err != nil {
		t.Fatal(err)
	}
	repoTemplates := "../../../templates"
	entries, err := os.ReadDir(repoTemplates)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoTemplates, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(templates, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return kb
}

// runNew execs the built binary through the shared runPylon helper and
// returns combined output plus the exit code.
func runNew(t *testing.T, kb string, args ...string) (string, int) {
	t.Helper()
	code, stdout, stderr := runPylon(t, kb, append([]string{"new", "--pylons", "."}, args...)...)
	return stdout + stderr, code
}

// fmValue reads one top-level key from a document's frontmatter block.
func fmValue(body, key string) string {
	fm := body
	if strings.HasPrefix(fm, "---\n") {
		if end := strings.Index(fm[4:], "\n---"); end >= 0 {
			fm = fm[4 : 4+end]
		}
	}
	for _, line := range strings.Split(fm, "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		}
	}
	return ""
}

func TestNewCreatesQuirkReadyToEdit(t *testing.T) {
	kb := newSandboxKB(t)
	out, ec := runNew(t, kb, "quirk", "webhook-retries")
	if ec != 0 {
		t.Fatalf("exit %d: %s", ec, out)
	}
	path := filepath.Join(kb, "quirks", "webhook-retries.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the quirk must land at quirks/webhook-retries.md: %v", err)
	}
	body := string(data)
	today := time.Now().Format("2006-01-02")
	if got := fmValue(body, "created"); got != today {
		t.Fatalf("created = %q, want %q", got, today)
	}
	if got := fmValue(body, "updated"); got != today {
		t.Fatalf("updated = %q, want %q", got, today)
	}
	if got := fmValue(body, "type"); got != "quirk" {
		t.Fatalf("type = %q", got)
	}
	if got := fmValue(body, "status"); got != "active" {
		t.Fatalf("status = %q, want the zone's default", got)
	}
	if got := fmValue(body, "title"); !strings.Contains(got, "Webhook retries") {
		t.Fatalf("title = %q, want the derived title", got)
	}
	if !strings.Contains(body, "## Resolution") {
		t.Fatal("the template body must carry the rule-required headings")
	}
	if !strings.Contains(out, "quirks/webhook-retries.md") {
		t.Fatalf("output must name the path, got %s", out)
	}
}

func TestNewInvestigationPrependsToday(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "investigation", "cache-thrash"); ec != 0 {
		t.Fatal("investigation creation failed")
	}
	want := filepath.Join(kb, "investigations", time.Now().Format("2006-01-02")+"-cache-thrash.md")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("investigation must land as %s per PYLON030: %v", filepath.Base(want), err)
	}
}

func TestNewProposedPlanLandsInProposals(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "plan", "feature-lane-swap"); ec != 0 {
		t.Fatal("plan creation failed")
	}
	if _, err := os.Stat(filepath.Join(kb, "plans", "proposals", "feature-lane-swap.md")); err != nil {
		t.Fatal("a proposed plan must land in plans/proposals/ per PYLON007")
	}
}

func TestNewRefusesOverwrite(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "quirk", "dup"); ec != 0 {
		t.Fatal("first creation must succeed")
	}
	out, ec := runNew(t, kb, "quirk", "dup")
	if ec != 2 {
		t.Fatalf("overwrite must be a usage error, got exit %d: %s", ec, out)
	}
}

func TestNewUnknownTypeListsTypes(t *testing.T) {
	kb := newSandboxKB(t)
	out, ec := runNew(t, kb, "gadget", "x")
	if ec != 2 {
		t.Fatalf("unknown type must exit 2, got %d", ec)
	}
	if !strings.Contains(out, "quirk") {
		t.Fatalf("error must list the valid types, got %s", out)
	}
}

func TestNewRejectsDirtyNames(t *testing.T) {
	kb := newSandboxKB(t)
	for _, name := range []string{"Web Hook", "../escape", "-lead", "a_b"} {
		if _, ec := runNew(t, kb, "quirk", name); ec != 2 {
			t.Fatalf("name %q must be refused as a usage error, got exit %d", name, ec)
		}
	}
}

func TestNewTitleFlag(t *testing.T) {
	kb := newSandboxKB(t)
	out, ec := runNew(t, kb, "quirk", "titled", "--title", "Webhook retries storm")
	if ec != 0 {
		t.Fatalf("exit %d: %s", ec, out)
	}
	body, _ := os.ReadFile(filepath.Join(kb, "quirks", "titled.md"))
	if got := fmValue(string(body), "title"); got != `"Webhook retries storm"` {
		t.Fatalf("title = %q, want the flagged title quoted", got)
	}
}

// The type table must cover every shipped template, so a new template
// can never ship without pylon new knowing it.
func TestNewCoversEveryTemplate(t *testing.T) {
	entries, err := os.ReadDir("../../../templates")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "AGENTS.md" {
			continue
		}
		known[strings.TrimSuffix(e.Name(), ".md")] = true
	}
	for _, docType := range newDocTypes {
		delete(known, docType.template)
	}
	if len(known) > 0 {
		t.Fatalf("templates without a pylon new entry: %v", known)
	}
}
