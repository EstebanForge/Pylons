// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestContractEntryExternalKeyIsKnown(t *testing.T) {
	m, ok := strictParse(t, "title: p\ntype: plan\nstatus: accepted\nticket: [T-1]\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\ncontract:\n  stable:\n    - kind: go-symbol\n      id: repo://app/svc.go::Run\n      external: true")
	if !ok {
		t.Fatalf("external is read by the checks and must be a known contract key: %v", m.ParseErrors)
	}
}

func TestAliasReferencingKeyAnchorRejected(t *testing.T) {
	m, ok := strictParse(t, "&k title: q\ntype: quirk\nb: *k\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]")
	if ok {
		t.Fatalf("an alias resolving a key anchor must fail closed, got %+v", m)
	}
	if !containsAll(m.ParseErrors, "anchor") {
		t.Fatalf("expected an anchor rejection, got %v", m.ParseErrors)
	}
}

func TestEngineReadKeysKnownOnEveryType(t *testing.T) {
	// links and supersedes are read on every doc by the graph rules,
	// so no type may flag them unknown (the fix hint would trip
	// PYLON004).
	for typ := range TemplateKeys {
		set := knownKeySet(typ)
		for _, want := range engineReadKeys {
			if !set[want] {
				t.Errorf("%s: known keys lack engine-read key %s", typ, want)
			}
		}
	}
}

func TestTemplateKeysCoverTheirTemplates(t *testing.T) {
	entries, err := os.ReadDir("../../../pylons/templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "AGENTS.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("../../../pylons/templates", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		front := ExtractFrontMatter(string(raw))
		if front == "" {
			continue
		}
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(front), &node); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if len(node.Content) == 0 {
			continue
		}
		doc := node.Content[0]
		typ := ""
		for i := 0; i+1 < len(doc.Content); i += 2 {
			if doc.Content[i].Value == "type" {
				typ = doc.Content[i+1].Value
			}
		}
		if typ == "" {
			continue
		}
		known := knownKeySet(typ)
		for _, k := range mappingKeys(doc) {
			if !known[k] {
				t.Errorf("%s (type %s): template declares key %q that TemplateKeys lacks", e.Name(), typ, k)
			}
		}
	}
}

func TestStructuralLineNumbersAreFileRelative(t *testing.T) {
	// The frontmatter body starts at file line 2, so an anchor on the
	// first frontmatter line must be reported at file line 2.
	m, ok := strictParse(t, "title: q\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\nowners: &shared [a]")
	if ok {
		t.Fatal("anchor must fail closed")
	}
	if !containsAll(m.ParseErrors, "line 7") {
		t.Fatalf("line numbers must be file-relative, got %v", m.ParseErrors)
	}
}

func TestUnknownDocTypeFallsBackToUnionKeys(t *testing.T) {
	// A custom or legacy doc type must not mass-fire the advisory on
	// keys that are known somewhere in the schema; only genuinely
	// unknown keys are advisory.
	m, ok := strictParse(t, "title: r\ntype: runbook\nstatus: active\ncreated: 2026-11-01\nupdated: 2026-11-01\ntags: [x]\npackage: legacy-dep\nseverity: high\nwat: 1")
	if !ok {
		t.Fatalf("unknown keys must not block: %v", m.ParseErrors)
	}
	if len(m.Unknown) != 1 || m.Unknown[0] != "wat" {
		t.Fatalf("only the genuinely unknown key must fire, got %v", m.Unknown)
	}
}

