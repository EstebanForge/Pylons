// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func hygieneFixture(t *testing.T) []docEntry {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "domain", "widget.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	front := "title: Widget\ntype: domain-entity\nstatus: active\ncreated: 2026-01-01\nupdated: 2026-06-01\nowners: [dana]\nledgertoken: zanzibar-x9\n"
	body := "A widget moves through the assembly line."
	if err := os.WriteFile(p, []byte("---\n"+front+"---\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return scanDocs(root, false)
}

// Frontmatter vocabulary must not enter the index: key names and unknown
// field values only exist in the YAML block, so a query on them must find
// nothing rather than every document.
func TestIndexIgnoresFrontmatterTokens(t *testing.T) {
	docs := hygieneFixture(t)
	for _, q := range []string{"zanzibar", "created"} {
		got := filterDocs(docs, "", "", "", "", "", q, 10, nil)
		if len(got) != 0 {
			t.Fatalf("query %q matched %d docs; frontmatter leaked into the index", q, len(got))
		}
	}
}

// Structured metadata must stay searchable through the context header even
// when the body never mentions the term.
func TestIndexSurfacesFrontmatterFields(t *testing.T) {
	docs := hygieneFixture(t)
	got := filterDocs(docs, "", "", "", "", "", "dana", 10, nil)
	if len(got) != 1 || got[0].Path != "domain/widget.md" {
		t.Fatalf("owner query: got %d docs, want domain/widget.md", len(got))
	}
}
