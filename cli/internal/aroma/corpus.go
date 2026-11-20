// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

import (
	"crypto/sha256"
	"encoding/base64"
	"sort"
)

// SourceFile is one file of a corpus.
type SourceFile struct {
	Path   string
	Source string
}

// MethodRef is where a method is, for reports.
type MethodRef struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// MethodLocation is one place a method is written, with what showing it there
// needs.
type MethodLocation struct {
	MethodRef
	// TokenLines and TokenEndLines give, per non-keyword token, the 1-based
	// lines it starts and ends on.
	TokenLines    []uint32
	TokenEndLines []uint32
	// Ordinal is the method's position among the methods its language parses
	// from the file.
	Ordinal int
}

// IndexedMethod is a method of the corpus with the features of each of its
// non-keyword tokens.
type IndexedMethod struct {
	MethodLocation
	TokenTable
	// Copies are where later methods with the same tokens, local variables,
	// and nesting are. A search that leaves this method out answers with one
	// of them.
	Copies []MethodLocation
}

// IndexStats counts what an index build saw.
type IndexStats struct {
	Files            int      `json:"files"`
	Methods          int      `json:"methods"`
	DuplicateMethods int      `json:"duplicateMethods"`
	Features         int      `json:"features"`
	UnsupportedFiles int      `json:"unsupportedFiles"`
	OversizedFiles   int      `json:"oversizedFiles"`
	SkippedFiles     int      `json:"skippedFiles"`
	Errors           []string `json:"errors,omitempty"`
}

// Index is a code corpus featurized for search.
type Index struct {
	Methods   []*IndexedMethod
	Stats     IndexStats
	Languages []Language
	// FeatureIDs gives each distinct feature string its id.
	FeatureIDs map[string]int
	// PostingOffsets and Postings store the paper's method-by-feature matrix
	// by feature: the methods that have feature f at least once are
	// Postings[PostingOffsets[f]] up to, not including, Postings[PostingOffsets[f+1]].
	PostingOffsets []int32
	Postings       []int32
	// Sources holds the source of each file that has an indexed method, for
	// showing recommendations.
	Sources map[string]string
}

// IndexOptions tune an index build.
type IndexOptions struct {
	Languages []Language
	// MaxFileLength skips files with more bytes, which keeps bundles and
	// data files out.
	MaxFileLength int
	// Skip decides per file whether to leave it out of the corpus (for a
	// test fixture's path filters). Its result is counted in SkippedFiles.
	Skip func(SourceFile) bool
}

const maxFileLength = 1_000_000

// BuildIndex featurizes every method of the given files (the indexing stage
// of Section 3). Files of unknown languages are skipped. A method that
// repeats an earlier method's tokens, local variables, and nesting is
// recorded as a copy of it and not indexed again, so that a recommendation
// always comes from different pieces of code (Section 4.1).
func BuildIndex(files []SourceFile, options IndexOptions) *Index {
	languages := options.Languages
	maxLength := options.MaxFileLength
	if maxLength == 0 {
		maxLength = maxFileLength
	}
	featureIDs := make(map[string]int)
	var methods []*IndexedMethod
	sources := make(map[string]string)
	seen := make(map[string]*IndexedMethod)
	stats := IndexStats{}

	for _, file := range files {
		language, known := LanguageForPath(languages, file.Path)
		if !known {
			stats.UnsupportedFiles++
			continue
		}
		if options.Skip != nil && options.Skip(file) {
			stats.SkippedFiles++
			continue
		}
		if len(file.Source) > maxLength {
			stats.OversizedFiles++
			continue
		}
		stats.Files++
		for ordinal, method := range language.ParseMethods(file.Source, file.Path) {
			tokens := Featurize(method.Body, FeaturizeOptions{MemberAccessLabels: language.MemberAccessLabels})
			if len(tokens) == 0 {
				continue
			}
			sources[file.Path] = file.Source
			location := MethodLocation{
				MethodRef: MethodRef{
					Path: file.Path, Name: method.Name, Language: language.ID,
					StartLine: method.StartLine, EndLine: method.EndLine,
				},
				TokenLines:    tokenLines(tokens, func(t TokenFeatures) uint32 { return uint32(t.Token.Line) }),
				TokenEndLines: tokenLines(tokens, func(t TokenFeatures) uint32 { return uint32(t.Token.EndLine) }),
				Ordinal:       ordinal,
			}
			key := language.ID + "\n" + digest(method.Body)
			if earlier, duplicate := seen[key]; duplicate {
				earlier.Copies = append(earlier.Copies, location)
				stats.DuplicateMethods++
				continue
			}
			var features []int32
			offsets := make([]uint32, len(tokens)+1)
			for position, entry := range tokens {
				for _, feature := range entry.Features {
					id, known := featureIDs[feature]
					if !known {
						id = len(featureIDs)
						featureIDs[feature] = id
					}
					features = append(features, int32(id))
				}
				offsets[position+1] = uint32(len(features))
			}
			indexed := &IndexedMethod{
				MethodLocation: location,
				TokenTable:     TokenTable{Features: features, Offsets: offsets},
			}
			seen[key] = indexed
			methods = append(methods, indexed)
		}
	}
	stats.Methods = len(methods)
	stats.Features = len(featureIDs)
	postingOffsets, postings := invert(methods, len(featureIDs))
	return &Index{
		Methods: methods, Stats: stats, Languages: languages,
		FeatureIDs: featureIDs, Sources: sources,
		PostingOffsets: postingOffsets, Postings: postings,
	}
}

