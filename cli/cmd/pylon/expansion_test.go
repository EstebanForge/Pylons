// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// expansionFixture builds a KB with a two-package dependency graph, a
// domain entity, and docs whose prose does and does not use the entity's
// vocabulary, so both expansion directions are observable.
func expansionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/pylons.yaml", "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n  ledger-sync:\n    path: sync\n")
	write("domain/shipments.md", "---\ntitle: Shipment\ntype: domain-entity\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\ntags: [delivery]\n---\nA shipment moves through pickup, transit, and delivery states, and mirrors into ledger-sync.\n")
	write("domain/run.md", "---\ntitle: Run\ntype: domain-entity\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\ntags: [execution]\n---\nA run executes once.\n")
	write("packages/core.md", "---\ntitle: Core\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: core\ncomponents: [svc]\n---\nThe core package owns the shipment pipeline.\n")
	write("packages/ledger-sync.md", "---\ntitle: Ledger Sync\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: ledger-sync\ncomponents: [ledger-sync]\ndepends_on: [core]\n---\nThe ledger sync package mirrors core events.\n")
	write("quirks/replay-note.md", "---\ntitle: replay-note\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\nlinks: [pylon://domain/shipments.md]\n---\nDuplicate replays drop via idempotency keys.\n")
	write("quirks/frag-note.md", "---\ntitle: frag-note\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\nlinks: [domain/shipments.md#states]\n---\nFragment anchors still cite the entity.\n")
	write("quirks/term-cited.md", "---\ntitle: term-cited\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\n---\nEvery Shipment records an audit trail.\n")
	write("quirks/run-user.md", "---\ntitle: run-user\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\n---\nCommands run at dawn.\n")
	write("svc/go.mod", "module svc\n\ngo 1.27\n")
	write("svc/shipments.go", "package svc\n\nimport \"ledger-sync\"\n\nfunc Ship() { ledgerSyncHint() }\n")
	write("svc/src/other.go", "package src\n\nfunc Other() {}\n")
	write("sync/go.mod", "module ledger-sync\n\ngo 1.27\n")
	write("sync/sync.go", "package sync\n\nfunc Mirror() {}\n")
	return root
}

func rankOf(docs []docEntry, path string) int {
	for i, d := range docs {
		if d.Path == path {
			return i + 1
		}
	}
	return -1
}

// TestIndexTermExpansion pins the index-side one-hop vocabulary: a doc
// that only cites the term (link or prose) becomes retrievable by the
// entity's other vocabulary.
func TestIndexTermExpansion(t *testing.T) {
	root := expansionFixture(t)
	docs := scanDocs(root, false)

	if got := filterDocs(docs, "", "", "", "", "", "delivery", 10, nil); rankOf(got, "quirks/replay-note.md") == -1 {
		t.Fatal("frontmatter-linked doc must surface for the entity's tag vocabulary")
	}
	if got := filterDocs(docs, "", "", "", "", "", "delivery", 10, nil); rankOf(got, "quirks/term-cited.md") == -1 {
		t.Fatal("prose-cited doc must surface for the entity's tag vocabulary")
	}
	// Control: a doc with no term relation stays out.
	if got := filterDocs(docs, "", "", "", "", "", "delivery", 10, nil); rankOf(got, "packages/ledger-sync.md") != -1 {
		t.Fatal("unrelated doc must not surface via term expansion")
	}
	// Bare control: without the expansion terms the cited docs must miss,
	// so the pass above is what carried them.
	for i := range docs {
		docs[i].IndexTerms = nil
	}
	if got := filterDocs(docs, "", "", "", "", "", "delivery", 10, nil); rankOf(got, "quirks/replay-note.md") != -1 || rankOf(got, "quirks/term-cited.md") != -1 {
		t.Fatal("control failed: cited docs surface without expansion terms; the assertions prove nothing")
	}
	// Exact fragment form still cites the entity.
	docs = scanDocs(root, false)
	if got := filterDocs(docs, "", "", "", "", "", "delivery", 10, nil); rankOf(got, "quirks/frag-note.md") == -1 {
		t.Fatal("fragment-suffixed link must still cite the entity")
	}
	// Short titles are exempt: entity Run (3 chars) grants nothing.
	if got := filterDocs(docs, "", "", "", "", "", "execution", 10, nil); rankOf(got, "quirks/run-user.md") != -1 {
		t.Fatal("short-title entity must not grant vocabulary")
	}
}

// TestQueryExpansionTerms pins the query-side expansion set: a source
// file path resolves to its owning project, the owning package, and the
// dependency-graph neighbors at depth one.
func TestQueryExpansionTerms(t *testing.T) {
	root := expansionFixture(t)
	cfg := loadConfigOrWarn(root)
	docs := scanDocs(root, false)

	got := expandQueryTerms(root, cfg, docs, docTokens("svc/shipments.go"))
	for _, want := range []string{"svc", "core", "ledger-sync"} {
		if !slices.Contains(got, want) {
			t.Fatalf("expansion missing %q: got %v", want, got)
		}
	}
	if !slices.IsSorted(got) {
		t.Fatalf("expansion must be deterministic sorted: %v", got)
	}

	if got := expandQueryTerms(root, cfg, docs, docTokens("unrelated words")); len(got) != 0 {
		t.Fatalf("plain prose query must not expand: %v", got)
	}
}