// The Atlas ledger documents its workflow keys (asana ticket links,
// audience persona, reference-to-package binding, review notes,
// anti-patterns) in templates/*.md. The registry must know them or
// PYLON064 fires on every doc the ledger's own templates bless.
func TestLedgerWorkflowKeysAreKnown(t *testing.T) {
	cases := []struct{ typ, front string }{
		{"reference", "title: r\ntype: reference\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\npackage: wicket-wp-stack\ntags: []"},
		{"package", "title: p\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\ntags: []"},
		{"convention", "title: c\ntype: convention\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\nanti_patterns: []\ntags: []"},
		{"plan", "title: pl\ntype: plan\nstatus: proposed\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\nreview_note: \"\"\ntags: []"},
		{"decision", "title: d\ntype: decision\nstatus: accepted\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\ntags: []"},
		{"domain-entity", "title: e\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\ntags: []"},
		{"user-guide", "title: u\ntype: user-guide\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\npersona: Developer\npackage: wicket-wp-stack\ntags: []"},
		{"configuration", "title: cfg\ntype: configuration\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\npersona: Developer\npackage: wicket-wp-stack\ntags: []"},
	}
	for _, tc := range cases {
		m, _ := strictParse(t, tc.front)
		if len(m.Unknown) > 0 {
			t.Errorf("type %s: keys %v must be known", tc.typ, m.Unknown)
		}
	}
	// guard stays a quirk key: entry docs get the checker skip, not a
	// widened reference schema.
	m, _ := strictParse(t, "title: r\ntype: reference\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nguard: test\ntags: []")
	if len(m.Unknown) == 0 {
		t.Error("guard on a reference must stay unknown")
	}
}

func TestLedgerProvenanceAndCapabilityKeysAreKnown(t *testing.T) {
	cases := []struct{ typ, front string }{
		{"package", "title: p\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nclonable: true\ntestable: true\nsources: []\nrefs: []\nmigrated_from: meta/x.md\ntags: []"},
		{"reference", "title: r\ntype: reference\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nmigrated_from: meta/x.md\ntags: []"},
		{"investigation", "title: i\ntype: investigation\nstatus: active\nseverity: high\nreproducible: true\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\nartifact: \"\"\ntarget_release: \"\"\ntags: []"},
		{"audit", "title: a\ntype: audit\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nasana: []\napplies_to: []\ntags: []"},
		{"decision", "title: d\ntype: decision\nstatus: accepted\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\ndate: 2026-11-26\ndeciders: [Pylons]\ntags: []"},
	}
	for _, tc := range cases {
		m, _ := strictParse(t, tc.front)
		if len(m.Unknown) > 0 {
			t.Errorf("type %s: keys %v must be known", tc.typ, m.Unknown)
		}
	}
}

func TestLedgerTemplatesExtendRegistry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "templates", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package-custom.md", "---\ntitle: Package custom\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nexperiment_key: true\n---\n\n# Package custom\n")
	write("field-log.md", "---\ntitle: Field log\ntype: field-log\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\ncapture_window: 30d\n---\n\n# Field log\n")
	write("broken.md", "---\nanchors: &a [x\n---\n")
	pkgFront := "title: p\ntype: package\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\nexperiment_key: v\ntags: []"
	customFront := "title: f\ntype: field-log\nstatus: active\ncreated: 2026-11-26\nupdated: 2026-11-26\nowners: [Pylons]\ncapture_window: 7d\ntags: []"

	// Baseline without a ledger templates dir: the extra keys stay unknown.
	if err := LoadLedgerTemplates(filepath.Join(root, "absent")); err != nil {
		t.Fatalf("absent templates dir must not error: %v", err)
	}
	if m, ok := strictParse(t, pkgFront); ok && len(m.Unknown) != 1 || ok && m.Unknown[0] != "experiment_key" {
		t.Fatalf("baseline must flag experiment_key, got %v", m.Unknown)
	}

	if err := LoadLedgerTemplates(root); err != nil {
		t.Fatalf("LoadLedgerTemplates: %v", err)
	}
	if m, _ := strictParse(t, pkgFront); len(m.Unknown) != 0 {
		t.Errorf("template key must join its declared type, got %v", m.Unknown)
	}
	if m, _ := strictParse(t, customFront); len(m.Unknown) != 0 {
		t.Errorf("custom type keys must join UnionKeys, got %v", m.Unknown)
	}
	// The built-in registry itself stays untouched: manifests mirror the engine schema.
	if registry := TemplateKeyRegistry(); slices.Contains(registry["package"], "experiment_key") {
		t.Error("TemplateKeyRegistry must stay built-in only")
	}
}
