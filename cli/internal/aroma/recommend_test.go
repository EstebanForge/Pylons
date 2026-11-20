// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"slices"
	"strings"
	"testing"
)

// Two families contain the query `open(); read();`. One goes on to flush and
// close, the other to log and retry, and every method ends with a call of its
// own. Hand-computed for the query and X1: |F(q)| = 10; similarity 0.8;
// |F(X1)| = 28; |F(Prune(F(q), X1))| = 11. Adding X2: cs = 22, csq = 11, so
// l = 2 and s = 1. Adding Y1 instead: cs = 10, csq = 10, so l = 1 < 1.5.
func toyFamily(prefix string, shared ...string) []aroma.SourceFile {
	var files []aroma.SourceFile
	for _, number := range []string{"1", "2", "3"} {
		called := append([]string{"open", "read"}, shared...)
		called = append(called, prefix+number+"_only")
		files = append(files, toyCalls(prefix+number, called...))
	}
	return files
}

func recommendFamilies(t *testing.T) (*aroma.Index, aroma.Query) {
	t.Helper()
	// The build order is the reference test's: the families interleave, and
	// the id tie-break makes that order significant.
	x := toyFamily("x", "flush", "close")
	y := toyFamily("y", "log", "retry")
	index := toyIndex(t, x[0], y[0], x[1], y[1], x[2], y[2])
	return index, toyQuery(t, index, callOf("open"), callOf("read"))
}

func memberNames(recommendations []aroma.Recommendation) [][]string {
	var out [][]string
	for _, recommendation := range recommendations {
		out = append(out, namesOf(recommendation.Members))
	}
	return out
}

func TestRecommendOneSnippetPerFamily(t *testing.T) {
	index, query := recommendFamilies(t)
	recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{})
	want := [][]string{{"x1", "x2", "x3"}, {"y1", "y2", "y3"}}
	got := memberNames(recommendations)
	if !slices.Equal(got[0], want[0]) || !slices.Equal(got[1], want[1]) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	first := recommendations[0]
	if first.Method.Path != "x1.toy" || first.Method.Name != "x1" || first.Method.StartLine != 2 || first.Method.EndLine != 6 {
		t.Fatalf("method = %+v", first.Method)
	}
	if abs64(first.Similarity-0.8) > 1e-9 {
		t.Fatalf("similarity = %v, want 0.8", first.Similarity)
	}
	wantLines := []aroma.RecommendedLine{
		{Line: 2, Text: callOf("open"), Role: aroma.RoleQuery},
		{Line: 3, Text: callOf("read"), Role: aroma.RoleQuery},
		{Line: 4, Text: callOf("flush"), Role: aroma.RoleAdded},
		{Line: 5, Text: callOf("close"), Role: aroma.RoleAdded},
	}
	if !slices.Equal(first.Lines, wantLines) {
		t.Fatalf("lines = %+v, want %+v", first.Lines, wantLines)
	}
	wantCode := strings.Join([]string{callOf("open"), callOf("read"), callOf("flush"), callOf("close")}, "\n")
	if first.Code != wantCode {
		t.Fatalf("code = %q, want %q", first.Code, wantCode)
	}
	if recommendations[1].Lines[2].Text != callOf("log") || recommendations[1].Lines[3].Text != callOf("retry") {
		t.Fatalf("second family lines wrong: %+v", recommendations[1].Lines)
	}
}

func TestRecommendDropsMostlyRepeatedClusters(t *testing.T) {
	index, query := recommendFamilies(t)
	recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSupport: 1})
	got := memberNames(recommendations)
	want := [][]string{{"x1", "x2", "x3"}, {"y1", "y2", "y3"}, {"x3"}, {"y3"}}
	for i, names := range want {
		if i >= len(got) || !slices.Equal(got[i], names) {
			t.Fatalf("clusters = %v, want %v", got, want)
		}
	}
	limited := aroma.Recommend(index, query, aroma.RecommendOptions{Limit: 1})
	if len(limited) != 1 || !slices.Equal(memberNames(limited)[0], want[0]) {
		t.Fatalf("limit 1 = %v, want %v", memberNames(limited), want[0])
	}
}

