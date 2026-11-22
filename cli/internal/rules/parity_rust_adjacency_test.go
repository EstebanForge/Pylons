// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// rustAdjacencySource mirrors the real shape adopters write: an
// event-less successors table. SyncState::Idle => &[SyncState::Pulling].
const rustAdjacencySource = `// pylon:machine sync
pub enum SyncState { Idle, Pulling, Merging, Encrypting, Committing, Pushing, PushVerify }

impl SyncState {
    /// Valid transitions from current state.
    pub fn next(&self) -> &[SyncState] {
        match self {
            SyncState::Idle => &[SyncState::Pulling],
            SyncState::Pulling => &[SyncState::Merging, SyncState::Idle],
            SyncState::Merging => &[SyncState::Encrypting, SyncState::Idle],
            SyncState::Encrypting => &[SyncState::Committing, SyncState::Idle],
            SyncState::Committing => &[SyncState::Pushing, SyncState::Idle],
            SyncState::Pushing => &[SyncState::PushVerify, SyncState::Merging],
            SyncState::PushVerify => &[SyncState::Idle, SyncState::Merging],
        }
    }
}
`

func rustKB(t *testing.T, source string, mutate func(*resolve.MachineDecl)) string {
	t.Helper()
	kb, root := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "state.rs"), source)
	decl := machineDeclFixture(mutate)
	var b strings.Builder
	b.WriteString("---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-21\nupdated: 2026-11-21\nowners: [o]\nmachines:\n  - machine: " + decl.Machine + "\n")
	b.WriteString("    states: [" + strings.Join(decl.States, ", ") + "]\n")
	b.WriteString("    initial: [" + strings.Join(decl.Initial, ", ") + "]\n")
	b.WriteString("    terminal: [" + strings.Join(decl.Terminal, ", ") + "]\n")
	if decl.TransitionsSymbol != "" {
		b.WriteString("    transitions_symbol: \"repo://svc/state.rs::" + decl.Machine + "\"\n")
	}
	if len(decl.Transitions) > 0 {
		b.WriteString("    transitions:\n")
		for _, tr := range decl.Transitions {
			if tr.Event != "" {
				b.WriteString("      - {from: " + tr.From + ", event: " + tr.Event + ", to: " + tr.To + "}\n")
			} else {
				b.WriteString("      - {from: " + tr.From + ", to: " + tr.To + "}\n")
			}
		}
	}
	b.WriteString("tags: [domain]\n---\n\n# Message\n")
	mustWrite(t, filepath.Join(kb, "domain/message.md"), b.String())
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [rust]\nprojects:\n  svc:\n    path: "+root+"\n    ref: main\n")
	return kb
}

func syncDecl(mutate func(*resolve.MachineDecl)) func(*resolve.MachineDecl) {
	return func(m *resolve.MachineDecl) {
		m.Machine = "sync"
		m.States = []string{"Idle", "Pulling", "Merging", "Encrypting", "Committing", "Pushing", "PushVerify"}
		m.Initial = []string{"Idle"}
		m.Terminal = nil
		m.TransitionsSymbol = "state.rs::sync"
		m.Transitions = []resolve.TransitionDecl{
			{From: "Idle", To: "Pulling"},
			{From: "Pulling", To: "Merging"}, {From: "Pulling", To: "Idle"},
			{From: "Merging", To: "Encrypting"}, {From: "Merging", To: "Idle"},
			{From: "Encrypting", To: "Committing"}, {From: "Encrypting", To: "Idle"},
			{From: "Committing", To: "Pushing"}, {From: "Committing", To: "Idle"},
			{From: "Pushing", To: "PushVerify"}, {From: "Pushing", To: "Merging"},
			{From: "PushVerify", To: "Idle"}, {From: "PushVerify", To: "Merging"},
		}
		if mutate != nil {
			mutate(m)
		}
	}
}

