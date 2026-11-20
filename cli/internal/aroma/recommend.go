// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

import (
	"sort"
	"strings"
)

// RecommendOptions tune recommendations. Its search fields carry into the
// ranking phases.
type RecommendOptions struct {
	SearchOptions
	// ClusterLimit is how many of the most similar methods are clustered
	// (the paper's η2).
	ClusterLimit int
	// MinSimilarity is the least similarity a clustered method shows (τ1).
	MinSimilarity float64
	// MinExpansion is the least ratio of a cluster's shared features to the
	// features its query-like parts share (τ2), so that a recommendation
	// adds to the query.
	MinExpansion float64
	// MinCohesion is the least share the query-like parts of a cluster's
	// methods share of the first method's query-like part (τ3), so that the
	// methods contain the same part of the query.
	MinCohesion float64
	// MaxClusterOverlap drops a cluster that shares more than this share of
	// its members with an earlier one.
	MaxClusterOverlap float64
	// Limit is the most recommendations to return (K).
	Limit int
	// MinSupport is the fewest methods a recommendation must come from: 2
	// when unset, so that only code that recurs is recommended. Pass 1 for
	// the paper's behavior, which also returns each unclustered method
	// whole.
	MinSupport int
}

// Defaults are the paper's and the reference implementation's values.
const (
	DefaultClusterLimit      = 100
	DefaultMinSimilarity     = 0.65
	DefaultMinExpansion      = 1.5
	DefaultMinCohesion       = 0.9
	DefaultMaxClusterOverlap = 0.5
	DefaultLimit             = 5
	DefaultMinSupport        = 2
)

// RecommendedLineRole says what one line of a recommendation is.
type RecommendedLineRole string

const (
	RoleQuery     RecommendedLineRole = "query"
	RoleAdded     RecommendedLineRole = "added"
	RoleStructure RecommendedLineRole = "structure"
)

// RecommendedLine is one line of a recommendation.
type RecommendedLine struct {
	// Line is the 1-based line of the recommendation's method's file.
	Line int    `json:"line"`
	Text string `json:"text"`
	// Role says what the line is: `query` when every name and literal kept on
	// it resembles the query, `added` when it has code the cluster's methods
	// share beyond the query, and `structure` when it holds only keywords and
	// punctuation around such lines.
	Role RecommendedLineRole `json:"role"`
}

// Recommendation is code that several methods of the corpus have around code
// like the query.
type Recommendation struct {
	// Method is the method the lines are taken from: the cluster's most
	// similar one.
	Method MethodRef `json:"method"`
	// Members are every method of the cluster, Method first.
	Members []MethodRef `json:"members"`
	// Similarity is how similar Method is to the query, from 0 to 1.
	Similarity float64 `json:"similarity"`
	// Lines are the lines of Method that the cluster's methods share.
	Lines []RecommendedLine `json:"lines"`
	// Code is the text of Lines, without their common indentation.
	Code string `json:"code"`
}

// Recommend is Aroma's recommendations for a query (Section 3.3): search for
// the methods that contain code like it, cluster those that are alike, and
// intersect each cluster down to the code its methods share. Each
// recommendation contains the query, approximately, and code that goes with
// it in the corpus.
func Recommend(index *Index, query Query, options RecommendOptions) []Recommendation {
	clusterLimit := options.ClusterLimit
	if clusterLimit == 0 {
		clusterLimit = DefaultClusterLimit
	}
	minSimilarity := options.MinSimilarity
	if minSimilarity == 0 {
		minSimilarity = DefaultMinSimilarity
	}
	minExpansion := options.MinExpansion
	if minExpansion == 0 {
		minExpansion = DefaultMinExpansion
	}
	minCohesion := options.MinCohesion
	if minCohesion == 0 {
		minCohesion = DefaultMinCohesion
	}
	maxOverlap := options.MaxClusterOverlap
	if maxOverlap == 0 {
		maxOverlap = DefaultMaxClusterOverlap
	}
	limit := options.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	minSupport := options.MinSupport
	if minSupport == 0 {
		minSupport = DefaultMinSupport
	}

	candidates := Rank(index, query, options.SearchOptions)
	kept := candidates[:0]
	for _, ranked := range candidates {
		if float64(ranked.Pruned.Score)/float64(query.Size) > minSimilarity {
			kept = append(kept, ranked)
		}
	}
	if len(kept) > clusterLimit {
		kept = kept[:clusterLimit]
	}
	parsed := make(map[string][]Method)
	var out []Recommendation
	for _, members := range cluster(kept, minExpansion, minCohesion, maxOverlap) {
		if len(members) < minSupport {
			continue
		}
		if len(out) >= limit {
			break
		}
		first := kept[members[0]]
		keptTokens := intersect(kept, members, query)
		lines := render(index, first.Method, keptTokens, first.Pruned.Tokens, parsed)
		refLines := make([]MethodRef, len(members))
		for i, member := range members {
			refLines[i] = kept[member].Method.MethodRef
		}
		out = append(out, Recommendation{
			Method:     first.Method.MethodRef,
			Members:    refLines,
			Similarity: float64(first.Pruned.Score) / float64(query.Size),
			Lines:      lines,
			Code:       strings.Join(dedentLines(lines), "\n"),
		})
	}
	return out
}

