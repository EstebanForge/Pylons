// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"slices"
	"testing"
)

func TestSearchPrunesToTheCommonPart(t *testing.T) {
	index := toyIndex(t, toyMethod("m2", "[ [ y = 2 ] ; ]", "[ [ z = 3 ] ; ]"))
	results := aroma.Search(index, toyQuery(t, index, "[ [ x = 1 ] ; ]", "[ [ y = 2 ] ; ]"), aroma.SearchOptions{})
	if len(results) != 1 || !slices.Equal(results[0].MatchedLines, []int{2}) {
		t.Fatalf("search = %+v, want matchedLines [2]", results)
	}
}

func TestSearchKeepsSwappedNesting(t *testing.T) {
	index := toyIndex(t, toyMethod("m2",
		"[ if [ ( [ z < 0 ] ) ]",
		"[ if [ ( [ y > 1 ] ) ]",
		"[ [ w = 4 ] ; ] ] ]",
		"[ [ v = 10 ] ; ]",
	))
	query := toyQuery(t, index,
		"[ [ x = 1 ] ; ]",
		"[ if [ ( [ y > 1 ] ) ] [ if [ ( [ z < 0 ] ) ] [ [ w = 4 ] ; ] ] ]",
	)
	results := aroma.Search(index, query, aroma.SearchOptions{})
	if len(results) == 0 || !slices.Equal(results[0].MatchedLines, []int{2, 3, 4}) {
		t.Fatalf("search = %+v, want matchedLines [2 3 4]", results)
	}
}

func TestSearchScoresByQueryShare(t *testing.T) {
	// The query has 10 features, 5 per token. `full` matches 4 of each token's
	// (all but the one naming the root, which has three statements, not two);
	// `half` matches 3 of a's (no (a, b) sibling pair) and none of b's.
	index := toyIndex(t, toyCalls("half", "a", "d", "e"), toyCalls("full", "a", "b", "c"))
	results := aroma.Search(index, toyQuery(t, index, callOf("a"), callOf("b")), aroma.SearchOptions{})
	var got []string
	for _, result := range results {
		got = append(got, result.Method.Name)
	}
	if !slices.Equal(got, []string{"full", "half"}) {
		t.Fatalf("order = %v, want [full half]", got)
	}
	if abs64(results[0].Similarity-0.8) > 1e-9 || abs64(results[1].Similarity-0.3) > 1e-9 {
		t.Fatalf("similarities = %v %v, want 0.8 0.3", results[0].Similarity, results[1].Similarity)
	}
	only := aroma.Search(index, toyQuery(t, index, callOf("a"), callOf("b")), aroma.SearchOptions{MinSimilarity: 0.65})
	if len(only) != 1 || only[0].Method.Name != "full" {
		t.Fatalf("minSimilarity 0.65 = %+v, want only full", only)
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestSearchCountsFeaturesOnceInPhaseI(t *testing.T) {
	repeats := toyCalls("repeats", "a", "a", "a", "a", "a", "a", "a", "a", "a", "a")
	index := toyIndex(t, repeats, toyCalls("both", "a", "b"))
	results := aroma.Search(index, toyQuery(t, index, callOf("a"), callOf("b")), aroma.SearchOptions{SearchLimit: 1})
	if len(results) != 1 || results[0].Method.Name != "both" {
		t.Fatalf("search = %+v, want only both", results)
	}
}

func TestSearchReranksByQueryShareWithRepeats(t *testing.T) {
	// `pair` has more of the query's distinct features (7 against 4), so the
	// first phase puts it first. `triple` has more of its features once
	// repeats count (13 against 8 of 22), so reranking puts it first.
	index := toyIndex(t, toyCalls("pair", "a", "b"), toyCalls("triple", "a", "a", "a"))
	query := toyQuery(t, index, callOf("a"), callOf("a"), callOf("a"), callOf("b"))
	results := aroma.Search(index, query, aroma.SearchOptions{})
	var got []string
	for _, result := range results {
		got = append(got, result.Method.Name)
	}
	if !slices.Equal(got, []string{"triple", "pair"}) {
		t.Fatalf("order = %v, want [triple pair]", got)
	}
	for i, want := range []float64{13, 8} {
		if abs64(results[i].Similarity*22-want) > 1e-9 {
			t.Fatalf("similarity %d = %v, want %v/22", i, results[i].Similarity, want)
		}
	}
	top := aroma.Search(index, query, aroma.SearchOptions{SearchLimit: 1})
	if len(top) != 1 || top[0].Method.Name != "pair" {
		t.Fatalf("searchLimit 1 = %+v, want only pair", top)
	}
}

func TestSearchHonorsExcludeAndLanguage(t *testing.T) {
	index := aroma.BuildIndex(
		[]aroma.SourceFile{toyCalls("one", "a", "b"), toyCalls("two", "a", "b", "c"), {Path: "notes.txt", Source: "a b"}},
		aroma.IndexOptions{Languages: []aroma.Language{Language}},
	)
	query := toyQuery(t, index, callOf("a"), callOf("b"))
	results := aroma.Search(index, query, aroma.SearchOptions{Exclude: func(candidate aroma.MethodRef) bool {
		return candidate.Name == "one"
	}})
	var got []string
	for _, result := range results {
		got = append(got, result.Method.Name)
	}
	if !slices.Equal(got, []string{"two"}) {
		t.Fatalf("exclude left %v, want [two]", got)
	}
	other := query
	other.Language = "other"
	if results := aroma.Search(index, other, aroma.SearchOptions{}); len(results) != 0 {
		t.Fatalf("other language returned %+v", results)
	}
	if index.Stats.Files != 2 || index.Stats.Methods != 2 || index.Stats.UnsupportedFiles != 1 {
		t.Fatalf("stats = %+v", index.Stats)
	}
}

func TestSearchFindsCopyWhenOriginExcluded(t *testing.T) {
	// The origin method is left out, but its copy elsewhere answers.
	index := toyIndex(t, toyMethod("m", callOf("a"), callOf("b")), toyMethod("copy", callOf("a"), callOf("b")))
	query := aroma.QueriesFromLines(index, aroma.SourceFile{Path: "m.toy", Source: index.Sources["m.toy"]}, []int{2})
	if len(query) != 1 {
		t.Fatalf("queriesFromLines = %v, want one", query)
	}
	results := aroma.Search(index, query[0], aroma.SearchOptions{})
	if len(results) != 1 || results[0].Method.Name != "copy" {
		t.Fatalf("search = %+v, want the copy", results)
	}
}

func TestIndexDeduplicatesByContent(t *testing.T) {
	index := toyIndex(t, toyCalls("first", "a", "b"), toyCalls("copy", "a", "b"), toyCalls("c", "a"))
	var got []string
	for _, method := range index.Methods {
		got = append(got, method.Name)
	}
	if !slices.Equal(got, []string{"first", "c"}) {
		t.Fatalf("methods = %v, want [first c]", got)
	}
	if index.Stats.Methods != 2 || index.Stats.DuplicateMethods != 1 {
		t.Fatalf("stats = %+v", index.Stats)
	}
}
