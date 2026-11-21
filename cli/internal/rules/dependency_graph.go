// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// GraphEdge is one actual import edge between two registered projects.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GraphNode is one project in the visited graph.
type GraphNode struct {
	Alias   string `json:"alias"`
	Package string `json:"package"`
}

// GraphReport is the report-only dependency view for one package:
// actual import edges across the registered projects, declared
// depends_on sets, and the divergences between the two.
type GraphReport struct {
	Package    string              `json:"package"`
	Depth      int                 `json:"depth"`
	Nodes      []GraphNode         `json:"nodes"`
	Edges      []GraphEdge         `json:"edges"`
	Declared   map[string][]string `json:"declared"`
	Undeclared []GraphEdge         `json:"undeclared"`
	Unused     []string            `json:"unused"`
	Warnings   []string            `json:"warnings"`
}

// graphDepthCap bounds the BFS so a cycle cannot walk forever.
const graphDepthCap = 8

// DependencyGraph resolves depends_on declared in packages/ against the
// actual import edges the import-boundary extractor finds across the
// registered projects. Depth 1 (the default) covers the package's own
// components; each further level adds the neighbors' actual edges. A
// declared dependency no import reaches reads as unused; an actual edge
// no depends_on entry covers reads as undeclared.
func DependencyGraph(kbRoot, pkgName string, depth int) (GraphReport, error) {
	g := GraphReport{
		Package:    pkgName,
		Nodes:      []GraphNode{},
		Edges:      []GraphEdge{},
		Declared:   map[string][]string{},
		Undeclared: []GraphEdge{},
		Unused:     []string{},
		Warnings:   []string{},
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > graphDepthCap {
		depth = graphDepthCap
	}
	g.Depth = depth
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return g, err
	}
	pkgs := collectPackages(kbRoot, nil)
	// alias -> package names claiming it (component membership).
	owners := map[string][]string{}
	pkgByName := map[string]pkgDoc{}
	for _, pd := range pkgs {
		pkgByName[pd.fm.Package] = pd
		for _, alias := range pd.fm.Components {
			if !slices.Contains(owners[alias], pd.fm.Package) {
				owners[alias] = append(owners[alias], pd.fm.Package)
			}
		}
	}
	start, ok := pkgByName[pkgName]
	if !ok {
		return g, fmt.Errorf("no packages/ doc declares package %q", pkgName)
	}
	ident := projectIdentities(kbRoot, cfg)
	otherRoots := map[string]string{}
	for _, id := range ident {
		if root := cfg.ProjectPath(kbRoot, id.alias); root != "" {
			otherRoots[root] = id.alias
		}
	}
	seen := map[string]bool{pkgName: true}
	frontier := []pkgDoc{start}
	for level := 0; level < depth && len(frontier) > 0; level++ {
		var next []pkgDoc
		for _, pd := range frontier {
			g.Declared[pd.fm.Package] = append([]string{}, pd.fm.DependsOn...)
			deps := map[string]bool{pd.fm.Package: true}
			for _, d := range pd.fm.DependsOn {
				deps[d] = true
			}
			touched := map[string]bool{}
			var actual []GraphEdge
			for _, alias := range pd.fm.Components {
				root := cfg.ProjectPath(kbRoot, alias)
				if root == "" {
					g.Warnings = append(g.Warnings, fmt.Sprintf("component %q of package %q is not a registered project", alias, pd.fm.Package))
					continue
				}
				g.Nodes = append(g.Nodes, GraphNode{Alias: alias, Package: pd.fm.Package})
				for _, edge := range scanImports(root, alias, ident, enabledLanguages(cfg, alias), otherRoots) {
					if slices.Contains(pd.fm.Components, edge.target) {
						continue
					}
					actual = append(actual, GraphEdge{From: alias, To: edge.target})
					targetDeps := owners[edge.target]
					if len(targetDeps) == 0 {
						g.Warnings = append(g.Warnings, fmt.Sprintf("component %q is claimed by no packages/ doc", edge.target))
						continue
					}
					for _, targetPkg := range targetDeps {
						if deps[targetPkg] {
							touched[targetPkg] = true
							continue
						}
						g.Undeclared = append(g.Undeclared, GraphEdge{From: pd.fm.Package, To: targetPkg})
					}
					g.Nodes = append(g.Nodes, GraphNode{Alias: edge.target, Package: targetDeps[0]})
					neighbor, isPkg := pkgByName[targetDeps[0]]
					if isPkg && !seen[neighbor.fm.Package] {
						seen[neighbor.fm.Package] = true
						next = append(next, neighbor)
					}
				}
			}
			for _, d := range pd.fm.DependsOn {
				if touched[d] {
					continue
				}
				depDoc, ok := pkgByName[d]
				if !ok {
					g.Warnings = append(g.Warnings, fmt.Sprintf("dependency %q has no packages/ doc", d))
					continue
				}
				if len(depDoc.fm.Components) == 0 {
					g.Warnings = append(g.Warnings, fmt.Sprintf("dependency %q declares no components to import", d))
					continue
				}
				g.Unused = append(g.Unused, pd.fm.Package+":"+d)
			}
			g.Edges = append(g.Edges, actual...)
		}
		frontier = next
	}
	g.Nodes = dedupeNodes(g.Nodes)
	g.Edges = dedupeEdges(g.Edges)
	slices.Sort(g.Warnings)
	g.Warnings = slices.Compact(g.Warnings)
	g.Undeclared = dedupeEdges(g.Undeclared)
	slices.Sort(g.Unused)
	g.Unused = slices.Compact(g.Unused)
	return g, nil
}

func dedupeNodes(nodes []GraphNode) []GraphNode {
	seen := map[string]bool{}
	var out []GraphNode
	for _, n := range nodes {
		if !seen[n.Alias] {
			seen[n.Alias] = true
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b GraphNode) int { return strings.Compare(a.Alias, b.Alias) })
	return out
}

func dedupeEdges(edges []GraphEdge) []GraphEdge {
	seen := map[string]bool{}
	var out []GraphEdge
	for _, e := range edges {
		key := e.From + "->" + e.To
		if !seen[key] {
			seen[key] = true
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b GraphEdge) int {
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		return strings.Compare(a.To, b.To)
	})
	return out
}
