// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"strings"
	"testing"
)

func strictParse(t *testing.T, front string) (DocMeta, bool) {
	t.Helper()
	var m DocMeta
	ok := ParseDocMeta("---\n"+front+"\n---\n\nbody\n", &m)
	return m, ok
}

func TestStrictRejectsAnchor(t *testing.T) {
	m, ok := strictParse(t, "title: leak\ntype: quirk\nstatus: active\nowners: &shared [Pylons]\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatal("anchors must fail closed")
	}
	if !containsAll(m.ParseErrors, "anchor") {
		t.Fatalf("parse errors must name the anchor, got %v", m.ParseErrors)
	}
}

func TestStrictRejectsAlias(t *testing.T) {
	m, ok := strictParse(t, "title: *ref\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatal("aliases must fail closed")
	}
	if !containsAll(m.ParseErrors, "anchor") {
		t.Fatalf("an alias has no anchor to resolve, so the error names the anchor: %v", m.ParseErrors)
	}
}

func TestStrictRejectsMergeKey(t *testing.T) {
	m, ok := strictParse(t, "<<: {a: 1}\ntitle: x\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatal("merge keys must fail closed")
	}
	if !containsAll(m.ParseErrors, "merge") {
		t.Fatalf("parse errors must name the merge key, got %v", m.ParseErrors)
	}
}

func TestStrictRejectsCustomTag(t *testing.T) {
	m, ok := strictParse(t, "title: !secret hunter2\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatal("custom tags must fail closed")
	}
	if !containsAll(m.ParseErrors, "tag") {
		t.Fatalf("parse errors must name the custom tag, got %v", m.ParseErrors)
	}
}

func TestSyntaxErrorNamesLine(t *testing.T) {
	m, ok := strictParse(t, "title: [unclosed\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatal("broken yaml must fail closed")
	}
	if !containsAll(m.ParseErrors, "line") {
		t.Fatalf("parse errors must carry a line, got %v", m.ParseErrors)
	}
}

func TestUnknownKeyCollectedPerType(t *testing.T) {
	m, ok := strictParse(t, "title: q\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\nwat: 1")
	if !ok {
		t.Fatalf("unknown top-level keys must not block: %v", m.ParseErrors)
	}
	if len(m.Unknown) != 1 || m.Unknown[0] != "wat" {
		t.Fatalf("expected unknown key wat, got %v", m.Unknown)
	}
	clean, ok := strictParse(t, "title: q\ntype: quirk\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if !ok || len(clean.Unknown) != 0 {
		t.Fatalf("clean quirk must carry no unknown keys, got %v", clean.Unknown)
	}
}

func TestPlanKeysUnknownOnOtherTypes(t *testing.T) {
	m, ok := strictParse(t, "title: d\ntype: domain-entity\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\ncomponents: [svc]\nmachines: []\navoid: [no]\ndescription: the entity\nslice: true\nscenarios: []")
	if !ok {
		t.Fatalf("plan keys on an entity must stay advisory, not blocking: %v", m.ParseErrors)
	}
	joined := strings.Join(m.Unknown, ",")
	if !strings.Contains(joined, "slice") || !strings.Contains(joined, "scenarios") {
		t.Fatalf("plan-only keys must be unknown on a domain entity, got %v", m.Unknown)
	}
	if strings.Contains(joined, "machines") || strings.Contains(joined, "avoid") || strings.Contains(joined, "description") {
		t.Fatalf("entity keys must be known, got %v", m.Unknown)
	}
}

func TestClosedMachinePayload(t *testing.T) {
	m, ok := strictParse(t, "title: d\ntype: domain-entity\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\nmachines:\n  - machine: ship\n    states: [pending, done]\n    initial: [pending]\n    terminal: [done]\n    bogus: 1")
	if ok {
		t.Fatal("unknown machine keys must fail closed")
	}
	if !containsAll(m.ParseErrors, "bogus") {
		t.Fatalf("parse errors must name the key, got %v", m.ParseErrors)
	}
}

func TestClosedTransitionPayload(t *testing.T) {
	m, ok := strictParse(t, "title: d\ntype: domain-entity\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\nmachines:\n  - machine: ship\n    states: [pending, done]\n    initial: [pending]\n    terminal: [done]\n    transitions:\n      - from: pending\n        event: ship\n        to: done\n        bogus: 1")
	if ok {
		t.Fatal("unknown transition keys must fail closed")
	}
	if !containsAll(m.ParseErrors, "bogus") {
		t.Fatalf("parse errors must name the key, got %v", m.ParseErrors)
	}
}

func TestClosedContractEntryPayload(t *testing.T) {
	m, ok := strictParse(t, "title: p\ntype: plan\nstatus: accepted\nticket: [EX-1]\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\ncontract:\n  stable:\n    - kind: go-schema\n      id: repo://svc/schema.sql::tables\n      bogus: 1")
	if ok {
		t.Fatal("unknown contract keys must fail closed")
	}
	if !containsAll(m.ParseErrors, "bogus") {
		t.Fatalf("parse errors must name the key, got %v", m.ParseErrors)
	}
}

func TestClosedScenarioPayload(t *testing.T) {
	m, ok := strictParse(t, "title: p\ntype: plan\nstatus: accepted\nticket: [EX-1]\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\nscenarios:\n  - given: a shipped order\n    when: the webhook replays\n    expect: no duplicate event\n    bogus: 1")
	if ok {
		t.Fatal("unknown scenario keys must fail closed")
	}
	if !containsAll(m.ParseErrors, "bogus") {
		t.Fatalf("parse errors must name the key, got %v", m.ParseErrors)
	}
}

func containsAll(haystack []string, want string) bool {
	for _, s := range haystack {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
