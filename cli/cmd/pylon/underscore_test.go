package main

import (
	"os"

	"github.com/estebanforge/pylons/cli/internal/aroma"
	"path/filepath"
	"strings"
	"testing"
)

// scanDocs backs brief, map, and search: underscore dirs are never live docs.
func TestScanDocsSkipsUnderscoreDirs(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "plans/proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "plans/_examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "plans/proposals/real.md"),
		[]byte("---\ntitle: \"Real\"\ntype: plan\nstatus: proposed\nupdated: 2026-11-21\ntags: []\n---\n\n# Real plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "plans/_examples/hidden.md"),
		[]byte("---\ntitle: \"Hidden\"\ntype: plan\nstatus: proposed\nupdated: 2026-11-21\ntags: []\n---\n\n# Hidden plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := scanDocs(kb, true)
	var paths []string
	for _, d := range docs {
		paths = append(paths, filepath.ToSlash(d.Path))
	}
	for _, p := range paths {
		if strings.Contains(p, "_examples") {
			t.Fatalf("underscore dir doc surfaced in scanDocs: %s", p)
		}
	}
	found := false
	for _, p := range paths {
		if p == "plans/proposals/real.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("real doc missing from scanDocs: %v", paths)
	}
}

// scanDocs backs brief, map, and search: pylons/ is the ledger itself,
// so a ledger-side pylons folder is user content and gets indexed like
// any other folder. The product manual no longer lives inside a ledger.
func TestScanDocsIndexesLedgerPylonsFolder(t *testing.T) {
	kb := t.TempDir()
	if err := os.MkdirAll(filepath.Join(kb, "plans/proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "pylons/notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "plans/proposals/real.md"),
		[]byte("---\ntitle: \"Real\"\ntype: plan\nstatus: proposed\nupdated: 2026-11-21\ntags: []\n---\n\n# Real plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "pylons/notes/team.md"),
		[]byte("---\ntitle: \"Team notes\"\ntype: reference\nstatus: active\nupdated: 2026-11-21\ntags: []\n---\n\n# Team notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := scanDocs(kb, true)
	var paths []string
	for _, d := range docs {
		paths = append(paths, filepath.ToSlash(d.Path))
	}
	found := false
	for _, p := range paths {
		if p == "pylons/notes/team.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ledger-side pylons folder doc missing from scanDocs: %v", paths)
	}
	found = false
	for _, p := range paths {
		if p == "plans/proposals/real.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("real doc missing from scanDocs: %v", paths)
	}
}

// The aroma corpus walk skips underscore dirs the same way it skips testdata.
func TestAromaWalkSkipsUnderscoreDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "_examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_examples", "demo.go"),
		[]byte("package demo\n\nfunc process(items []string) error {\n\tfor _, item := range items {\n\t\tprintln(item)\n\t}\n\treturn nil\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc run(items []string) error {\n\tfor _, item := range items {\n\t\tprintln(item)\n\t}\n\treturn nil\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	langs := []aroma.Language{aroma.GoLanguage}
	root := corpusRoot{label: "", path: ".", abs: dir}
	files, _ := walk(root, []corpusRoot{root}, langs, false, &strings.Builder{})
	if len(files) != 1 || filepath.Base(files[0].Path) != "main.go" {
		t.Fatalf("underscore corpus files must be skipped, got %+v", files)
	}
}

func TestScanDocsSkipsNestedLedgers(t *testing.T) {
	kb := t.TempDir()
	seed := func(rel, body string) {
		t.Helper()
		path := filepath.Join(kb, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seed("manifests/pylons.yaml", "version: 1\n")
	seed("decisions/d-root.md", "---\ntitle: Root\ntype: decision\nstatus: accepted\n---\n\n# Root\n")
	// A nested ledger inside this one: its docs belong to it alone.
	seed("child/manifests/pylons.yaml", "version: 1\n")
	seed("child/quirks/d-child.md", "---\ntitle: Child\ntype: quirk\nstatus: active\n---\n\n# Child\n")

	got := scanDocs(kb, false)
	for _, d := range got {
		if strings.HasPrefix(d.Path, "child/") {
			t.Fatalf("scanDocs walked into a nested ledger: %s", d.Path)
		}
	}
	if len(got) != 1 {
		t.Fatalf("scanDocs = %v, want the root doc only", got)
	}
}

func TestScanDocsTreatsDirectoryManifestAsContent(t *testing.T) {
	// A directory named manifests/pylons.yaml is not a ledger marker:
	// lint and the doc index must agree and both descend.
	kb := t.TempDir()
	seed := func(rel string, body []byte, dir bool) {
		t.Helper()
		path := filepath.Join(kb, filepath.FromSlash(rel))
		if dir {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seed("manifests/pylons.yaml", []byte("version: 1\n"), false)
	seed("quirks/sub/manifests/pylons.yaml", nil, true)
	seed("quirks/sub/q.md", []byte("---\ntitle: q\ntype: quirk\nstatus: active\n---\n\n# q\n"), false)

	got := scanDocs(kb, false)
	found := false
	for _, d := range got {
		if d.Path == "quirks/sub/q.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("a directory named manifests/pylons.yaml is not a ledger: scanDocs must descend")
	}
}
