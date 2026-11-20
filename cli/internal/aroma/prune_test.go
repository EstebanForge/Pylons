// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"fmt"
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"math/rand"
	"slices"
	"testing"
)

// naivePrune is Algorithm 1 as written: rescan every remaining token for the
// best gain at each step.
func naivePrune(target aroma.Multiset, table aroma.TokenTable, candidates []int) aroma.Pruned {
	featuresOfToken := func(token int) []int32 {
		return table.Features[table.Offsets[token]:table.Offsets[token+1]]
	}
	score := func(tokens []int) int {
		have := map[int32]int{}
		for _, token := range tokens {
			for _, feature := range featuresOfToken(token) {
				have[feature]++
			}
		}
		total := 0
		for feature, count := range have {
			total += min(count, target[int(feature)])
		}
		return total
	}
	var kept []int
	for {
		best := -1
		bestScore := score(kept)
		for _, token := range candidates {
			if !slices.Contains(kept, token) {
				if next := score(append(slices.Clone(kept), token)); next > bestScore {
					best = token
					bestScore = next
				}
			}
		}
		if best < 0 {
			slices.Sort(kept)
			return aroma.Pruned{Tokens: kept, Score: bestScore}
		}
		kept = append(kept, best)
	}
}

func table(tokens [][]int32) aroma.TokenTable {
	offsets := make([]uint32, len(tokens)+1)
	var features []int32
	for i, token := range tokens {
		features = append(features, token...)
		offsets[i+1] = uint32(len(features))
	}
	return aroma.TokenTable{Features: features, Offsets: offsets}
}

func TestPruneRespectsTargetCapacity(t *testing.T) {
	tokens := table([][]int32{{7}, {7}, {7, 8}})
	got := aroma.Prune(aroma.Multiset{7: 1, 8: 1}, tokens, nil)
	want := aroma.Pruned{Tokens: []int{2}, Score: 2}
	if !slices.Equal(got.Tokens, want.Tokens) || got.Score != want.Score {
		t.Fatalf("Prune = %+v, want %+v", got, want)
	}
	got = aroma.Prune(aroma.Multiset{7: 2, 8: 1}, tokens, nil)
	want = aroma.Pruned{Tokens: []int{0, 2}, Score: 3}
	if !slices.Equal(got.Tokens, want.Tokens) || got.Score != want.Score {
		t.Fatalf("Prune = %+v, want %+v", got, want)
	}
}

func TestPruneKeepsEverythingAgainstItself(t *testing.T) {
	tokens := table([][]int32{{1, 2, 2}, {3}, {1, 4}})
	target := aroma.Multiset{1: 2, 2: 2, 3: 1, 4: 1}
	got := aroma.Prune(target, tokens, nil)
	if !slices.Equal(got.Tokens, []int{0, 1, 2}) || got.Score != 6 {
		t.Fatalf("Prune = %+v, want all tokens, score 6", got)
	}
}

func TestPruneMatchesNaiveRescan(t *testing.T) {
	random := rand.New(rand.NewSource(20181201))
	for round := 0; round < 300; round++ {
		alphabet := 1 + random.Intn(12)
		var tokens [][]int32
		count := 1 + random.Intn(30)
		for i := 0; i < count; i++ {
			var token []int32
			for j := 0; j < 1+random.Intn(6); j++ {
				token = append(token, int32(random.Intn(alphabet)))
			}
			tokens = append(tokens, token)
		}
		target := aroma.Multiset{}
		for feature := 0; feature < alphabet; feature++ {
			if c := random.Intn(4); c > 0 {
				target[feature] = c
			}
		}
		all := make([]int, count)
		for i := range all {
			all[i] = i
		}
		candidates := all
		if round%3 == 0 {
			// An empty non-nil slice must stay empty, not nil: nil means all
			// tokens in Prune, an empty slice means none.
			candidates = []int{}
			for _, token := range all {
				if random.Intn(2) == 0 {
					candidates = append(candidates, token)
				}
			}
		}
		got := aroma.Prune(target, table(tokens), candidates)
		want := naivePrune(target, table(tokens), candidates)
		if !slices.Equal(got.Tokens, want.Tokens) || got.Score != want.Score {
			t.Fatalf("round %d: Prune = %+v, naive = %+v", round, got, want)
		}
	}
}

func TestPruneRespectsCandidates(t *testing.T) {
	tokens := table([][]int32{{1}, {2}, {3}})
	got := aroma.Prune(aroma.Multiset{1: 1, 2: 1, 3: 1}, tokens, []int{0, 1})
	if !slices.Equal(got.Tokens, []int{0, 1}) || got.Score != 2 {
		t.Fatalf("Prune with candidates = %+v, want tokens 0,1 score 2", got)
	}
}

func fmtRef(m aroma.MethodRef) string {
	return fmt.Sprintf("%s:%s:%d-%d", m.Path, m.Name, m.StartLine, m.EndLine)
}

// Corpus helpers mirroring the reference test suite.

func toyMethod(name string, statements ...string) aroma.SourceFile {
	lines := []string{"method " + name}
	lines = append(lines, statements...)
	lines = append(lines, "end")
	return aroma.SourceFile{Path: name + ".toy", Source: joinLines(lines)}
}

func joinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}

func callOf(name string) string { return "[ [ " + name + " ( ) ] ; ]" }

func toyCalls(name string, called ...string) aroma.SourceFile {
	statements := make([]string, len(called))
	for i, callee := range called {
		statements[i] = callOf(callee)
	}
	return toyMethod(name, statements...)
}

func toyIndex(t *testing.T, files ...aroma.SourceFile) *aroma.Index {
	t.Helper()
	return aroma.BuildIndex(files, aroma.IndexOptions{Languages: []aroma.Language{Language}})
}

func toyQuery(t *testing.T, index *aroma.Index, statements ...string) aroma.Query {
	t.Helper()
	query, ok := aroma.QueryFromSnippet(index, joinLines(statements), aroma.SnippetOptions{}, "toy")
	if !ok {
		t.Fatalf("toy query from %d statements returned nothing", len(statements))
	}
	return query
}

func namesOf(refs []aroma.MethodRef) []string {
	out := make([]string, len(refs))
	for i, ref := range refs {
		out[i] = ref.Name
	}
	return out
}
