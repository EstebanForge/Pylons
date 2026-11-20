// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

import "slices"

// SearchOptions tune search.
type SearchOptions struct {
	// SearchLimit is how many methods the light-weight search keeps for
	// reranking (the paper's η1).
	SearchLimit int
	// Limit is the most results to return.
	Limit int
	// MinSimilarity is the least similarity a result must show, from 0 to 1.
	MinSimilarity float64
	// Exclude leaves methods out, such as ones the pull request under review
	// added. A copy elsewhere of a method left out still counts.
	Exclude func(MethodRef) bool
	// IncludeOrigin keeps the method a query was cut from in the results.
	// QueriesFromLines excludes it by default; the recall benchmark wants it
	// kept, because the original method is the expected answer.
	IncludeOrigin bool
}

// DefaultSearchLimit is η1: how many methods the light-weight search keeps.
const DefaultSearchLimit = 1000

// DefaultResultLimit is the most results search returns.
const DefaultResultLimit = 100

// SearchResult is a method that contains code like the query.
type SearchResult struct {
	Method MethodRef `json:"method"`
	// Similarity is the share of the query's features found in the part of
	// the method that resembles it, from 0 to 1. The paper treats 0.71 and up
	// as close enough to reuse the existing code with small changes
	// (Section 2).
	Similarity float64 `json:"similarity"`
	// MatchedLines are the lines of the method's file that resemble the
	// query.
	MatchedLines []int `json:"matchedLines"`
}

// Ranked is a method of the reranked list, at the first of its locations the
// search may use, with the tokens pruning kept.
type Ranked struct {
	ID     int
	Method IndexedMethod
	Pruned Pruned
}

// Rank is Phases I and II of the paper. The light-weight search scores every
// method by how many of the query's distinct features it has and keeps the
// top SearchLimit. Each of those is then pruned against the query, and the
// list is reordered by how similar the pruned snippet is to the query, which
// says how well a method contains the query instead of how much it shares
// with it.
func Rank(index *Index, query Query, options SearchOptions) []Ranked {
	overlap := make([]int32, len(index.Methods))
	for feature := range query.Features {
		end := index.PostingOffsets[feature+1]
		for at := index.PostingOffsets[feature]; at < end; at++ {
			overlap[index.Postings[at]]++
		}
	}
	origin := query.Origin
	usable := func(location MethodLocation) bool {
		if !options.IncludeOrigin && origin != nil &&
			location.Path == origin.Path &&
			location.StartLine <= origin.EndLine &&
			location.EndLine >= origin.StartLine {
			return false
		}
		return !(options.Exclude != nil && options.Exclude(location.MethodRef))
	}
	var candidates []int
	locations := make(map[int]MethodLocation)
	for id, method := range index.Methods {
		if overlap[id] == 0 || method.Language != query.Language {
			continue
		}
		// The query's own method, or one the caller excludes, may have a copy
		// elsewhere.
		location := method.MethodLocation
		if !usable(location) {
			var found MethodLocation
			foundKnown := false
			for _, copy := range method.Copies {
				if usable(copy) {
					found = copy
					foundKnown = true
					break
				}
			}
			if !foundKnown {
				continue
			}
			location = found
		}
		candidates = append(candidates, id)
		locations[id] = location
	}
	slices.SortStableFunc(candidates, func(a, b int) int {
		if overlap[a] != overlap[b] {
			return int(overlap[b]) - int(overlap[a])
		}
		return a - b
	})

	limit := options.SearchLimit
	if limit == 0 {
		limit = DefaultSearchLimit
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	ranked := make([]Ranked, 0, len(candidates))
	for _, id := range candidates {
		indexed := index.Methods[id]
		location := locations[id]
		if location.MethodRef != indexed.MethodRef {
			indexed.MethodLocation = location
		}
		ranked = append(ranked, Ranked{ID: id, Method: *indexed, Pruned: Prune(query.Features, indexed.TokenTable, nil)})
	}
	slices.SortStableFunc(ranked, func(a, b Ranked) int {
		return b.Pruned.Score - a.Pruned.Score
	})
	return ranked
}

// Search is the methods of the corpus that contain code like the query, most
// similar first. This is Aroma's structural code-to-code search on its own,
// without the clustering that turns results into recommendations.
func Search(index *Index, query Query, options SearchOptions) []SearchResult {
	limit := options.Limit
	if limit == 0 {
		limit = DefaultResultLimit
	}
	var results []SearchResult
	for _, ranked := range Rank(index, query, options) {
		similarity := float64(ranked.Pruned.Score) / float64(query.Size)
		if similarity > options.MinSimilarity && len(results) < limit {
			results = append(results, SearchResult{
				Method:       ranked.Method.MethodRef,
				Similarity:   similarity,
				MatchedLines: LinesOf(ranked.Method, ranked.Pruned.Tokens),
			})
		}
	}
	return results
}

// LinesOf is every line the given tokens of a method are on, ascending.
func LinesOf(method IndexedMethod, tokens []int) []int {
	seen := map[int]bool{}
	var lines []int
	for _, token := range tokens {
		for line := method.TokenLines[token]; line <= method.TokenEndLines[token]; line++ {
			if !seen[int(line)] {
				seen[int(line)] = true
				lines = append(lines, int(line))
			}
		}
	}
	slices.Sort(lines)
	return lines
}
