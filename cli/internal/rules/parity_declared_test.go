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

// machineDeclFixture is a valid shipment declaration; tests mutate it.
func machineDeclFixture(mutate func(*resolve.MachineDecl)) resolve.MachineDecl {
	m := resolve.MachineDecl{
		Machine:           "shipment",
		States:            []string{"pending", "picked", "shipped"},
		Initial:           []string{"pending"},
		Terminal:          []string{"shipped"},
		StatesSymbol:      "repo://svc/state.go::Status",
		TransitionsSymbol: "repo://svc/state.go::shipment",
		Transitions: []resolve.TransitionDecl{
			{From: "pending", Event: "pick", To: "picked"},
			{From: "picked", Event: "ship", To: "shipped"},
		},
	}
	if mutate != nil {
		mutate(&m)
	}
	return m
}

func entityDoc(rel string, machines []resolve.MachineDecl) corpusDoc {
	fm := "---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\ntags: [domain]\n---\n\n# Message\n"
	return corpusDoc{rel: rel, content: fm, meta: resolve.DocMeta{Type: "domain-entity", Machines: machines}}
}

func ruleIDs(fs []report.Finding) map[string]int {
	out := map[string]int{}
	for _, f := range fs {
		out[f.RuleID]++
	}
	return out
}

func TestMachineMalformedDeclFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*resolve.MachineDecl)
	}{
		{"no id", func(m *resolve.MachineDecl) { m.Machine = "" }},
		{"no initial", func(m *resolve.MachineDecl) { m.Initial = nil }},
		{"no terminal", func(m *resolve.MachineDecl) { m.Terminal = nil }},
		{"no states", func(m *resolve.MachineDecl) { m.States = nil }},
		{"bad triple", func(m *resolve.MachineDecl) { m.Transitions[0].Event = "" }},
		{"bad knob", func(m *resolve.MachineDecl) { m.OnUnevaluated = "sometimes" }},
	}
	for _, tc := range cases {
		docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(tc.mut)})}
		fs := scanMachineDeclarations(docs)
		if _, ok := ruleIDs(fs)["PYLON053"]; !ok {
			t.Fatalf("%s: malformed declaration must yield PYLON053, got %v", tc.name, ruleIDs(fs))
		}
	}
}

func TestMachineDuplicateIDRepoWide(t *testing.T) {
	docs := []corpusDoc{
		entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(nil)}),
		entityDoc("domain/other.md", []resolve.MachineDecl{machineDeclFixture(nil)}),
	}
	if _, ok := ruleIDs(scanMachineDeclarations(docs))["PYLON053"]; !ok {
		t.Fatal("a machine id declared twice in the repo must yield PYLON053")
	}
}

func TestMachineUnknownStateInTriple(t *testing.T) {
	docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(func(m *resolve.MachineDecl) {
		m.Transitions[0].To = "vanished"
	})})}
	if _, ok := ruleIDs(scanMachineDeclarations(docs))["PYLON055"]; !ok {
		t.Fatal("a triple naming an undeclared state must yield PYLON055")
	}
}

func TestMachineUnreachableState(t *testing.T) {
	docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(func(m *resolve.MachineDecl) {
		m.States = []string{"pending", "picked", "shipped", "stranded"}
	})})}
	if _, ok := ruleIDs(scanMachineDeclarations(docs))["PYLON056"]; !ok {
		t.Fatal("a state unreachable from initial must yield PYLON056")
	}
}

func TestMachineDeadState(t *testing.T) {
	// picked has no outgoing and is not terminal.
	docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(func(m *resolve.MachineDecl) {
		m.Transitions = m.Transitions[:1]
	})})}
	if _, ok := ruleIDs(scanMachineDeclarations(docs))["PYLON057"]; !ok {
		t.Fatal("a non-terminal state with no outgoing transitions must yield PYLON057")
	}
}

func TestMachineDuplicateDeclaredTriple(t *testing.T) {
	docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(func(m *resolve.MachineDecl) {
		m.Transitions = append(m.Transitions, m.Transitions[0])
	})})}
	if _, ok := ruleIDs(scanMachineDeclarations(docs))["PYLON058"]; !ok {
		t.Fatal("a duplicated declared triple must yield PYLON058")
	}
}

func TestMachineCleanDeclarationIsSilent(t *testing.T) {
	docs := []corpusDoc{entityDoc("domain/message.md", []resolve.MachineDecl{machineDeclFixture(nil)})}
	if fs := scanMachineDeclarations(docs); len(fs) != 0 {
		t.Fatalf("a clean declaration must stay silent, got %v", fs)
	}
}