func tokenLines(tokens []TokenFeatures, pick func(TokenFeatures) uint32) []uint32 {
	out := make([]uint32, len(tokens))
	for i, entry := range tokens {
		out[i] = pick(entry)
	}
	return out
}

func invert(methods []*IndexedMethod, featureCount int) ([]int32, []int32) {
	lastMethod := make([]int32, featureCount)
	for i := range lastMethod {
		lastMethod[i] = -1
	}
	postingOffsets := make([]int32, featureCount+1)
	for id, method := range methods {
		for _, feature := range method.Features {
			if lastMethod[feature] != int32(id) {
				lastMethod[feature] = int32(id)
				postingOffsets[feature+1]++
			}
		}
	}
	for feature := 0; feature < featureCount; feature++ {
		postingOffsets[feature+1] += postingOffsets[feature]
	}
	postings := make([]int32, postingOffsets[featureCount])
	next := make([]int32, featureCount)
	copy(next, postingOffsets[:featureCount])
	for i := range lastMethod {
		lastMethod[i] = -1
	}
	for id, method := range methods {
		for _, feature := range method.Features {
			if lastMethod[feature] != int32(id) {
				lastMethod[feature] = int32(id)
				postings[next[feature]] = int32(id)
				next[feature]++
			}
		}
	}
	return postingOffsets, postings
}

// digest hashes everything a method's features are computed from: its tokens,
// which of them are keywords and local variables, and how they nest.
func digest(body Node) string {
	h := sha256.New()
	var visit func(Node)
	visit = func(node Node) {
		if token, ok := node.(Token); ok {
			kind := "n"
			if token.Keyword {
				kind = "k"
			} else if token.Local {
				kind = "l"
			}
			h.Write([]byte(kind))
			h.Write([]byte(token.Text))
			h.Write([]byte{0})
			return
		}
		h.Write([]byte("("))
		for _, child := range node.(*Tree).Children {
			visit(child)
		}
		h.Write([]byte(")"))
	}
	visit(body)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Query is a code snippet to search for, featurized against one index.
type Query struct {
	Language string
	// Features are the query's features that some method of the index has.
	Features Multiset
	// Size is how many features the query has, counting repeats and ones the
	// index has never seen.
	Size int
	// Origin is the method a query was cut from, which searches leave out.
	Origin *MethodRef
	// Lines are the lines of Origin the query covers.
	Lines []int
}

func toQuery(index *Index, language string, tokens []TokenFeatures) Query {
	features := Multiset{}
	size := 0
	for _, entry := range tokens {
		for _, feature := range entry.Features {
			size++
			if id, known := index.FeatureIDs[feature]; known {
				features[id]++
			}
		}
	}
	return Query{Language: language, Features: features, Size: size}
}

// QueryFromSnippet builds a query from a piece of code on its own, as the
// paper takes one. The snippet need not be a whole statement or method. It
// reports false when the language is unknown to the index or the snippet has
// no names or literals.
func QueryFromSnippet(index *Index, source string, options SnippetOptions, languageID string) (Query, bool) {
	var language *Language
	for i := range index.Languages {
		if index.Languages[i].ID == languageID {
			language = &index.Languages[i]
			break
		}
	}
	if language == nil && options.Path != "" {
		if found, known := LanguageForPath(index.Languages, options.Path); known {
			language = &found
		}
	}
	if language == nil {
		return Query{}, false
	}
	tree, ok := language.ParseSnippet(source, options)
	if !ok || tree == nil {
		return Query{}, false
	}
	tokens := Featurize(tree, FeaturizeOptions{MemberAccessLabels: language.MemberAccessLabels})
	if len(tokens) == 0 {
		return Query{}, false
	}
	return toQuery(index, language.ID, tokens), true
}

// QueriesFromLines builds one query per method of a file that has code on the
// given lines: the snippet made of the tokens on those lines, which keep the
// ancestors and the local variables they have in their method. This is how to
// ask about changed lines of a file in the checkout the index was built from.
// Lines outside every method produce no query.
func QueriesFromLines(index *Index, file SourceFile, lines []int) []Query {
	language, known := LanguageForPath(index.Languages, file.Path)
	if !known {
		return nil
	}
	wanted := make(map[int]bool, len(lines))
	for _, line := range lines {
		wanted[line] = true
	}
	var queries []Query
	for _, method := range language.ParseMethods(file.Source, file.Path) {
		covered := map[int]bool{}
		var coveredList []int
		tokens := Featurize(method.Body, FeaturizeOptions{
			MemberAccessLabels: language.MemberAccessLabels,
			Select: func(token Token) bool {
				selected := false
				for line := token.Line; line <= token.EndLine; line++ {
					if wanted[line] {
						if !covered[line] {
							covered[line] = true
							coveredList = append(coveredList, line)
						}
						selected = true
					}
				}
				return selected
			},
		})
		if len(tokens) > 0 {
			query := toQuery(index, language.ID, tokens)
			sort.Ints(coveredList)
			query.Origin = &MethodRef{
				Path: file.Path, Name: method.Name, Language: language.ID,
				StartLine: method.StartLine, EndLine: method.EndLine,
			}
			query.Lines = coveredList
			queries = append(queries, query)
		}
	}
	return queries
}
