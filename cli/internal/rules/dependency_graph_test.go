// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newGraphFixture builds a KB with three go projects and a chain of
// imports a -> b -> c. deps optionally declares depends_on per package.
func newGraphFixture(t *testing.T, deps map[string]string) (kb string) {
	t.Helper()
	kb = t.TempDir()
	mk := func(alias, module string, imports string) string {
		root := t.TempDir()
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/"+module+"\n")
		mustWrite(t, filepath.Join(root, "main.go"), "package "+alias+"\n\n"+imports+"\n")
		return root
	}
	roots := map[string]string{
		"a": mk("a", "a", "import _ \"example.com/b\""),
		"b": mk("b", "b", "import _ \"example.com/c\""),
		"c": mk("c", "c", ""),
	}
	entries := ""
	for alias, root := range roots {
		entries += "  " + alias + ":\n    path: " + root + "\n    ref: main\n"
	}
	manifest := "version: 1\nextensions: [go]\nprojects:\n" + entries
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), manifest)
	for _, name := range []string{"a", "b", "c"} {
		var depsYAML string
		if d, ok := deps[name]; ok {
			depsYAML = "\ndepends_on: " + d
		} else {
			depsYAML = "\ndepends_on: []"
		}
		mustWrite(t, filepath.Join(kb, "packages/"+name+".md"),
			"---\ntitle: \""+strings.ToUpper(name)+"\"\ntype: package\nstatus: active\ntier: core\npackage: "+name+"\ncomponents: ["+name+"]"+depsYAML+"\n---\n\n# "+strings.ToUpper(name)+"\n")
	}
	return kb
}

func TestDependencyGraphDeclaredAndActual(t *testing.T) {
	kb := newGraphFixture(t, map[string]string{"a": "[b]", "b": "[c]"})
	g, err := DependencyGraph(kb, "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("depth 1 must visit a and b, got %+v", g.Nodes)
	}
	if len(g.Edges) != 1 || g.Edges[0].From != "a" || g.Edges[0].To != "b" {
		t.Fatalf("edges must carry the actual a->b edge, got %+v", g.Edges)
	}
	if len(g.Undeclared) != 0 || len(g.Unused) != 0 {
		t.Fatalf("a declared chain must not diverge, got undeclared=%v unused=%v", g.Undeclared, g.Unused)
	}
}

func TestDependencyGraphDepthWalksTransitively(t *testing.T) {
	kb := newGraphFixture(t, map[string]string{"a": "[b]", "b": "[c]"})
	g, err := DependencyGraph(kb, "a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("depth 2 must visit a, b, and c, got %+v", g.Nodes)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("depth 2 must carry both edges, got %+v", g.Edges)
	}
}

func TestDependencyGraphReportsDivergences(t *testing.T) {
	kb := newGraphFixture(t, map[string]string{"a": "[c]", "b": "[]", "c": "[]"})
	g, err := DependencyGraph(kb, "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	// a imports b but declares c: b is undeclared and c is unused.
	if len(g.Undeclared) != 1 || g.Undeclared[0].From != "a" || g.Undeclared[0].To != "b" {
		t.Fatalf("the actual edge to b must be undeclared, got %+v", g.Undeclared)
	}
	if len(g.Unused) != 1 || !strings.Contains(g.Unused[0], "a") || !strings.Contains(g.Unused[0], "c") {
		t.Fatalf("the declared c must read as unused, got %+v", g.Unused)
	}
}

func TestDependencyGraphSiblingComponentsInternal(t *testing.T) {
	// Two components of one package never fire against each other or
	// against their own package's depends_on entries.
	kb := t.TempDir()
	shared := t.TempDir()
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(shared, "go.mod"), "module example.com/shared\n")
	mustWrite(t, filepath.Join(shared, "main.go"), "package shared\n")
	other := t.TempDir()
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(other, "go.mod"), "module example.com/other\n")
	mustWrite(t, filepath.Join(other, "main.go"), "package other\n\nimport _ \"example.com/shared\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [go]\nprojects:\n  p1:\n    path: "+shared+"\n    ref: main\n  p2:\n    path: "+other+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/platform.md"),
		"---\ntitle: \"Platform\"\ntype: package\nstatus: active\ntier: core\npackage: platform\ncomponents: [p1, p2]\ndepends_on: []\n---\n\n# Platform\n")
	g, err := DependencyGraph(kb, "platform", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Undeclared) != 0 {
		t.Fatalf("sibling components must stay internal, got %+v", g.Undeclared)
	}
}

func TestDependencyGraphMissingPackageIsUsageError(t *testing.T) {
	kb := newGraphFixture(t, nil)
	if _, err := DependencyGraph(kb, "ghost", 1); err == nil {
		t.Fatal("a package that no doc declares must be a usage error, not an empty report")
	}
}

