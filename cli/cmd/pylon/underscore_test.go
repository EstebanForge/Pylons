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
	files := walk(root, []corpusRoot{root}, langs, false, &strings.Builder{})
	if len(files) != 1 || filepath.Base(files[0].Path) != "main.go" {
		t.Fatalf("underscore corpus files must be skipped, got %+v", files)
	}
}
