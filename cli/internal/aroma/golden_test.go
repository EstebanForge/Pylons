// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"encoding/json"
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The goldens in testdata/goldens/core.json were produced by
// scripts/gengoldens.ts running the TypeScript reference implementation over
// the same toy corpus; this file replays them through the Go port. Refreshing
// the goldens needs Node once; the replay never does.

type goldenFile struct {
	GeneratedFrom struct {
		Commit string `json:"commit"`
	} `json:"_generated_from"`
	Listing4Tokens []struct {
		Text     string   `json:"text"`
		Local    bool     `json:"local"`
		Features []string `json:"features"`
	} `json:"listing4Tokens"`
	Search struct {
		Swap   []aroma.SearchResult `json:"swap"`
		Rerank []aroma.SearchResult `json:"rerank"`
	} `json:"search"`
	Recommend struct {
		Families           []aroma.Recommendation `json:"families"`
		FamiliesMinSupport []aroma.Recommendation `json:"familiesMinSupport1"`
		Single             []aroma.Recommendation `json:"single"`
		Apartment          []aroma.Recommendation `json:"apart"`
		Mixed              []aroma.Recommendation `json:"mixed"`
	} `json:"recommend"`
	Lines []struct {
		Size    int                  `json:"size"`
		Origin  aroma.MethodRef      `json:"origin"`
		Lines   []int                `json:"lines"`
		Results []aroma.SearchResult `json:"results"`
	} `json:"lines"`
}

func loadGoldens(t *testing.T) goldenFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "goldens", "core.json"))
	if err != nil {
		t.Fatalf("goldens: %v", err)
	}
	var goldens goldenFile
	if err := json.Unmarshal(raw, &goldens); err != nil {
		t.Fatalf("goldens: %v", err)
	}
	if goldens.GeneratedFrom.Commit == "" {
		t.Fatalf("goldens lack the reference commit")
	}
	return goldens
}

func toyListings(name string, statements ...string) aroma.SourceFile {
	return toyMethod(name, statements...)
}

func goldenFamily(prefix string, shared ...string) []aroma.SourceFile {
	var files []aroma.SourceFile
	for _, number := range []string{"1", "2", "3"} {
		called := append([]string{"open", "read"}, shared...)
		called = append(called, prefix+number+"_only")
		files = append(files, toyCalls(prefix+number, called...))
	}
	return files
}

func TestGoldenListing4Tokens(t *testing.T) {
	goldens := loadGoldens(t)
	tokens := aroma.Featurize(snippet(t, listing4), aroma.FeaturizeOptions{})
	if len(tokens) != len(goldens.Listing4Tokens) {
		t.Fatalf("tokens = %d, golden = %d", len(tokens), len(goldens.Listing4Tokens))
	}
	for i, entry := range tokens {
		want := goldens.Listing4Tokens[i]
		gotFeatures := slices.Clone(entry.Features)
		slices.Sort(gotFeatures)
		if entry.Token.Text != want.Text || entry.Token.Local != want.Local || !slices.Equal(gotFeatures, want.Features) {
			t.Fatalf("token %d = %+v, want %+v", i, entry, want)
		}
	}
}

func goldenIndex(t *testing.T, files ...aroma.SourceFile) *aroma.Index {
	t.Helper()
	return toyIndex(t, files...)
}

func TestGoldenSearch(t *testing.T) {
	goldens := loadGoldens(t)

	swap := goldenIndex(t, toyListings("m2",
		"[ if [ ( [ z < 0 ] ) ]",
		"[ if [ ( [ y > 1 ] ) ]",
		"[ [ w = 4 ] ; ] ] ]",
		"[ [ v = 10 ] ; ]",
	))
	swapQuery := toyQuery(t, swap,
		"[ [ x = 1 ] ; ]",
		"[ if [ ( [ y > 1 ] ) ] [ if [ ( [ z < 0 ] ) ] [ [ w = 4 ] ; ] ] ]",
	)
	assertResultsEqual(t, "swap", aroma.Search(swap, swapQuery, aroma.SearchOptions{}), goldens.Search.Swap)

	rerank := goldenIndex(t, toyCalls("pair", "a", "b"), toyCalls("triple", "a", "a", "a"))
	rerankQuery := toyQuery(t, rerank, callOf("a"), callOf("a"), callOf("a"), callOf("b"))
	assertResultsEqual(t, "rerank", aroma.Search(rerank, rerankQuery, aroma.SearchOptions{}), goldens.Search.Rerank)
}