// machineKB builds a ledger with one entity declaring machines and one
// registered project carrying the Go machine source.
func machineKB(t *testing.T, source string, mutate func(*resolve.MachineDecl)) string {
	t.Helper()
	kb, root := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/svc\n")
	mustWrite(t, filepath.Join(root, "state.go"), source)
	decl := machineDeclFixture(mutate)
	var b strings.Builder
	b.WriteString("---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\nmachines:\n  - machine: " + decl.Machine + "\n")
	if len(decl.States) > 0 {
		b.WriteString("    states: [" + strings.Join(decl.States, ", ") + "]\n")
	}
	b.WriteString("    initial: [" + strings.Join(decl.Initial, ", ") + "]\n")
	b.WriteString("    terminal: [" + strings.Join(decl.Terminal, ", ") + "]\n")
	if decl.StatesSymbol != "" {
		b.WriteString("    states_symbol: \"" + decl.StatesSymbol + "\"\n")
	}
	if decl.TransitionsSymbol != "" {
		b.WriteString("    transitions_symbol: \"" + decl.TransitionsSymbol + "\"\n")
	}
	if decl.OnUnevaluated != "" {
		b.WriteString("    on_unevaluated: " + decl.OnUnevaluated + "\n")
	}
	if len(decl.Transitions) > 0 {
		b.WriteString("    transitions:\n")
		for _, tr := range decl.Transitions {
			b.WriteString("      - {from: " + tr.From + ", event: " + tr.Event + ", to: " + tr.To + "}\n")
		}
	}
	b.WriteString("tags: [domain]\n---\n\n# Message\n")
	mustWrite(t, filepath.Join(kb, "domain/message.md"), b.String())
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+root+"\n    ref: main\n")
	return kb
}

func hasRule(fs []report.Finding, id string) bool {
	for _, f := range fs {
		if f.RuleID == id {
			return true
		}
	}
	return false
}

func TestMachineCodeCompareMatch(t *testing.T) {
	kb := machineKB(t, goMachineSource, nil)
	fs := lintParity(kb, cfgMust(t, kb), false)
	for _, bad := range []string{"PYLON049", "PYLON050", "PYLON051", "PYLON053", "PYLON054", "PYLON058"} {
		if hasRule(fs, bad) {
			t.Fatalf("a matching machine must stay silent, got %v", fs)
		}
	}
}

func TestMachineCodeCompareMismatch(t *testing.T) {
	kb := machineKB(t, goMachineSource, func(m *resolve.MachineDecl) {
		m.Transitions = append(m.Transitions, resolve.TransitionDecl{From: "pending", Event: "hold", To: "picked"})
	})
	if !hasRule(lintParity(kb, cfgMust(t, kb), false), "PYLON054") {
		t.Fatal("a declared triple missing from code must yield PYLON054")
	}
}

func TestMachineCodeUnresolvable(t *testing.T) {
	kb := machineKB(t, "package svc\n\nvar nothing = 1\n", nil)
	if !hasRule(lintParity(kb, cfgMust(t, kb), false), "PYLON053") {
		t.Fatal("a code side without the marker must yield PYLON053")
	}
}

func TestMachineNoTransitionsSymbolAdvisory(t *testing.T) {
	kb := machineKB(t, goMachineSource, func(m *resolve.MachineDecl) { m.TransitionsSymbol = "" })
	var found *report.Finding
	for i := range lintParity(kb, cfgMust(t, kb), false) {
		if fs := lintParity(kb, cfgMust(t, kb), false); len(fs) > i && fs[i].RuleID == "PYLON049" {
			found = &fs[i]
			break
		}
	}
	if found == nil || found.Severity != report.SeverityAdvisory {
		t.Fatal("a machine without transitions_symbol must yield the advisory")
	}
}

func TestMachineNoTransitionsSymbolKnobError(t *testing.T) {
	kb := machineKB(t, goMachineSource, func(m *resolve.MachineDecl) {
		m.TransitionsSymbol = ""
		m.OnUnevaluated = "error"
	})
	var found *report.Finding
	for _, f := range lintParity(kb, cfgMust(t, kb), false) {
		if f.RuleID == "PYLON049" {
			found = &f
			break
		}
	}
	if found == nil || found.Severity != report.SeverityError {
		t.Fatal("on_unevaluated: error must upgrade the not-evaluated advisory")
	}
}

func TestMachineDuplicateExtractedTriple(t *testing.T) {
	src := strings.Replace(goMachineSource,
		"StatusPicked: {\n\t\tEventShip: StatusShipped,\n\t},",
		"StatusPending: {\n\t\tEventPick: StatusShipped,\n\t},", 1)
	kb := machineKB(t, src, nil)
	if !hasRule(lintParity(kb, cfgMust(t, kb), false), "PYLON058") {
		t.Fatal("a duplicated code map key must yield PYLON058")
	}
}

func TestMachineExternalTransitionsExtract(t *testing.T) {
	kb, root := newParityExternalFixture(t, "#!/bin/sh\nIFS= read -r line\nprintf '{\"triples\":[{\"from\":\"draft\",\"event\":\"send\",\"to\":\"sent\"}]}'\n",
		"repo://svc/state.kt::Status")
	// Re-point the entity at a machine declaration over the same project.
	fm := "---\ntitle: \"Message\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-20\nupdated: 2026-11-20\nowners: [o]\nmachines:\n  - machine: shipment\n    states: [draft, sent]\n    initial: [draft]\n    terminal: [sent]\n    transitions_symbol: \"repo://svc/state.kt::shipment\"\n    transitions:\n      - {from: draft, event: send, to: sent}\ntags: [domain]\n---\n\n# Message\n"
	mustWrite(t, filepath.Join(kb, "domain/message.md"), fm)
	_ = root
	fs := lintParity(kb, cfgMust(t, kb), true)
	for _, f := range fs {
		if f.RuleID == "PYLON053" || f.RuleID == "PYLON054" || f.RuleID == "PYLON049" {
			t.Fatalf("a working external transitions profile must compare silently, got %+v", f)
		}
	}
}
