// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

import "sort"

// Multiset is a multiset of feature ids: how many times each id occurs.
type Multiset map[int]int

// TokenTable holds the features of a snippet's non-keyword tokens: token i
// has features[Offsets[i]] up to, not including, features[Offsets[i+1]].
type TokenTable struct {
	Features []int32
	Offsets  []uint32
}

// Pruned is the result of pruning: the tokens kept, ascending, and SimScore,
// the size of the multiset intersection of the target and the kept tokens'
// features.
type Pruned struct {
	Tokens []int
	Score  int
}

// Prune is Algorithm 1 of the paper: the tokens of a table that it has in
// common with target, chosen greedily. Each step adds the token that raises
// the similarity to the target most, the earliest one on a tie, and it stops
// when no token raises it. A token's gain never grows as tokens are added, so
// the search rechecks tokens from the highest earlier gain down instead of
// rescanning them all. Candidates limits the tokens to choose from; nil means
// all of them.
func Prune(target Multiset, table TokenTable, candidates []int) Pruned {
	used := make(map[int32]int)
	gain := func(token int) int {
		start := table.Offsets[token]
		end := table.Offsets[token+1]
		total := 0
		for at := start; at < end; at++ {
			feature := table.Features[at]
			capacity, known := target[int(feature)]
			if !known {
				continue
			}
			taken := used[feature]
			for earlier := start; earlier < at; earlier++ {
				if table.Features[earlier] == feature {
					taken++
				}
			}
			if taken < capacity {
				total++
			}
		}
		return total
	}

	buckets := make(map[int][]int)
	add := func(token int, value int) {
		if value > 0 {
			buckets[value] = append(buckets[value], token)
		}
	}
	if candidates == nil {
		for token := 0; token < len(table.Offsets)-1; token++ {
			add(token, gain(token))
		}
	} else {
		for _, token := range candidates {
			add(token, gain(token))
		}
	}

	maxGain := 0
	for value := range buckets {
		if value > maxGain {
			maxGain = value
		}
	}
	var tokens []int
	score := 0
	for value := maxGain; value > 0; value-- {
		bucket, known := buckets[value]
		if !known {
			continue
		}
		// Tokens only ever move to a lower bucket, so this one is complete.
		sort.Ints(bucket)
		for _, token := range bucket {
			current := gain(token)
			if current < value {
				add(token, current)
				continue
			}
			tokens = append(tokens, token)
			score += value
			for at := table.Offsets[token]; at < table.Offsets[token+1]; at++ {
				feature := table.Features[at]
				if _, has := target[int(feature)]; has {
					used[feature]++
				}
			}
		}
	}
	sort.Ints(tokens)
	return Pruned{Tokens: tokens, Score: score}
}
