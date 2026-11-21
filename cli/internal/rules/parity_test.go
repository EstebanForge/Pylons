// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// newParityFixture builds a KB registering one project root, with a
// domain entity declaring a state set and an optional symbol pointer.
func newParityFixture(t *testing.T, lang, source, states, symbol string) (kb, root string) {
	t.Helper()
	kb = t.TempDir()
	root = t.TempDir()
	mk := func(name, body string) {
		mustWrite(t, filepath.Join(root, name), body)
	}
	switch lang {
	case "go":
		mk("go.mod", "module example.com/svc\n")
		mk("state.go", source)
	case "rust":
		mk("Cargo.toml", "[package]\nname = \"svc\"\nversion = \"0.1.0\"\n")
		mk("state.rs", source)
	case "typescript":
		mk("package.json", "{\n  \"name\": \"svc\"\n}\n")
		mk("state.ts", source)
	case "php":
		mk("composer.json", "{\n  \"name\": \"svc/svc\"\n}\n")
		mk("state.php", source)
	}
	exts := map[string]string{"go": "go", "rust": "rust", "typescript": "typescript", "php": "php"}[lang]
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: ["+exts+"]\nprojects:\n  svc:\n    path: "+root+"\n    ref: main\n")
	fm := "---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\nstates: " + states + "\nstates_symbol: \"" + symbol + "\"\ntags: [domain]\n---\n\n# Message\n"
	mustWrite(t, filepath.Join(kb, "domain/message.md"), fm)
	return kb, root
}

// writeDomainMessage writes the shared domain/message.md entity with a
// state set and a symbol pointer.
func writeDomainMessage(t *testing.T, kb, states, symbol string) {
	t.Helper()
	fm := "---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\nstates: " + states + "\nstates_symbol: \"" + symbol + "\"\ntags: [domain]\n---\n\n# Message\n"
	mustWrite(t, filepath.Join(kb, "domain/message.md"), fm)
}

func cfgMust(t *testing.T, kb string) config.Config {
	t.Helper()
	cfg, err := config.Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestParityGoStringConstStatesMatch(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("declared states must match the backing values, got %+v", f)
		}
	}
}

func TestParityGoIotaComparesNames(t *testing.T) {
	src := "package svc\n\ntype Status int\n\nconst (\n\tDraft Status = iota\n\tSent\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[Draft, Sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("an iota enum must compare case names, got %+v", f)
		}
	}
}

func TestParityCountAndStateMismatch(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n\tFailed Status = \"failed\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	findings := lintParity(kb, cfgMust(t, kb), false)
	var count, state bool
	for _, f := range findings {
		if f.RuleID == "PYLON050" {
			count = true
		}
		if f.RuleID == "PYLON051" && strings.Contains(f.Message, "failed") {
			state = true
		}
	}
	if !count || !state {
		t.Fatalf("a 3-case enum against 2 states must fire count and state mismatch, got %+v", findings)
	}
}

func TestParityRustEnum(t *testing.T) {
	src := "#[derive(Debug)]\npub enum Status {\n    Draft,\n    Sent,\n}\n"
	kb, _ := newParityFixture(t, "rust", src, "[Draft, Sent]", "repo://svc/state.rs::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a rust enum must compare variant names, got %+v", f)
		}
	}
}

func TestParityNotEvaluatedFailsClosed(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n)\n"
	// States declared with no symbol pointer.
	kb, _ := newParityFixture(t, "go", src, "[draft]", "")
	findings := lintParity(kb, cfgMust(t, kb), false)
	if len(findings) == 0 || findings[0].RuleID != "PYLON049" {
		t.Fatalf("states without a symbol must fail closed as not evaluated, got %+v", findings)
	}
	// A symbol naming an enum that does not exist must never read as parity.
	kb2, _ := newParityFixture(t, "go", src, "[draft]", "repo://svc/state.go::Missing")
	findings = lintParity(kb2, cfgMust(t, kb2), false)
	if len(findings) == 0 || findings[0].RuleID != "PYLON049" {
		t.Fatalf("an unresolvable enum must fail closed, got %+v", findings)
	}
}