func TestDependencyGraphDefaultDepthIsOne(t *testing.T) {
	kb := newGraphFixture(t, map[string]string{"a": "[b]", "b": "[c]"})
	g, err := DependencyGraph(kb, "a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("depth 0 must mean the default depth 1, got %+v nodes", g.Nodes)
	}
}

func TestDependencyGraphMultiOwnerAlias(t *testing.T) {
	// An alias claimed by two packages splits into per-owner edges:
	// the declared owner is touched, the other reads as undeclared,
	// and no false unused report appears.
	kb := t.TempDir()
	xa := t.TempDir()
	if err := os.MkdirAll(xa, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(xa, "go.mod"), "module example.com/xa\n")
	mustWrite(t, filepath.Join(xa, "main.go"), "package xa\n")
	app := t.TempDir()
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(app, "go.mod"), "module example.com/app\n")
	mustWrite(t, filepath.Join(app, "main.go"), "package app\n\nimport _ \"example.com/xa\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [go]\nprojects:\n  app:\n    path: "+app+"\n    ref: main\n  xa:\n    path: "+xa+"\n    ref: main\n")
	mkPkg := func(name string, comps, deps string) {
		mustWrite(t, filepath.Join(kb, "packages/"+name+".md"),
			"---\ntitle: \""+name+"\"\ntype: package\nstatus: active\ntier: core\npackage: "+name+"\ncomponents: "+comps+"\ndepends_on: "+deps+"\n---\n\n# "+name+"\n")
	}
	mkPkg("app", "[app]", "[extra]")
	mkPkg("core", "[xa]", "[]")
	mkPkg("extra", "[xa]", "[]")
	g, err := DependencyGraph(kb, "app", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unused) != 0 {
		t.Fatalf("the declared owner extra must be touched, got unused=%v", g.Unused)
	}
	if len(g.Undeclared) != 1 || g.Undeclared[0].From != "app" || g.Undeclared[0].To != "core" {
		t.Fatalf("the other owner core must read as undeclared, got %+v", g.Undeclared)
	}
}

func TestDependencyGraphWarningsDeduped(t *testing.T) {
	// One unclaimed alias imported by several files yields one
	// warning, not one per import occurrence.
	kb := t.TempDir()
	orphan := t.TempDir()
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(orphan, "go.mod"), "module example.com/orphan\n")
	app := t.TempDir()
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(app, "go.mod"), "module example.com/app\n")
	mustWrite(t, filepath.Join(app, "a.go"), "package app\n\nimport _ \"example.com/orphan\"\n")
	mustWrite(t, filepath.Join(app, "b.go"), "package app\n\nimport _ \"example.com/orphan\"\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [go]\nprojects:\n  app:\n    path: "+app+"\n    ref: main\n  orphan:\n    path: "+orphan+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/app.md"),
		"---\ntitle: \"App\"\ntype: package\nstatus: active\ntier: core\npackage: app\ncomponents: [app]\ndepends_on: []\n---\n\n# App\n")
	g, err := DependencyGraph(kb, "app", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Warnings) != 1 {
		t.Fatalf("identical warnings must dedupe, got %d: %v", len(g.Warnings), g.Warnings)
	}
}

func TestDependencyGraphUnscannableDepsWarnNotUnused(t *testing.T) {
	// A dependency with no scannable components, or no packages/ doc
	// at all, warns instead of reading as a false unused dependency.
	kb := t.TempDir()
	leaf := t.TempDir()
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(leaf, "go.mod"), "module example.com/leaf\n")
	mustWrite(t, filepath.Join(leaf, "main.go"), "package leaf\n")
	ghost := t.TempDir()
	if err := os.MkdirAll(ghost, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(ghost, "go.mod"), "module example.com/ghost\n")
	mustWrite(t, filepath.Join(ghost, "main.go"), "package ghost\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"),
		"version: 1\nextensions: [go]\nprojects:\n  top:\n    path: "+leaf+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/top.md"),
		"---\ntitle: \"Top\"\ntype: package\nstatus: active\ntier: core\npackage: top\ncomponents: [top]\ndepends_on: [hollow, phantom]\n---\n\n# Top\n")
	mustWrite(t, filepath.Join(kb, "packages/hollow.md"),
		"---\ntitle: \"Hollow\"\ntype: package\nstatus: active\ntier: core\npackage: hollow\ncomponents: []\ndepends_on: []\n---\n\n# Hollow\n")
	g, err := DependencyGraph(kb, "top", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unused) != 0 {
		t.Fatalf("deps without components must warn, not read unused, got %v", g.Unused)
	}
	if len(g.Warnings) != 2 {
		t.Fatalf("both unscannable deps must warn, got %v", g.Warnings)
	}
}
