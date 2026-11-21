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
	}
	exts := map[string]string{"go": "go", "rust": "rust", "typescript": "typescript"}[lang]
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: ["+exts+"]\nprojects:\n  svc:\n    path: "+root+"\n    ref: main\n")
	fm := "---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\nstates: " + states + "\nstates_symbol: \"" + symbol + "\"\ntags: [domain]\n---\n\n# Message\n"
	mustWrite(t, filepath.Join(kb, "domain/message.md"), fm)
	return kb, root
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
	for _, f := range lintParity(kb, cfgMust(t, kb)) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("declared states must match the backing values, got %+v", f)
		}
	}
}

func TestParityGoIotaComparesNames(t *testing.T) {
	src := "package svc\n\ntype Status int\n\nconst (\n\tDraft Status = iota\n\tSent\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[Draft, Sent]", "repo://svc/state.go::Status")
	for _, f := range lintParity(kb, cfgMust(t, kb)) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("an iota enum must compare case names, got %+v", f)
		}
	}
}

func TestParityCountAndStateMismatch(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n\tSent Status = \"sent\"\n\tFailed Status = \"failed\"\n)\n"
	kb, _ := newParityFixture(t, "go", src, "[draft, sent]", "repo://svc/state.go::Status")
	findings := lintParity(kb, cfgMust(t, kb))
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
	for _, f := range lintParity(kb, cfgMust(t, kb)) {
		if f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("a rust enum must compare variant names, got %+v", f)
		}
	}
}

func TestParityNotEvaluatedFailsClosed(t *testing.T) {
	src := "package svc\n\ntype Status string\n\nconst (\n\tDraft Status = \"draft\"\n)\n"
	// States declared with no symbol pointer.
	kb, _ := newParityFixture(t, "go", src, "[draft]", "")
	findings := lintParity(kb, cfgMust(t, kb))
	if len(findings) == 0 || findings[0].RuleID != "PYLON049" {
		t.Fatalf("states without a symbol must fail closed as not evaluated, got %+v", findings)
	}
	// A symbol naming an enum that does not exist must never read as parity.
	kb2, _ := newParityFixture(t, "go", src, "[draft]", "repo://svc/state.go::Missing")
	findings = lintParity(kb2, cfgMust(t, kb2))
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
	for _, f := range lintParity(kb, cfgMust(t, kb)) {
		if f.RuleID == "PYLON052" {
			t.Fatalf("a registered route must stay silent, got %+v", f)
		}
	}
	// Drop the registration: advisory.
	if err := os.Remove(filepath.Join(root, "routes.go")); err != nil {
		t.Fatal(err)
	}
	var route bool
	for _, f := range lintParity(kb, cfgMust(t, kb)) {
		if f.RuleID == "PYLON052" {
			route = true
		}
	}
	if !route {
		t.Fatal("a route the source never mentions must fire the advisory")
	}
}