// cluster builds clusters of the reranked methods, as lists of positions in
// candidates.
//
// A cluster is valid when the features all its methods share (cs) outnumber
// the features their query-like parts share (csq) by minExpansion, and csq is
// at least minCohesion of the first method's query-like part. Each valid
// single method grows by the later method that keeps the cluster valid with
// the highest cs / csq, until none does. Clusters are then ordered by their
// first method, and one that mostly repeats an earlier cluster is dropped.
func cluster(candidates []Ranked, minExpansion, minCohesion, maxOverlap float64) [][]int {
	whole := make([]Multiset, len(candidates))
	queryLike := make([]Multiset, len(candidates))
	for i, candidate := range candidates {
		whole[i] = featuresOf(candidate.Method.TokenTable, nil)
		queryLike[i] = featuresOf(candidate.Method.TokenTable, candidate.Pruned.Tokens)
	}
	var clusters [][]int
	for seed := range candidates {
		seedSize := multisetSize(queryLike[seed])
		if seedSize == 0 || float64(multisetSize(whole[seed]))/float64(seedSize) <= minExpansion {
			continue
		}
		members := []int{seed}
		shared := whole[seed]
		sharedQueryLike := queryLike[seed]
		for {
			var best struct {
				member    int
				expansion float64
			}
			found := false
			for member := members[len(members)-1] + 1; member < len(candidates); member++ {
				csq := intersectionSize(sharedQueryLike, queryLike[member])
				if csq == 0 || float64(csq)/float64(seedSize) <= minCohesion {
					continue
				}
				expansion := float64(intersectionSize(shared, whole[member])) / float64(csq)
				if expansion > minExpansion && (!found || expansion > best.expansion) {
					best = struct {
						member    int
						expansion float64
					}{member, expansion}
					found = true
				}
			}
			if !found {
				break
			}
			members = append(members, best.member)
			shared = intersection(shared, whole[best.member])
			sharedQueryLike = intersection(sharedQueryLike, queryLike[best.member])
		}
		clusters = append(clusters, members)
	}
	var out [][]int
	for position, members := range clusters {
		redundant := false
		for _, earlier := range clusters[:position] {
			if jaccard(earlier, members) > maxOverlap {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, members)
		}
	}
	return out
}

// intersect is the tokens of a cluster's first method that its other methods
// have too (Intersect in the paper): the first method pruned against each
// other method in turn. The query's features are added to each target so that
// code like the query is never pruned away. A cluster of one is the whole
// method (Section 4.3.5).
func intersect(candidates []Ranked, members []int, query Query) []int {
	first := candidates[members[0]].Method
	// wholeMethod distinguishes "no prune ran yet" (the first member is
	// kept whole) from "the prune kept nothing", which mirrors the
	// reference's undefined-versus-empty distinction.
	wholeMethod := true
	var kept []int
	for _, member := range members[1:] {
		target := featuresOf(candidates[member].Method.TokenTable, nil)
		for feature, count := range query.Features {
			target[feature] += count
		}
		kept = Prune(target, first.TokenTable, keptOrNil(wholeMethod, kept)).Tokens
		wholeMethod = false
	}
	if wholeMethod {
		tokens := make([]int, len(first.Offsets)-1)
		for token := range tokens {
			tokens[token] = token
		}
		return tokens
	}
	return kept
}

// keptOrNil maps the whole-method state to Prune's candidates slice: nil
// means every token, an empty slice means none.
func keptOrNil(wholeMethod bool, kept []int) []int {
	if wholeMethod {
		return nil
	}
	return kept
}