func TestRecommendUnclusteredWholeBody(t *testing.T) {
	index := toyIndex(t, toyCalls("only", "open", "read", "flush", "close"))
	query := toyQuery(t, index, callOf("open"), callOf("read"))
	if recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{}); len(recommendations) != 0 {
		t.Fatalf("default minSupport returned %+v", recommendations)
	}
	recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSupport: 1})
	if len(recommendations) != 1 {
		t.Fatalf("minSupport 1 = %+v, want one", recommendations)
	}
	recommendation := recommendations[0]
	if !slices.Equal(namesOf(recommendation.Members), []string{"only"}) {
		t.Fatalf("members = %v", namesOf(recommendation.Members))
	}
	var roles []aroma.RecommendedLineRole
	for _, line := range recommendation.Lines {
		roles = append(roles, line.Role)
	}
	if !slices.Equal(roles, []aroma.RecommendedLineRole{aroma.RoleQuery, aroma.RoleQuery, aroma.RoleAdded, aroma.RoleAdded}) {
		t.Fatalf("roles = %v", roles)
	}
}

func TestRecommendNothingFromTooLittleAddition(t *testing.T) {
	// |F(same)| / |F(pruned)| = 1 and |F(one_more)| / |F(pruned)| = 16 / 11.
	index := toyIndex(t, toyCalls("same", "open", "read"), toyCalls("one_more", "open", "read", "x"))
	query := toyQuery(t, index, callOf("open"), callOf("read"))
	if recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSupport: 1}); len(recommendations) != 0 {
		t.Fatalf("default expansion returned %+v", recommendations)
	}
	recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSupport: 1, MinExpansion: 1.4})
	if len(recommendations) != 1 || !slices.Equal(namesOf(recommendations[0].Members), []string{"one_more"}) {
		t.Fatalf("expansion 1.4 = %+v, want one_more", recommendations)
	}
}

func TestRecommendKeepsDifferentQueryPartsApart(t *testing.T) {
	// m1 has a, b, c of the query and m2 has b, c, d. They share 24 features,
	// 3 times the 8 their query-like parts share, but those 8 are under half
	// of the 17 in m1's query-like part.
	index := toyIndex(t,
		toyCalls("m1", "a", "b", "c", "x", "y", "z"),
		toyCalls("m2", "b", "c", "d", "x", "y", "z"),
	)
	query := toyQuery(t, index, callOf("a"), callOf("b"), callOf("c"), callOf("d"))
	options := aroma.RecommendOptions{MinSimilarity: 0.3, MinSupport: 1}
	got := memberNames(aroma.Recommend(index, query, options))
	if len(got) != 2 || !slices.Equal(got[0], []string{"m1"}) || !slices.Equal(got[1], []string{"m2"}) {
		t.Fatalf("clusters = %v, want m1 and m2 apart", got)
	}
	if recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSimilarity: 0.3}); len(recommendations) != 0 {
		t.Fatalf("default support returned %+v", recommendations)
	}
	joined := memberNames(aroma.Recommend(index, query, aroma.RecommendOptions{MinSimilarity: 0.3, MinCohesion: 0.4}))
	if len(joined) != 1 || !slices.Equal(joined[0], []string{"m1", "m2"}) {
		t.Fatalf("cohesion 0.4 = %v, want [m1 m2]", joined)
	}
}

func TestRecommendKeepsQueryCodeTheClusterDoesNotShare(t *testing.T) {
	// `partial` lacks `c();`, so intersecting `whole` with it alone would drop
	// that line; the thresholds are lowered to let the two cluster at all.
	index := toyIndex(t,
		toyCalls("whole", "a", "b", "c", "x", "y", "z", "w"),
		toyCalls("partial", "a", "b", "x", "y", "z", "w"),
	)
	query := toyQuery(t, index, callOf("a"), callOf("b"), callOf("c"))
	recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{MinSimilarity: 0.4, MinCohesion: 0.4})
	if len(recommendations) != 1 || !slices.Equal(namesOf(recommendations[0].Members), []string{"whole", "partial"}) {
		t.Fatalf("recommendations = %+v", recommendations)
	}
	var got [][2]string
	for _, line := range recommendations[0].Lines {
		got = append(got, [2]string{line.Text, string(line.Role)})
	}
	want := [][2]string{
		{callOf("a"), "query"}, {callOf("b"), "query"}, {callOf("c"), "query"},
		{callOf("x"), "added"}, {callOf("y"), "added"}, {callOf("z"), "added"}, {callOf("w"), "added"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}

func TestRecommendNothingBelowSimilarity(t *testing.T) {
	index := toyIndex(t, toyCalls("far", "a", "x", "y", "z", "w"))
	recommendations := aroma.Recommend(index, toyQuery(t, index, callOf("a"), callOf("b")), aroma.RecommendOptions{MinSupport: 1})
	if len(recommendations) != 0 {
		t.Fatalf("recommendations = %+v, want none", recommendations)
	}
}