func TestParityRouteMismatch(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n)\n"
	kb, root := newParityFixture(t, "go", src, "[draft]", "repo://svc/state.go::Status")
	fm := "---\ntitle: \"Svc\"\ntype: package\nstatus: active\ntier: core\npackage: svc\ncomponents: []\ndepends_on: []\nexposes:\n  - id: repo://svc/api/users\n    kind: route\n---\n\n# Svc\n"
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), fm)
	// The route path present in project source: silent.
	mustWrite(t, filepath.Join(root, "routes.go"), "package svc\n\n// GET /api/users\n")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON052" {
			t.Fatalf("a registered route must stay silent, got %+v", f)
		}
	}
	// Drop the registration: advisory.
	if err := os.Remove(filepath.Join(root, "routes.go")); err != nil {
		t.Fatal(err)
	}
	var route bool
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON052" {
			route = true
		}
	}
	if !route {
		t.Fatal("a route the source never mentions must fire the advisory")
	}
}

func TestParityTrailingCommentsParse(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\" // live\n\tSent Status = \"sent\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" || f.RuleID == "PYLON049" {
			t.Fatalf("a commented const line must still parse, got %+v", f)
		}
	}
}

func TestParityUnparsableLineFailsClosed(t *testing.T) {
	src := "package svc\n\nconst (\n\tDraft Status = \"draft\"\n\t???garbage???\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft]", "repo://svc/state.go::Status")
	findings := lintParity(kb, cfgMust(t, kb), false)
	if len(findings) == 0 || findings[0].RuleID != "PYLON049" {
		t.Fatalf("an unparseable case line must fail closed, got %+v", findings)
	}
}

func TestParitySecondConstBlock(t *testing.T) {
	src := "package svc\n\nconst (\n\tA Kind = iota\n)\n\nconst (\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" || f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("an enum in a later const block must be found, got %+v", f)
		}
	}
}

func TestParityBlankIdentifierSkipped(t *testing.T) {
	src := "package svc\n\nconst (\n\t_ Status = \"pad\"\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a padding blank identifier must not count as a case, got %+v", f)
		}
	}
}

func TestParityTSSingleQuotes(t *testing.T) {
	src := "export enum Status {\n  Draft = 'draft',\n  Sent = 'sent',\n}\n"
	kb, _ := newParityFixture(t, "typescript", src, "[draft, sent]", "repo://svc/state.ts::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" || f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("single-quoted enum values must compare, got %+v", f)
		}
	}
}

func TestParityRustNoTrailingComma(t *testing.T) {
	src := "#[derive(Debug)]\npub enum Status {\n    Draft,\n    Sent\n}\n"
	kb, _ := newParityFixture(t, "rust", src, "[Draft, Sent]", "repo://svc/state.rs::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a last variant without a comma must compare, got %+v", f)
		}
	}
}

func TestParityEmptyStringValueIsValued(t *testing.T) {
	src := "package svc\n\nconst (\n\tDraft Status = \"\"\n\tSent Status = \"sent\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	findings := lintParity(kb, cfgMust(t, kb), false)
	var count, emptyState, draftState bool
	for _, f := range findings {
		if f.RuleID == "PYLON049" {
			t.Fatalf("an empty-string value must not degrade to not-evaluated, got %+v", f)
		}
		if f.RuleID == "PYLON050" {
			count = true
		}
		if f.RuleID == "PYLON051" && strings.Contains(f.Message, "\"\"") {
			emptyState = true
		}
		if f.RuleID == "PYLON051" && strings.Contains(f.Message, "draft") {
			draftState = true
		}
	}
	if count || !emptyState || !draftState {
		t.Fatalf("an empty-string value must still compare by values (count mismatch off; both sides surface in the state mismatch), got %+v", findings)
	}
}

func TestParityDuplicateDeclaredStates(t *testing.T) {
	src := "package svc\n\nconst (\n\tDraft Status = \"draft\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, draft]", "repo://svc/state.go::Status")
	findings := lintParity(kb, cfgMust(t, kb), false)
	var count bool
	for _, f := range findings {
		if f.RuleID == "PYLON050" {
			count = true
		}
	}
	if !count {
		t.Fatalf("duplicate declared states must fire the count mismatch, got %+v", findings)
	}
}

func TestParityPhpEnum(t *testing.T) {
	src := "<?php\nenum Status: string {\n    case Draft = 'draft';\n    case Sent = 'sent';\n}\n"
	kb, _ := newParityFixture(t, "php", src, "[draft, sent]", "repo://svc/state.php::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a php enum must compare backing values, got %+v", f)
		}
	}
}

