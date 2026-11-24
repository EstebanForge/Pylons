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
	repoTemplates := "../../../pylons/templates"
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

func TestNewDecisionNumberAssignsNext(t *testing.T) {
	kb := newSandboxKB(t)
	dec := filepath.Join(kb, "decisions")
	if err := os.MkdirAll(filepath.Join(dec, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"D0001-first.md", "D0003-third.md", filepath.Join("archive", "D0002-old.md")} {
		if err := os.WriteFile(filepath.Join(dec, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, ec := runNew(t, kb, "decision", "use-ledger-sync", "--number")
	if ec != 0 {
		t.Fatalf("exit %d: %s", ec, out)
	}
	if _, err := os.Stat(filepath.Join(kb, "decisions", "proposals", "D0004-use-ledger-sync.md")); err != nil {
		t.Fatalf("the next free number must be assigned: %v\n%s", err, out)
	}
}

func TestNewDecisionNumberHonorsPrefix(t *testing.T) {
	kb := newSandboxKB(t)
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\ndecision_prefix: A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dec := filepath.Join(kb, "decisions")
	if err := os.MkdirAll(dec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dec, "A0007-prior.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ec := runNew(t, kb, "decision", "event-sourcing", "--number"); ec != 0 {
		t.Fatal("numbered creation failed")
	}
	if _, err := os.Stat(filepath.Join(kb, "decisions", "proposals", "A0008-event-sourcing.md")); err != nil {
		t.Fatal("the configured prefix must drive the numbering")
	}
}

func TestNewDecisionNumberOnlyForDecisions(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "quirk", "numbered", "--number"); ec != 2 {
		t.Fatalf("--number outside decision must be a usage error, got exit %d", ec)
	}
}

func TestNewDecisionNumberRefusesExhausted(t *testing.T) {
	kb := newSandboxKB(t)
	dec := filepath.Join(kb, "decisions")
	if err := os.MkdirAll(dec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dec, "D9999-last.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ec := runNew(t, kb, "decision", "one-more", "--number"); ec != 2 {
		t.Fatalf("exhausted numbers must be a usage error, got exit %d", ec)
	}
}

// The type table must cover every shipped template, so a new template
// can never ship without pylon new knowing it.
func TestNewCoversEveryTemplate(t *testing.T) {
	entries, err := os.ReadDir("../../../pylons/templates")
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
		if _, err := os.Stat(filepath.Join("../../../pylons/templates", docType.template+".md")); err != nil {
			t.Errorf("table entry %s has no template: %v", docType.typ, err)
		}
		delete(known, docType.template)
	}
	if len(known) > 0 {
		t.Fatalf("templates without a pylon new entry: %v", known)
	}
}

func TestNewDecisionPrefixFailsClosed(t *testing.T) {
	kb := newSandboxKB(t)
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte("version: 1\ndecision_prefix: \"../../evil\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ec := runNew(t, kb, "decision", "escape", "--number")
	if ec != 2 {
		t.Fatalf("a hostile prefix must be a usage error, got exit %d: %s", ec, out)
	}
	if _, err := os.Stat(filepath.Join(kb, "decisions", "proposals", "escape.md")); !os.IsNotExist(err) {
		t.Fatal("nothing may be written when the prefix is hostile")
	}
	if _, err := os.Stat(filepath.Dir(filepath.Join(kb, "..", "evil0001-escape.md"))); err == nil {
		t.Log("checked outside path")
	}
}

func TestNewTitleRejectsControlAndBadUTF8(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "quirk", "injected", "--title", "line1\nline2: x"); ec != 2 {
		t.Fatalf("a newline title must be refused, got exit %d", ec)
	}
	if _, err := os.Stat(filepath.Join(kb, "quirks", "injected.md")); !os.IsNotExist(err) {
		t.Fatal("nothing may be written for a rejected title")
	}
	if _, ec := runNew(t, kb, "quirk", "injected2", "--title", "bad\xffutf8"); ec != 2 {
		t.Fatalf("invalid UTF-8 title must be refused, got exit %d", ec)
	}
}

func TestNewNumberFollowsSymlinkedDecisions(t *testing.T) {
	kb := newSandboxKB(t)
	real := filepath.Join(kb, "real-decisions")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "D0077-z.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(kb, "decisions")); err != nil {
		t.Fatal(err)
	}
	if _, ec := runNew(t, kb, "decision", "through-link", "--number"); ec != 0 {
		t.Fatal("numbered creation failed")
	}
	if _, err := os.Stat(filepath.Join(real, "proposals", "D0078-through-link.md")); err != nil {
		t.Fatal("the scan must follow a symlinked decisions directory")
	}
}

func TestNewPlanRequiresTypePrefix(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "plan", "big-plan"); ec != 2 {
		t.Fatalf("an unprefixed plan must be refused, got exit %d", ec)
	}
	if _, ec := runNew(t, kb, "plan", "feature-big-plan"); ec != 0 {
		t.Fatal("a prefixed plan must be created")
	}
}

