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
	write("manifests/pylons.yaml", "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: svc\n  sync:\n    path: sync\n")
	write("domain/shipments.md", "---\ntitle: Shipment\ntype: domain-entity\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: core\ntags: [delivery]\n---\nA shipment moves through pickup, transit, and delivery states.\n")
	write("packages/core.md", "---\ntitle: Core\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: core\ncomponents: [svc]\n---\nThe core package owns the shipment pipeline.\n")
	write("packages/ledger-sync.md", "---\ntitle: Ledger Sync\ntype: package\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\npackage: ledger-sync\ncomponents: [sync]\ndepends_on: [core]\n---\nThe ledger sync package mirrors core events.\n")
	write("quirks/replay-note.md", "---\ntitle: replay-note\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\nlinks: [repo://svc/domain/shipments.md]\n---\nDuplicate replays drop via idempotency keys.\n")
	write("quirks/term-cited.md", "---\ntitle: term-cited\ntype: quirk\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\n---\nEvery Shipment records an audit trail.\n")
	write("svc/go.mod", "module svc\n\ngo 1.27\n")
	write("svc/shipments.go", "package svc\n\nimport \"ledger-sync\"\n\nfunc Ship() { ledgerSyncHint() }\n")
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