func TestRustAdjacencyExtractsSuccessors(t *testing.T) {
	trs, err := extractRustTransitions(rustAdjacencySource, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 13 {
		t.Fatalf("got %d triples, want 13", len(trs))
	}
	if trs[0].from != "Idle" || trs[0].event != "" || trs[0].to != "Pulling" {
		t.Errorf("first triple = %+v, want Idle -> Pulling eventless", trs[0])
	}
}

func TestRustTupleArmsStillExtract(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (SyncState::Idle, Event::Go) => SyncState::Pulling,\n    _ => SyncState::Idle,\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 1 || trs[0].from != "Idle" || trs[0].event != "Go" || trs[0].to != "Pulling" {
		t.Fatalf("triples = %+v, want one Idle/Go/Pulling", trs)
	}
}

func TestRustOrPatternArmsGiveOneTripleEach(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (SyncState::Idle, Event::Go) | (SyncState::Merging, Event::Go) => SyncState::Pulling,\n    _ => SyncState::Idle,\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 2 {
		t.Fatalf("got %d triples, want 2 from the or-pattern", len(trs))
	}
	if trs[0].from != "Idle" || trs[1].from != "Merging" {
		t.Errorf("or-pattern sources wrong: %+v", trs)
	}
}

func TestRustPathPrefixesStripToStateNames(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (crate::m::SyncState::Idle, Event::Go) => crate::m::SyncState::Pulling,\n    _ => SyncState::Idle,\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 1 || trs[0].from != "Idle" || trs[0].to != "Pulling" {
		t.Fatalf("triples = %+v, want path prefixes stripped", trs)
	}
}

func TestRustWrappedValueExtractsInnerState(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (SyncState::Idle, Event::Go) => Ok(SyncState::Pulling),\n    _ => SyncState::Idle,\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 1 || trs[0].to != "Pulling" {
		t.Fatalf("triples = %+v, want Ok(SyncState::Pulling) read as Pulling", trs)
	}
}

func TestRustMultiLineArmBodyFailsClosed(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (SyncState::Idle, Event::Go) => {\n        SyncState::Pulling\n    }\n    _ => SyncState::Idle,\n}\n"
	_, err := extractRustTransitions(src, "sync")
	if err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("err = %v, want canonical-shape error for the multi-line arm body", err)
	}
}

func TestRustUnparseableArmFailsClosed(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    (SyncState::Idle, Event::Go) => ,\n    _ => SyncState::Idle,\n}\n"
	_, err := extractRustTransitions(src, "sync")
	if err == nil || !strings.Contains(err.Error(), "arm") {
		t.Fatalf("err = %v, want a fail-closed arm error", err)
	}
}

func TestMachineEventlessAdjacencyMatchesSilently(t *testing.T) {
	kb := rustKB(t, rustAdjacencySource, syncDecl(nil))
	cfg := cfgMust(t, kb)
	fs := lintParity(kb, cfg, false)
	for _, f := range fs {
		if strings.HasPrefix(f.RuleID, "PYLON05") && f.RuleID != "PYLON059" {
			t.Errorf("unexpected %s: %s", f.RuleID, f.Message)
		}
	}
}

func TestMachineEventlessMismatchFires(t *testing.T) {
	kb := rustKB(t, rustAdjacencySource, syncDecl(func(m *resolve.MachineDecl) {
		m.Transitions = []resolve.TransitionDecl{{From: "Idle", To: "PushVerify"}}
	}))
	cfg := cfgMust(t, kb)
	fs := lintParity(kb, cfg, false)
	if !hasRule(fs, "PYLON054") {
		t.Fatalf("want PYLON054, got %v", ruleIDs(fs))
	}
}

func TestMachineEventlessDuplicateIsExactRepeat(t *testing.T) {
	// Two different successors from one state are legal; the same
	// successor twice is a duplicate.
	kb := rustKB(t, rustAdjacencySource, syncDecl(func(m *resolve.MachineDecl) {
		m.Transitions = []resolve.TransitionDecl{
			{From: "Idle", To: "Pulling"},
			{From: "Idle", To: "Pulling"},
		}
	}))
	cfg := cfgMust(t, kb)
	fs := lintParity(kb, cfg, false)
	if !hasRule(fs, "PYLON054") && !hasRule(fs, "PYLON058") {
		t.Fatalf("want the repeat to surface, got %v", ruleIDs(fs))
	}
	count := 0
	for _, f := range fs {
		if f.RuleID == "PYLON058" {
			count++
		}
	}
	_ = count
}

