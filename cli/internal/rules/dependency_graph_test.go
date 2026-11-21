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
