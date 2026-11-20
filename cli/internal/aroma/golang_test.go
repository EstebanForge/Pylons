// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"slices"
	"strings"
	"testing"
)

const sample = `package sample

import "fmt"

type Widget struct{ size int }

func (w *Widget) Grow(by int) int {
	w.size = w.size + by
	if w.size > 10 {
		return w.size
	}
	return 0
}

func helper(items []string) error {
	var out string
	for _, item := range items {
		out = out + item
	}
	n := len(items)
	if n == 0 {
		return nil
	}
	fmt.Println(out, n)
	return nil
}

var globalFn = func(x int) int {
	y := x * 2
	return y
}
`

func methods(t *testing.T, source string) []aroma.Method {
	t.Helper()
	got := aroma.GoLanguage.ParseMethods(source, "sample.go")
	if len(got) == 0 {
		t.Fatalf("ParseMethods found no methods")
	}
	return got
}

func TestParseMethodsFindsAllKinds(t *testing.T) {
	got := methods(t, sample)
	var names []string
	for _, method := range got {
		names = append(names, method.Name)
	}
	want := []string{"*Widget.Grow", "helper", "globalFn"}
	if !slices.Equal(names, want) {
		t.Fatalf("methods = %v, want %v", names, want)
	}
}

func TestGoFrontEndFeaturizesInSourceOrder(t *testing.T) {
	got := methods(t, sample)
	tokens := aroma.Featurize(got[0].Body, aroma.FeaturizeOptions{})
	var texts []string
	for _, entry := range tokens {
		texts = append(texts, entry.Token.Text)
	}
	// The body of Grow: w.size = w.size + by; if ... return ...; return 0.
	want := []string{"w", "size", "w", "size", "by", "w", "size", "10", "w", "size", "0"}
	if !slices.Equal(texts, want) {
		t.Fatalf("token texts = %v, want %v", texts, want)
	}
}

func TestGoFrontEndKeywordGapsKeepSourceOrder(t *testing.T) {
	got := methods(t, sample)
	// Collect the keyword tokens of the body via the tree walk.
	var walk func(node aroma.Node) []string
	walk = func(node aroma.Node) []string {
		switch n := node.(type) {
		case aroma.Token:
			if n.Keyword {
				return []string{n.Text}
			}
			return nil
		case *aroma.Tree:
			var out []string
			for _, child := range n.Children {
				out = append(out, walk(child)...)
			}
			return out
		}
		return nil
	}
	keywords := walk(got[0].Body)
	joined := strings.Join(keywords, " ")
	if !strings.Contains(joined, "if . > { return . ; }") {
		t.Fatalf("keyword order = %q, want the if-block sequence in source order", joined)
	}
}

func TestGoFrontEndParamsAreLocalsPackageVarsAreNot(t *testing.T) {
	got := methods(t, sample)
	grow := aroma.Featurize(got[0].Body, aroma.FeaturizeOptions{})
	// `by`, a parameter, is a local; `w`, the receiver, is a local; nothing
	// else named w appears as a global.
	byLocal := false
	for _, entry := range grow {
		if entry.Token.Text == "by" && entry.Token.Local {
			byLocal = true
		}
		if entry.Token.Text == "w" && !entry.Token.Local {
			t.Fatalf("receiver w is not marked local")
		}
	}
	if !byLocal {
		t.Fatalf("parameter by is not marked local")
	}
	helper := aroma.Featurize(got[1].Body, aroma.FeaturizeOptions{})
	for _, entry := range helper {
		if entry.Token.Text == "fmt" && entry.Token.Local {
			t.Fatalf("package name fmt marked local")
		}
		if entry.Token.Text == "len" && entry.Token.Local {
			t.Fatalf("builtin len marked local")
		}
	}
}

func TestGoFrontEndShadowingCollapsesByName(t *testing.T) {
	source := `package p

func shadow() int {
	x := 1
	if x > 0 {
		x := 2
		return x
	}
	return x
}
`
	got := aroma.GoLanguage.ParseMethods(source, "p.go")
	tokens := aroma.Featurize(got[0].Body, aroma.FeaturizeOptions{})
	for _, entry := range tokens {
		if entry.Token.Text == "x" && !entry.Token.Local {
			t.Fatalf("shadowed x not marked local")
		}
	}
}

func TestParseSnippetStatements(t *testing.T) {
	node, ok := aroma.GoLanguage.ParseSnippet("if x > 0 {\n  y := 1\n  return y\n}", aroma.SnippetOptions{})
	if !ok || node == nil {
		t.Fatalf("statement snippet failed to parse")
	}
	tokens := aroma.Featurize(node, aroma.FeaturizeOptions{})
	var texts []string
	for _, entry := range tokens {
		texts = append(texts, entry.Token.Text)
	}
	// x is not declared, not called, lowercase, and not a selector left side:
	// it reads as a local declared above the snippet.
	if !slices.Equal(texts, []string{"x", "0", "y", "1", "y"}) {
		t.Fatalf("snippet tokens = %v", texts)
	}
	var locals []bool
	for _, entry := range tokens {
		locals = append(locals, entry.Token.Local)
	}
	if !slices.Equal(locals, []bool{true, false, true, false, true}) {
		t.Fatalf("snippet locals = %v", locals)
	}
}

