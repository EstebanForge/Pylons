// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This is the paper's recall micro-benchmark (Section 5), scaled to this
// repository: queries are 5 contiguous and 5 random lines taken from Go
// methods of at least 12 lines, built in snippet mode, and the original
// method must come back in the reranked results. The gate is looser than the
// paper's 99.1%/98.3% Recall@1 because the corpus here is small and the Go
// front end is not the paper's parser; the first run's results calibrate it.
func TestRecallBenchmark(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []aroma.SourceFile
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (name == ".git" || name == "testdata" || name == "dist" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, aroma.SourceFile{Path: filepath.ToSlash(rel), Source: string(source)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("corpus too small: %d files", len(files))
	}
	index := aroma.BuildIndex(files, aroma.IndexOptions{Languages: []aroma.Language{aroma.GoLanguage}})
	if index.Stats.Methods < 20 {
		t.Fatalf("too few methods: %d", index.Stats.Methods)
	}

	type benchQuery struct {
		file  aroma.SourceFile
		name  string
		path  string
		lines []int
	}
	var contiguous, noncontiguous []benchQuery
	random := rand.New(rand.NewSource(20181201))
	for _, file := range files {
		for _, method := range aroma.GoLanguage.ParseMethods(file.Source, file.Path) {
			if method.EndLine-method.StartLine+1 < 12 {
				continue
			}
			// 5 contiguous lines: the first 5 of the method's body region.
			start := method.StartLine + 1
			var lines []int
			for line := start; line < start+5 && line <= method.EndLine; line++ {
				lines = append(lines, line)
			}
			if len(lines) == 5 {
				contiguous = append(contiguous, benchQuery{file: file, name: method.Name, path: file.Path, lines: lines})
			}
			// 5 random distinct lines.
			pool := map[int]bool{}
			for line := method.StartLine + 1; line <= method.EndLine; line++ {
				pool[line] = true
			}
			picked := map[int]bool{}
			var randomLines []int
			for len(randomLines) < 5 && len(picked) < len(pool) {
				candidate := method.StartLine + 1 + random.Intn(method.EndLine-method.StartLine)
				if picked[candidate] {
					continue
				}
				picked[candidate] = true
				randomLines = append(randomLines, candidate)
			}
			if len(randomLines) == 5 {
				noncontiguous = append(noncontiguous, benchQuery{file: file, name: method.Name, path: file.Path, lines: randomLines})
			}
		}
	}

	recall := func(bench []benchQuery, top int) float64 {
		if len(bench) == 0 {
			return 1
		}
		hits := 0
		for _, item := range bench {
			queries := aroma.QueriesFromLines(index, item.file, item.lines)
			found := false
			for _, query := range queries {
				results := aroma.Search(index, query, aroma.SearchOptions{IncludeOrigin: true, Limit: top})
				for _, result := range results {
					if result.Method.Name == item.name && result.Method.Path == item.path {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				hits++
			}
		}
		return float64(hits) / float64(len(bench))
	}

	if got := recall(contiguous, 1); got < 0.90 {
		t.Errorf("contiguous Recall@1 = %.3f, want at least 0.90", got)
	}
	if got := recall(contiguous, 100); got < 0.99 {
		t.Errorf("contiguous Recall@100 = %.3f, want at least 0.99", got)
	}
	if got := recall(noncontiguous, 1); got < 0.85 {
		t.Errorf("non-contiguous Recall@1 = %.3f, want at least 0.85", got)
	}
	if got := recall(noncontiguous, 100); got < 0.99 {
		t.Errorf("non-contiguous Recall@100 = %.3f, want at least 0.99", got)
	}
}