func TestParityProfileClaimsExtension(t *testing.T) {
	kb, root := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/svc\n")
	mustWrite(t, filepath.Join(root, "state.custom"),
		"package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n)\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), `version: 1
extensions: [go]
parity:
  version: 1
  profiles:
    go:
      use: builtin:go
      extensions:
        - .custom
projects:
  svc:
    path: `+root+`
    ref: main
`)
	writeDomainMessage(t, kb, "[draft, sent]", "repo://svc/state.custom::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a profile-claimed extension must extract, got %+v", f)
		}
	}
}

func TestParityExplicitSectionDisablesImplicit(t *testing.T) {
	kb, _ := newParityFixture(t, "go", "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n)\n",
		"[draft]", "repo://svc/state.go::Status")
	// Bind only rust: the go entity must no longer resolve implicitly.
	manifest := filepath.Join(kb, "manifests/pylons.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	withParity := strings.Replace(string(body), "version: 1\n", "version: 1\nparity:\n  version: 1\n  profiles:\n    rust:\n      use: builtin:rust\n", 1)
	mustWrite(t, manifest, withParity)
	var found bool
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" && strings.Contains(f.Message, "no parity profile binds go") {
			found = true
		}
	}
	if !found {
		t.Fatal("an unbound language under an explicit section must yield the not-evaluated advisory")
	}
}

func TestParityBuiltinWithoutExtractor(t *testing.T) {
	kb, root := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "state.py"), "class Status(enum.Enum):\n    DRAFT = 1\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), `version: 1
extensions: [python]
parity:
  version: 1
  profiles:
    python:
      use: builtin:python
projects:
  svc:
    path: `+root+`
    ref: main
`)
	writeDomainMessage(t, kb, "[DRAFT]", "repo://svc/state.py::Status")
	var found bool
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" && strings.Contains(f.Message, "no in-process extractor") {
			found = true
		}
	}
	if !found {
		t.Fatal("a builtin binding without an extractor must yield the not-evaluated advisory naming the gap")
	}
}

func newParityExternalFixture(t *testing.T, pluginBody, symbol string) (kb string, root string) {
	t.Helper()
	kb, root = t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "state.kt"), "enum class Status { DRAFT, SENT }\n")
	if err := os.MkdirAll(filepath.Join(kb, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "extensions", "pylon-ext-kotlin"), []byte(pluginBody), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), `version: 1
extensions: [kotlin]
parity:
  version: 1
  profiles:
    kotlin:
      use: external:kotlin
      extensions:
        - .kt
projects:
  svc:
    path: `+root+`
    ref: main
`)
	writeDomainMessage(t, kb, "[draft, sent]", symbol)
	return kb, root
}

func TestParityExternalProfileDisabledByDefault(t *testing.T) {
	kb, _ := newParityExternalFixture(t, "#!/bin/sh\nprintf '{\"cases\":[{\"name\":\"Draft\",\"value\":\"draft\"}]}'\n",
		"repo://svc/state.kt::Status")
	var found bool
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" && strings.Contains(f.Message, "external extensions disabled") {
			found = true
		}
	}
	if !found {
		t.Fatal("an external profile without the opt-in must yield the not-evaluated advisory with the cause")
	}
}

func TestParityExternalProfileExtracts(t *testing.T) {
	kb, _ := newParityExternalFixture(t, "#!/bin/sh\nIFS= read -r line\nprintf '{\"cases\":[{\"name\":\"Draft\",\"value\":\"draft\"},{\"name\":\"Sent\",\"value\":\"sent\"}]}'\n",
		"repo://svc/state.kt::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb), true) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" || f.RuleID == "PYLON049" {
			t.Fatalf("a working external profile must extract, got %+v", f)
		}
	}
}

func TestParityExternalPluginVerdictError(t *testing.T) {
	kb, _ := newParityExternalFixture(t, "#!/bin/sh\nprintf '{\"cases\":[],\"error\":\"cannot resolve Status\"}'\n",
		"repo://svc/state.kt::Status")
	var found bool
	for _, f := range lintParity(kb, cfgMust(t, kb), true) {
		if f.RuleID == "PYLON049" && strings.Contains(f.Message, "cannot resolve Status") {
			found = true
		}
	}
	if !found {
		t.Fatal("a plugin that reports it cannot resolve must surface its error, never read as parity")
	}
}

func TestParityExternalBadOutput(t *testing.T) {
	kb, _ := newParityExternalFixture(t, "#!/bin/sh\nprintf 'not json at all'\n", "repo://svc/state.kt::Status")
	var found bool
	for _, f := range lintParity(kb, cfgMust(t, kb), true) {
		if f.RuleID == "PYLON049" && strings.Contains(f.Message, "invalid extension JSON") {
			found = true
		}
	}
	if !found {
		t.Fatal("a plugin emitting garbage must yield the not-evaluated advisory with the cause")
	}
}