func TestNewMalformedTemplateFailsClosed(t *testing.T) {
	kb := newSandboxKB(t)
	if err := os.WriteFile(filepath.Join(kb, "templates", "quirk.md"), []byte("---\r\ntitle: \"{{Quirk title}}\"\r\n---\r\nbody\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ec := runNew(t, kb, "quirk", "crlf"); ec != 2 {
		t.Fatalf("a CRLF template must fail closed, got exit %d", ec)
	}
	if err := os.WriteFile(filepath.Join(kb, "templates", "quirk.md"), []byte("no frontmatter at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ec := runNew(t, kb, "quirk", "nofm"); ec != 2 {
		t.Fatalf("a frontmatter-less template must fail closed, got exit %d", ec)
	}
}

func TestNewNumberIgnoresNonCanonicalFiles(t *testing.T) {
	kb := newSandboxKB(t)
	dec := filepath.Join(kb, "decisions")
	if err := os.MkdirAll(dec, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"D12345678-sixdigit.md", "D+500-signed.md"} {
		if err := os.WriteFile(filepath.Join(dec, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, ec := runNew(t, kb, "decision", "clean", "--number"); ec != 0 {
		t.Fatal("numbered creation failed")
	}
	if _, err := os.Stat(filepath.Join(dec, "proposals", "D0001-clean.md")); err != nil {
		t.Fatal("non-canonical decision files must not inflate the numbering")
	}
}

func TestNewTightensSlugNames(t *testing.T) {
	kb := newSandboxKB(t)
	for _, name := range []string{"x-", "a--b"} {
		if _, ec := runNew(t, kb, "quirk", name); ec != 2 {
			t.Fatalf("name %q must be refused, got exit %d", name, ec)
		}
	}
}

func TestNewRefusesExtraArgs(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "quirk", "x", "extra"); ec != 2 {
		t.Fatalf("extra arguments must be a usage error, got exit %d", ec)
	}
}

func TestNewMissingTemplateIsIOError(t *testing.T) {
	kb := newSandboxKB(t)
	if err := os.RemoveAll(filepath.Join(kb, "templates")); err != nil {
		t.Fatal(err)
	}
	out, ec := runNew(t, kb, "quirk", "none")
	if ec != 1 {
		t.Fatalf("a missing template is an environment error, got exit %d", ec)
	}
	if !strings.Contains(out, "templates") {
		t.Fatalf("the error must point at templates/, got %s", out)
	}
}

func TestNewFillsTitlePlaceholdersInBody(t *testing.T) {
	kb := newSandboxKB(t)
	if _, ec := runNew(t, kb, "quirk", "storm-drain", "--title", "Storm drain"); ec != 0 {
		t.Fatal("creation failed")
	}
	body, _ := os.ReadFile(filepath.Join(kb, "quirks", "storm-drain.md"))
	if strings.Contains(string(body), "{{Quirk title}}") {
		t.Fatal("the template title placeholder must be substituted in the body")
	}
	if !strings.Contains(string(body), "# Storm drain") {
		t.Fatal("the body heading must carry the title")
	}
}