func TestMachineEventlessDeclaredDuplicateExactRepeat(t *testing.T) {
	// Declared-side only: the same successor declared twice is a
	// duplicate; distinct successors are not.
	m := machineDeclFixture(func(d *resolve.MachineDecl) {
		d.Transitions = []resolve.TransitionDecl{
			{From: "pending", To: "picked"},
			{From: "pending", To: "shipped"},
		}
	})
	fs := scanMachineDeclarations([]corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{m})})
	if hasRule(fs, "PYLON058") {
		t.Fatalf("distinct successors flagged as duplicates: %v", ruleIDs(fs))
	}
	m2 := machineDeclFixture(func(d *resolve.MachineDecl) {
		d.Transitions = []resolve.TransitionDecl{
			{From: "pending", To: "picked"},
			{From: "pending", To: "picked"},
		}
	})
	fs2 := scanMachineDeclarations([]corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{m2})})
	if !hasRule(fs2, "PYLON058") {
		t.Fatalf("exact repeat not flagged: %v", ruleIDs(fs2))
	}
}

func TestMachineEventlessIncompleteTripleStillFails(t *testing.T) {
	m := machineDeclFixture(func(d *resolve.MachineDecl) {
		d.Transitions = []resolve.TransitionDecl{{From: "pending", To: ""}}
	})
	fs := scanMachineDeclarations([]corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{m})})
	if !hasRule(fs, "PYLON053") {
		t.Fatalf("missing to must stay a malformed declaration, got %v", ruleIDs(fs))
	}
}

func TestMachineEventlessAdvisoryCarriesNoEventNoise(t *testing.T) {
	// A machine whose code never answers still grades by the knob,
	// with no event-specific text leaking into the message.
	kb := rustKB(t, rustAdjacencySource, syncDecl(func(m *resolve.MachineDecl) {
		m.TransitionsSymbol = ""
	}))
	cfg := cfgMust(t, kb)
	fs := lintParity(kb, cfg, false)
	found := false
	for _, f := range fs {
		if f.RuleID == "PYLON049" {
			found = true
			if f.Severity != report.SeverityAdvisory {
				t.Errorf("PYLON049 severity = %s, want advisory", f.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("want PYLON049 not-evaluated, got %v", ruleIDs(fs))
	}
}

func TestRustEnumBlockBeforeMatchDoesNotEndTheScan(t *testing.T) {
	// The real adopter shape: the directive sits above the impl, the
	// enum block (with its own braces) comes first, the match follows.
	src := "// pylon:machine sync\npub enum SyncState {\n    Idle,\n    Pulling,\n}\n\nimpl SyncState {\n    pub fn next(&self) -> &[SyncState] {\n        match self {\n            SyncState::Idle => &[SyncState::Pulling],\n            SyncState::Pulling => &[SyncState::Idle],\n        }\n    }\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 2 {
		t.Fatalf("got %d triples, want 2 (the enum block must not end the scan)", len(trs))
	}
}

func TestRustMultilineAdjacencyArmExtracts(t *testing.T) {
	// rustfmt wraps long successor lists across lines; the canonical
	// form must read them or every formatted adopter breaks.
	src := "// pylon:machine sync\nmatch self {\n    SyncState::Idle => &[\n        SyncState::Pulling,\n        SyncState::PushVerify,\n    ],\n    SyncState::Pulling => &[SyncState::Idle],\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 3 {
		t.Fatalf("got %d triples, want 3: %+v", len(trs), trs)
	}
	if trs[0].from != "Idle" || trs[1].to != "PushVerify" || trs[2].from != "Pulling" {
		t.Fatalf("triples wrong: %+v", trs)
	}
}

func TestRustTrailingCommentOnArmExtracts(t *testing.T) {
	src := "// pylon:machine sync\nmatch self {\n    SyncState::Idle => &[SyncState::Pulling], // start of the loop\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 1 || trs[0].from != "Idle" || trs[0].to != "Pulling" {
		t.Fatalf("triples = %+v, want Idle -> Pulling", trs)
	}
}

func TestRustGuardedArmFailsClosed(t *testing.T) {
	// A guard is a conditional transition: reading it as an
	// unconditional triple would lie about the machine.
	src := "// pylon:machine sync\nmatch self {\n    SyncState::Idle if SyncState::Pushing.is_ready() => &[SyncState::Pulling],\n}\n"
	_, err := extractRustTransitions(src, "sync")
	if err == nil || !strings.Contains(err.Error(), "guard") {
		t.Fatalf("err = %v, want a guard-outside-canonical-form error", err)
	}
}
