// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Doc is one document to index.
type BM25Doc struct {
	ID   string
	Text string
}

// BM25Result is one ranked hit.
type BM25Result struct {
	ID    string
	Score float64
}

// BM25Index is an in-memory Okapi BM25 term-frequency table.
type BM25Index struct {
	docIDs    []string
	docTerms  []map[string]int
	docLen    []int
	df        map[string]int
	n         int
	avgDocLen float64
}

// NewBM25Index builds a BM25 index over docs.
func NewBM25Index(docs []BM25Doc) *BM25Index {
	idx := &BM25Index{
		df: make(map[string]int),
	}
	totalLen := 0
	for _, d := range docs {
		terms := tokenize(d.Text)
		freq := make(map[string]int, len(terms))
		for _, t := range terms {
			freq[t]++
		}
		idx.docIDs = append(idx.docIDs, d.ID)
		idx.docTerms = append(idx.docTerms, freq)
		idx.docLen = append(idx.docLen, len(terms))
		totalLen += len(terms)
		for t := range freq {
			idx.df[t]++
		}
	}
	idx.n = len(docs)
	if idx.n > 0 {
		idx.avgDocLen = float64(totalLen) / float64(idx.n)
	}
	return idx
}

// Search scores docs against query and returns the top limit hits.
func (idx *BM25Index) Search(query string, limit int) []BM25Result {
	terms := tokenize(query)
	if len(terms) == 0 || idx.n == 0 || idx.avgDocLen == 0 {
		return nil
	}

	idf := make(map[string]float64, len(terms))
	unique := make([]string, 0, len(terms))
	for _, t := range terms {
		if _, ok := idf[t]; ok {
			continue
		}
		df := idx.df[t]
		idf[t] = math.Log(1.0 + (float64(idx.n)-float64(df)+0.5)/(float64(df)+0.5))
		unique = append(unique, t)
	}

	var results []BM25Result
	for i, tfMap := range idx.docTerms {
		score := 0.0
		dl := float64(idx.docLen[i])
		for _, t := range unique {
			tf := float64(tfMap[t])
			if tf == 0 {
				continue
			}
			num := tf * (bm25K1 + 1.0)
			den := tf + bm25K1*(1.0-bm25B+bm25B*(dl/idx.avgDocLen))
			score += idf[t] * (num / den)
		}
		if score > 0 {
			results = append(results, BM25Result{ID: idx.docIDs[i], Score: score})
		}
	}

	slices.SortFunc(results, func(a, b BM25Result) int {
		if a.Score != b.Score {
			return cmp.Compare(b.Score, a.Score)
		}
		return strings.Compare(a.ID, b.ID)
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