func TestParseSnippetSelectorLeftSideIsGlobal(t *testing.T) {
	node, ok := aroma.GoLanguage.ParseSnippet("fmt.Println(x)", aroma.SnippetOptions{})
	if !ok {
		t.Fatalf("snippet failed to parse")
	}
	tokens := aroma.Featurize(node, aroma.FeaturizeOptions{})
	if len(tokens) != 3 {
		t.Fatalf("tokens = %d, want 3", len(tokens))
	}
	if tokens[0].Token.Text != "fmt" || tokens[0].Token.Local {
		t.Fatalf("fmt should be a global name token")
	}
	if tokens[1].Token.Text != "Println" || tokens[1].Token.Local {
		t.Fatalf("Println should be a global name token: %+v", tokens[1].Token)
	}
	if tokens[2].Token.Text != "x" || !tokens[2].Token.Local {
		t.Fatalf("x should read as a local: %+v", tokens[2].Token)
	}
}

func TestParseSnippetGlobalsAndCalled(t *testing.T) {
	node, ok := aroma.GoLanguage.ParseSnippet("helper(data)", aroma.SnippetOptions{Globals: map[string]bool{"data": true}})
	if !ok {
		t.Fatalf("snippet failed to parse")
	}
	tokens := aroma.Featurize(node, aroma.FeaturizeOptions{})
	if tokens[0].Token.Text != "helper" || tokens[0].Token.Local {
		t.Fatalf("called name should not be local: %+v", tokens[0].Token)
	}
	if tokens[1].Token.Text != "data" || tokens[1].Token.Local {
		t.Fatalf("global-marked name should not be local: %+v", tokens[1].Token)
	}
}

func TestParseSnippetExpression(t *testing.T) {
	node, ok := aroma.GoLanguage.ParseSnippet("x + 1", aroma.SnippetOptions{})
	if !ok {
		t.Fatalf("expression snippet failed to parse")
	}
	tokens := aroma.Featurize(node, aroma.FeaturizeOptions{})
	var texts []string
	for _, entry := range tokens {
		texts = append(texts, entry.Token.Text)
	}
	if !slices.Equal(texts, []string{"x", "1"}) {
		t.Fatalf("expression tokens = %v", texts)
	}
}

func TestParseSnippetEmpty(t *testing.T) {
	if _, ok := aroma.GoLanguage.ParseSnippet("   \n\t\n", aroma.SnippetOptions{}); ok {
		t.Fatalf("empty snippet should report false")
	}
}

func TestRoundTripThroughTheEngine(t *testing.T) {
	// Two Go files share a shape; a query from changed lines finds both.
	first := `package a

func process(items []string) error {
	for _, item := range items {
		if item == "" {
			continue
		}
		println(item)
	}
	return nil
}
`
	second := `package b

func handle(rows []string) error {
	for _, row := range rows {
		if row == "" {
			continue
		}
		println(row)
	}
	return nil
}
`
	index := aroma.BuildIndex([]aroma.SourceFile{
		{Path: "a/a.go", Source: first},
		{Path: "b/b.go", Source: second},
	}, aroma.IndexOptions{Languages: []aroma.Language{aroma.GoLanguage}})
	if index.Stats.Methods != 2 {
		t.Fatalf("methods = %d, want 2", index.Stats.Methods)
	}
	queries := aroma.QueriesFromLines(index, aroma.SourceFile{Path: "a/a.go", Source: first}, []int{4, 5, 6, 7})
	if len(queries) != 1 {
		t.Fatalf("queries = %d, want 1", len(queries))
	}
	results := aroma.Search(index, queries[0], aroma.SearchOptions{MinSimilarity: 0.7})
	if len(results) != 1 || results[0].Method.Name != "handle" {
		t.Fatalf("results = %+v, want handle", results)
	}
	recommendations := aroma.Recommend(index, queries[0], aroma.RecommendOptions{MinSupport: 1})
	if len(recommendations) != 1 {
		t.Fatalf("recommendations = %d, want 1", len(recommendations))
	}
	if recommendations[0].Method.Name != "handle" {
		t.Fatalf("recommendation = %+v", recommendations[0].Method)
	}
}

func TestSelectorFieldAndLiteralKeyAreNotLocals(t *testing.T) {
	source := "package p\n\ntype cfg struct{ size int }\n\nfunc write(items []string) int {\n\tn := 3\n\tc := cfg{size: n}\n\treturn c.size + len(items)\n}\n"
	methods := aroma.GoLanguage.ParseMethods(source, "p.go")
	if len(methods) != 1 {
		t.Fatalf("methods = %d", len(methods))
	}
	var locals []string
	feats := aroma.Featurize(methods[0].Body, aroma.FeaturizeOptions{})
	for _, entry := range feats {
		if entry.Token.Local {
			locals = append(locals, entry.Token.Text)
		}
	}
	// n and items are locals; the field name size and the literal key size
	// must not read as locals even though a local named size could exist.
	for _, name := range locals {
		if name == "size" {
			t.Fatalf("field name read as a local: %v", locals)
		}
	}
	if !slices.Contains(locals, "n") || !slices.Contains(locals, "items") {
		t.Fatalf("missing locals: %v", locals)
	}
}