// featuresOf is the features of a method's tokens, or of the given ones, as a
// multiset.
func featuresOf(table TokenTable, tokens []int) Multiset {
	counts := Multiset{}
	add := func(start, end uint32) {
		for at := start; at < end; at++ {
			counts[int(table.Features[at])]++
		}
	}
	if tokens != nil {
		for _, token := range tokens {
			add(table.Offsets[token], table.Offsets[token+1])
		}
	} else {
		add(0, uint32(len(table.Features)))
	}
	return counts
}

func multisetSize(features Multiset) int {
	total := 0
	for _, count := range features {
		total += count
	}
	return total
}

func intersection(a, b Multiset) Multiset {
	small, large := a, b
	if len(small) > len(large) {
		small, large = large, small
	}
	shared := Multiset{}
	for feature, count := range small {
		if other, known := large[feature]; known {
			shared[feature] = min(count, other)
		}
	}
	return shared
}

func intersectionSize(a, b Multiset) int {
	small, large := a, b
	if len(small) > len(large) {
		small, large = large, small
	}
	total := 0
	for feature, count := range small {
		total += min(count, large[feature])
	}
	return total
}

func jaccard(a, b []int) float64 {
	shared := 0
	for _, member := range a {
		if sliceContains(b, member) {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

func sliceContains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// render is the lines of a method that a pruned snippet covers: the lines of
// its kept tokens, and of the keywords and punctuation of every tree on the
// way from a kept token to the root, which is what the paper's pruned tree
// retains.
func render(index *Index, method IndexedMethod, kept []int, queryLike []int, parsed map[string][]Method) []RecommendedLine {
	source := index.Sources[method.Path]
	methods, known := parsed[method.Path]
	if !known {
		var language *Language
		for i := range index.Languages {
			if index.Languages[i].ID == method.Language {
				language = &index.Languages[i]
				break
			}
		}
		methods = language.ParseMethods(source, method.Path)
		parsed[method.Path] = methods
	}
	body := methods[method.Ordinal].Body

	keptSet := make(map[int]bool, len(kept))
	for _, token := range kept {
		keptSet[token] = true
	}
	queryLikeSet := make(map[int]bool, len(queryLike))
	for _, token := range queryLike {
		queryLikeSet[token] = true
	}

	// Per line: whether it has a kept token, and whether all of them are
	// query-like.
	roles := map[int]RecommendedLineRole{}
	var nonKeyword int
	var visit func(Node) bool
	visit = func(node Node) bool {
		if token, isToken := node.(Token); isToken {
			if token.Keyword {
				return false
			}
			own := nonKeyword
			nonKeyword++
			if !keptSet[own] {
				return false
			}
			for line := token.Line; line <= token.EndLine; line++ {
				if role, known := roles[line]; !known || role != RoleAdded {
					if queryLikeSet[own] {
						roles[line] = RoleQuery
					} else {
						roles[line] = RoleAdded
					}
				}
			}
			return true
		}
		any := false
		tree := node.(*Tree)
		for _, child := range tree.Children {
			any = visit(child) || any
		}
		if any {
			for _, child := range tree.Children {
				if token, isToken := child.(Token); isToken && token.Keyword {
					if _, known := roles[token.Line]; !known {
						roles[token.Line] = RoleStructure
					}
				}
			}
		}
		return any
	}
	visit(body)

	lines := strings.Split(source, "\n")
	out := make([]RecommendedLine, 0, len(roles))
	ordered := make([]int, 0, len(roles))
	for line := range roles {
		ordered = append(ordered, line)
	}
	sort.Ints(ordered)
	for _, line := range ordered {
		text := ""
		if line-1 < len(lines) {
			text = strings.TrimRight(lines[line-1], " \t")
		}
		out = append(out, RecommendedLine{Line: line, Text: text, Role: roles[line]})
	}
	return out
}

func dedentLines(lines []RecommendedLine) []string {
	common := -1
	for _, line := range lines {
		if strings.TrimSpace(line.Text) == "" {
			continue
		}
		indent := len(line.Text) - len(strings.TrimLeft(line.Text, " \t"))
		if common < 0 || indent < common {
			common = indent
		}
	}
	if common < 0 {
		common = 0
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line.Text) >= common {
			out[i] = line.Text[common:]
		} else {
			out[i] = line.Text
		}
	}
	return out
}