func assertResultsEqual(t *testing.T, name string, got, want []aroma.SearchResult) {
	t.Helper()
	rawGot, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	rawWant, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawGot) != string(rawWant) {
		t.Fatalf("%s search:\n got %s\nwant %s", name, rawGot, rawWant)
	}
}

func TestGoldenRecommend(t *testing.T) {
	goldens := loadGoldens(t)

	// The build order is the generator's: x1, x2, x3, y1, y2, y3.
	x := goldenFamily("x", "flush", "close")
	y := goldenFamily("y", "log", "retry")
	families := goldenIndex(t, x[0], x[1], x[2], y[0], y[1], y[2])
	openRead := toyQuery(t, families, callOf("open"), callOf("read"))
	assertRecommendationsEqual(t, "families",
		aroma.Recommend(families, openRead, aroma.RecommendOptions{}), goldens.Recommend.Families)
	assertRecommendationsEqual(t, "familiesMinSupport1",
		aroma.Recommend(families, openRead, aroma.RecommendOptions{MinSupport: 1}), goldens.Recommend.FamiliesMinSupport)

	single := goldenIndex(t, toyCalls("same", "open", "read"), toyCalls("one_more", "open", "read", "x"))
	singleQuery := toyQuery(t, single, callOf("open"), callOf("read"))
	assertRecommendationsEqual(t, "single",
		aroma.Recommend(single, singleQuery, aroma.RecommendOptions{MinSupport: 1, MinExpansion: 1.4}), goldens.Recommend.Single)

	apartment := goldenIndex(t,
		toyCalls("m1", "a", "b", "c", "x", "y", "z"),
		toyCalls("m2", "b", "c", "d", "x", "y", "z"),
	)
	apartmentQuery := toyQuery(t, apartment, callOf("a"), callOf("b"), callOf("c"), callOf("d"))
	assertRecommendationsEqual(t, "apart",
		aroma.Recommend(apartment, apartmentQuery, aroma.RecommendOptions{MinSimilarity: 0.3}), goldens.Recommend.Apartment)

	mixed := goldenIndex(t,
		toyCalls("whole", "a", "b", "c", "x", "y", "z", "w"),
		toyCalls("partial", "a", "b", "x", "y", "z", "w"),
	)
	mixedQuery := toyQuery(t, mixed, callOf("a"), callOf("b"), callOf("c"))
	assertRecommendationsEqual(t, "mixed",
		aroma.Recommend(mixed, mixedQuery, aroma.RecommendOptions{MinSimilarity: 0.4, MinCohesion: 0.4}), goldens.Recommend.Mixed)
}

func assertRecommendationsEqual(t *testing.T, name string, got, want []aroma.Recommendation) {
	t.Helper()
	if got == nil {
		got = []aroma.Recommendation{}
	}
	rawGot, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	rawWant, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(rawGot) != string(rawWant) {
		t.Fatalf("%s recommend:\n got %s\nwant %s", name, rawGot, rawWant)
	}
}

func TestGoldenQueriesFromLines(t *testing.T) {
	goldens := loadGoldens(t)
	x := goldenFamily("x", "flush", "close")
	y := goldenFamily("y", "log", "retry")
	families := goldenIndex(t, x[0], x[1], x[2], y[0], y[1], y[2])
	x1 := families.Sources["x1.toy"]
	queries := aroma.QueriesFromLines(families, aroma.SourceFile{Path: "x1.toy", Source: x1}, []int{2, 3})
	if len(queries) != len(goldens.Lines) {
		t.Fatalf("queries = %d, golden = %d", len(queries), len(goldens.Lines))
	}
	for i, query := range queries {
		want := goldens.Lines[i]
		if query.Size != want.Size || !slices.Equal(query.Lines, want.Lines) || query.Origin == nil || *query.Origin != want.Origin {
			t.Fatalf("query %d = size %d lines %v origin %+v, want %+v", i, query.Size, query.Lines, query.Origin, want.Origin)
		}
		assertResultsEqual(t, "lines", aroma.Search(families, query, aroma.SearchOptions{}), want.Results)
	}
}