// TestQueryExpansionRanksEntity is the roadmap acceptance test: a query
// naming a project source file ranks the owning domain entity first
// with no manual query terms.
func TestQueryExpansionRanksEntity(t *testing.T) {
	root := expansionFixture(t)
	cfg := loadConfigOrWarn(root)
	docs := scanDocs(root, false)

	expanded := expandQueryTerms(root, cfg, docs, docTokens("svc/shipments.go"))
	query := "svc/shipments.go " + strings.Join(expanded, " ")
	got := filterDocs(docs, "", "", "", "", "", query, 10, nil)
	if rank := rankOf(got, "domain/shipments.md"); rank == -1 {
		t.Fatalf("owning entity missing from results: %d docs", len(got))
	} else if rank > 3 {
		t.Fatalf("owning entity ranked %d, want top 3: %v", rank, pathsOf(got))
	}
	// Without expansion the same query must miss the entity entirely.
	bare := filterDocs(docs, "", "", "", "", "", "svc/shipments.go", 10, nil)
	if rank := rankOf(bare, "domain/shipments.md"); rank != -1 {
		t.Fatalf("entity surfaced without expansion at rank %d; the test proves nothing", rank)
	}
}

func pathsOf(docs []docEntry) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Path)
	}
	return out
}

// runPylon execs the built binary: run() writes straight to os.Stdout,
// so stdout-shape assertions must go through the process boundary.
func runPylon(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(pwd, "..", "..", "..", "bin", "pylon")
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatal(err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// TestSearchOutputCarriesExpansion pins the surfaced expansion: a human
// label line, and an explicit JSON field on the always-object shape.
func TestSearchOutputCarriesExpansion(t *testing.T) {
	root := expansionFixture(t)

	code, stdout, _ := runPylon(t, root, "search", "svc/shipments.go")
	if code != 0 {
		t.Fatalf("search exit %d", code)
	}
	if !strings.Contains(stdout, "expanded: core ledger-sync svc") {
		t.Fatalf("human output missing expansion label: %q", stdout[:min(200, len(stdout))])
	}

	code, stdout, _ = runPylon(t, root, "search", "--json", "svc/shipments.go")
	if code != 0 {
		t.Fatalf("search --json exit %d", code)
	}
	var out struct {
		Expanded []string   `json:"expanded"`
		Docs     []docEntry `json:"docs"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("search --json must be an object with expanded and docs: %v", err)
	}
	if !slices.Equal(out.Expanded, []string{"core", "ledger-sync", "svc"}) {
		t.Fatalf("json expanded = %v", out.Expanded)
	}
	if rankOf(out.Docs, "domain/shipments.md") == -1 {
		t.Fatal("entity missing from json docs")
	}
}

// TestQueryExpansionPathAttribution pins the resolution order: repo://
// tokens raw, cwd-relative paths, then KB-root-relative; hyphenated and
// case-exact aliases match as raw fields.
func TestQueryExpansionPathAttribution(t *testing.T) {
	root := expansionFixture(t)
	cfg := loadConfigOrWarn(root)
	docs := scanDocs(root, false)

	// A path whose directory differs from the alias must still attribute.
	got := expandQueryTerms(root, cfg, docs, strings.Fields("svc/src/other.go"))
	if !slices.Contains(got, "svc") || !slices.Contains(got, "core") {
		t.Fatalf("nested path must attribute through cwd/kb resolution: %v", got)
	}
	// Hyphenated aliases survive as raw fields.
	got = expandQueryTerms(root, cfg, docs, strings.Fields("ledger-sync"))
	if !slices.Contains(got, "ledger-sync") {
		t.Fatalf("hyphenated alias token must match: %v", got)
	}
	// repo:// tokens reach the resolver unjoined.
	got = expandQueryTerms(root, cfg, docs, strings.Fields("repo://svc/svc/src/other.go"))
	if !slices.Contains(got, "svc") {
		t.Fatalf("repo:// token must attribute directly: %v", got)
	}
	// Absolute query paths attribute too.
	got = expandQueryTerms(root, cfg, docs, strings.Fields(filepath.Join(root, "svc", "src", "other.go")))
	if !slices.Contains(got, "svc") {
		t.Fatalf("absolute path must attribute: %v", got)
	}
	// Caller context: from inside the project, a bare filename resolves.
	t.Chdir(filepath.Join(root, "svc"))
	got = expandQueryTerms(root, cfg, docs, strings.Fields("other.go"))
	if !slices.Contains(got, "svc") {
		t.Fatalf("cwd-relative bare filename must attribute from inside the project: %v", got)
	}
}

// TestQueryExpansionOutsideKBProject pins the registry-style layout:
// a project living outside the KB (path: ../project) attributes from
// the caller's cwd, never from a KB-root join.
func TestQueryExpansionOutsideKBProject(t *testing.T) {
	base := t.TempDir()
	kb := filepath.Join(base, "kb")
	proj := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(kb, "manifests"), filepath.Join(kb, "packages"), proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifest := "version: 1\nextensions: [go]\nprojects:\n  outside:\n    path: ../outside\n"
	if err := os.WriteFile(filepath.Join(kb, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "packages", "outside.md"), []byte("---\ntitle: Outside\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: outside\ncomponents: [outside]\n---\nThe outside package.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "go.mod"), []byte("module outside\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigOrWarn(kb)
	// From inside the project, a bare filename resolves via the cwd.
	t.Chdir(proj)
	got := expandQueryTerms(kb, cfg, nil, strings.Fields("main.go"))
	if !slices.Contains(got, "outside") {
		t.Fatalf("cwd-relative filename in an outside-KB project must attribute: %v", got)
	}
	// Absolute path resolves regardless of cwd.
	got = expandQueryTerms(kb, cfg, nil, strings.Fields(filepath.Join(proj, "main.go")))
	if !slices.Contains(got, "outside") {
		t.Fatalf("absolute path in an outside-KB project must attribute: %v", got)
	}
}
