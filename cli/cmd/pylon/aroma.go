// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Command aroma: structural code similarity search over a code corpus,
// wired into the pylon CLI. Corpus walking, flag parsing, queries from
// lines, snippets, or diffs, and human or JSON output.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"

	"github.com/estebanforge/pylons/cli/internal/aroma"
)

// SchemaVersion versions the JSON output; it is the only coupling point for a
// later bridge.
const SchemaVersion = 1

const aromaUsage = `pylon aroma: structural code similarity search over a code corpus

Given changed lines, a snippet, or a diff, find the methods of the corpus
whose code resembles the query, and the code those methods share.
Deterministic, no model calls; the output is advisory and never gates.

Usage:
  pylon aroma search    --corpus <dir> --lines <path>:120-160[,180-190] [flags]
  pylon aroma search    --corpus <dir> --snippet <file|-> [flags]
  pylon aroma recommend --corpus <dir> --lines <path>:120-160 [flags]
  pylon aroma recommend --corpus <dir> --diff <file|-> [flags]

Flags:
  --corpus <dir>       corpus root; repeatable; default is the ledger's registered projects, else "."
  --lang <id>          language front end (go); default infers from extensions
  --min <float>        results must be more similar than this; search 0.71, recommend 0.65
  --limit <n>          most results (search 100) or recommendations (5)
  --min-support <n>    fewest methods behind a recommendation; 2, or 1 for the paper's behavior
  --min-query-size <n> drop queries with fewer features than this; default 0
  --include-tests      keep _test.go files in the corpus
  --json               JSON output (schema_version 1)

Exit codes: 0 ran (even with no results), 2 usage or unreadable corpus,
1 the corpus has no files of a known language.
`

// cmdAroma dispatches `pylon aroma` with os std streams.
func cmdAroma(args []string) int {
	return runAroma(args, os.Stdin, os.Stdout, os.Stderr)
}

// runAroma runs one aroma command. It returns the process exit code.
func runAroma(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, aromaUsage)
		return 2
	}
	command := args[0]
	if command == "-h" || command == "--help" || command == "help" {
		fmt.Fprint(stdout, aromaUsage)
		return 0
	}
	if command != "search" && command != "recommend" {
		fmt.Fprintf(stderr, "pylon aroma: unknown command %q\n\n%s", command, aromaUsage)
		return 2
	}
	options, err := parseFlags(command, args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "pylon aroma: %v\n\n%s", err, aromaUsage)
		return 2
	}

	languages := []aroma.Language{aroma.GoLanguage}
	var files []aroma.SourceFile
	options.corpus = defaultCorpus(options.corpus)
	for _, root := range options.corpus {
		collected, err := walk(root, options.includeTests)
		if err != nil {
			fmt.Fprintf(stderr, "pylon aroma: cannot read corpus %q: %v\n", root, err)
			return 2
		}
		files = append(files, collected...)
	}
	index := aroma.BuildIndex(files, aroma.IndexOptions{Languages: languages})
	if index.Stats.Files == 0 {
		fmt.Fprintln(stderr, "pylon aroma: the corpus has no files of a known language")
		return 1
	}

	queries, err := options.queries(index, stdin)
	if err != nil {
		if errors.Is(err, errNoTokens) {
			// A query with no tokens is not a failure: report it and finish.
			fmt.Fprintln(stderr, "pylon aroma: the query has no names or literals; nothing to search")
			queries = nil
		} else {
			fmt.Fprintf(stderr, "pylon aroma: %v\n", err)
			return 2
		}
	}
	stats := fmt.Sprintf("indexed %d files, %d methods (%d duplicates), %d features, %d skips",
		index.Stats.Files, index.Stats.Methods, index.Stats.DuplicateMethods, index.Stats.Features,
		index.Stats.UnsupportedFiles+index.Stats.OversizedFiles+index.Stats.SkippedFiles)
	fmt.Fprintln(stderr, stats)

	if options.json {
		return writeJSON(stdout, command, index, queries, options)
	}
	writeHuman(stdout, command, index, queries, options)
	return 0
}

type runOptions struct {
	corpus       []string
	lang         string
	lines        map[string][]int
	snippet      string
	diff         string
	min          float64
	limit        int
	minSupport   int
	minQuerySize int
	includeTests bool
	json         bool
}

func parseFlags(command string, args []string) (runOptions, error) {
	options := runOptions{lines: map[string][]int{}}
	var err error
	if command == "search" {
		options.min = 0.71
		options.limit = 100
	} else {
		options.min = 0.65
		options.limit = 5
		options.minSupport = 2
	}
	defaults := options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := func(name string) (string, error) {
			i++
			if i >= len(args) {
				return "", fmt.Errorf("flag %s needs a value", name)
			}
			return args[i], nil
		}
		switch {
		case arg == "--corpus":
			var root string
			root, err = value("--corpus")
			if err != nil {
				return options, err
			}
			options.corpus = append(options.corpus, root)
		case arg == "--lang":
			var err error
			options.lang, err = value("--lang")
			if err != nil {
				return options, err
			}
		case arg == "--lines":
			var spec string
			spec, err = value("--lines")
			if err != nil {
				return options, err
			}
			path, ranges, err := parseLinesSpec(spec)
			if err != nil {
				return options, err
			}
			options.lines[path] = append(options.lines[path], ranges...)
		case arg == "--snippet":
			options.snippet, err = value("--snippet")
			if err != nil {
				return options, err
			}
		case arg == "--diff":
			options.diff, err = value("--diff")
			if err != nil {
				return options, err
			}
		case arg == "--min":
			var raw string
			raw, err = value("--min")
			if err != nil {
				return options, err
			}
			options.min, err = strconv.ParseFloat(raw, 64)
			if err != nil {
				return options, fmt.Errorf("--min needs a number, got %q", raw)
			}
		case arg == "--limit":
			var raw string
			raw, err = value("--limit")
			if err != nil {
				return options, err
			}
			options.limit, err = strconv.Atoi(raw)
			if err != nil {
				return options, fmt.Errorf("--limit needs a number, got %q", raw)
			}
		case arg == "--min-support":
			var raw string
			raw, err = value("--min-support")
			if err != nil {
				return options, err
			}
			options.minSupport, err = strconv.Atoi(raw)
			if err != nil {
				return options, fmt.Errorf("--min-support needs a number, got %q", raw)
			}
		case arg == "--min-query-size":
			var raw string
			raw, err = value("--min-query-size")
			if err != nil {
				return options, err
			}
			options.minQuerySize, err = strconv.Atoi(raw)
			if err != nil {
				return options, fmt.Errorf("--min-query-size needs a number, got %q", raw)
			}
		case arg == "--include-tests":
			options.includeTests = true
		case arg == "--json":
			options.json = true
		default:
			return options, fmt.Errorf("unknown flag %q", arg)
		}
	}
	// A flag with an empty value would be indistinguishable from unset, so
	// zero-valued numbers keep their command defaults.
	if options.min == 0 {
		options.min = defaults.min
	}
	if options.limit == 0 {
		options.limit = defaults.limit
	}
	if options.minSupport == 0 && command == "recommend" {
		options.minSupport = defaults.minSupport
	}
	if len(options.lines) == 0 && options.snippet == "" && options.diff == "" {
		return options, fmt.Errorf("one of --lines, --snippet, or --diff is required")
	}
	return options, nil
}

// parseLinesSpec splits `<path>:a-b,c-d` into a path and 1-based lines.
func parseLinesSpec(spec string) (string, []int, error) {
	colon := strings.LastIndex(spec, ":")
	if colon < 0 {
		return "", nil, fmt.Errorf("--lines needs <path>:<ranges>, got %q", spec)
	}
	path, rangesSpec := spec[:colon], spec[colon+1:]
	var lines []int
	for _, part := range strings.Split(rangesSpec, ",") {
		bounds := strings.SplitN(part, "-", 2)
		if len(bounds) != 2 {
			return "", nil, fmt.Errorf("--lines range %q is not start-end", part)
		}
		start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil {
			return "", nil, fmt.Errorf("--lines start %q is not a number", bounds[0])
		}
		end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
		if err != nil {
			return "", nil, fmt.Errorf("--lines end %q is not a number", bounds[1])
		}
		for line := start; line <= end; line++ {
			lines = append(lines, line)
		}
	}
	return path, lines, nil
}

// skipDirs keeps walk out of dependency and VCS trees.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true, "testdata": true,
	"dist": true, "target": true, "__pycache__": true,
	".venv": true, "venv": true,
}

// walk collects the corpus files under root, root-relative and
// slash-normalized, in sorted order.
func walk(root string, includeTests bool) ([]aroma.SourceFile, error) {
	var files []aroma.SourceFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "._") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() > 1_000_000 {
			return nil
		}
		if isGenerated(path) {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, aroma.SourceFile{Path: filepath.ToSlash(rel), Source: string(source)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(a, b aroma.SourceFile) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// isGenerated reports a Go file whose header says it was generated.
func isGenerated(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	chunk := make([]byte, 1024)
	read, _ := file.Read(chunk)
	for _, line := range strings.Split(string(chunk[:read]), "\n") {
		if strings.Contains(line, "Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, " *") {
			continue
		}
		break
	}
	return false
}

// queries builds the run's queries from the requested sources.
func (options runOptions) queries(index *aroma.Index, stdin io.Reader) ([]aroma.Query, error) {
	var queries []aroma.Query
	add := func(query aroma.Query, ok bool) {
		if ok && query.Size >= options.minQuerySize {
			queries = append(queries, query)
		}
	}
	if len(options.lines) > 0 {
		paths := make([]string, 0, len(options.lines))
		for path := range options.lines {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			file, err := readCorpusFile(path)
			if err != nil {
				return nil, err
			}
			queries = append(queries, aroma.QueriesFromLines(index, *file, options.lines[path])...)
		}
	}
	if options.snippet != "" {
		source, path, err := readInput(options.snippet, stdin)
		if err != nil {
			return nil, err
		}
		query, ok := aroma.QueryFromSnippet(index, source, aroma.SnippetOptions{Path: path}, options.lang)
		add(query, ok)
	}
	if options.diff != "" {
		source, _, err := readInput(options.diff, stdin)
		if err != nil {
			return nil, err
		}
		for _, change := range diffLines(source) {
			file, err := readCorpusFile(change.path)
			if err != nil {
				continue
			}
			queries = append(queries, aroma.QueriesFromLines(index, *file, change.lines)...)
		}
	}
	if len(queries) == 0 {
		return nil, errNoTokens
	}
	return queries, nil
}

// errNoTokens says the requested sources produced no query tokens. It is not
// a failure: the run finishes with an empty result set.
var errNoTokens = errors.New("the query has no names or literals to search for")

// readCorpusFile reads a query file, normalizing its path the way walk does:
// root-relative and slash-separated, resolved against the current directory.
func readCorpusFile(path string) (*aroma.SourceFile, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %v", path, err)
	}
	normalized := filepath.ToSlash(path)
	normalized = strings.TrimPrefix(normalized, "./")
	return &aroma.SourceFile{Path: normalized, Source: string(source)}, nil
}

func readInput(spec string, stdin io.Reader) (string, string, error) {
	if spec == "-" {
		source, err := io.ReadAll(stdin)
		return string(source), "", err
	}
	source, err := os.ReadFile(spec)
	return string(source), spec, err
}

type diffChange struct {
	path  string
	lines []int
}

// diffLines extracts the added lines of every file in a unified diff.
func diffLines(diff string) []diffChange {
	var changes []diffChange
	var current *diffChange
	newLine := 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff "):
			if current != nil {
				changes = append(changes, *current)
				current = nil
			}
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(line[4:], "b/")
			current = &diffChange{path: path}
		case strings.HasPrefix(line, "@@"):
			newLine = hunkStart(line, '+')
		case strings.HasPrefix(line, "+") && current != nil:
			current.lines = append(current.lines, newLine)
			newLine++
		case strings.HasPrefix(line, "-"):
			// A removed line is not on the new side.
		default:
			if current != nil && line != "\\ No newline at end of file" {
				newLine++
			}
		}
	}
	if current != nil {
		changes = append(changes, *current)
	}
	return changes
}

// hunkStart reads the new-file start line from a hunk header.
func hunkStart(header string, side byte) int {
	after := strings.LastIndex(header, " "+string(side))
	if after < 0 {
		return 1
	}
	rest := header[after+2:]
	end := strings.IndexAny(rest, ", ")
	if end < 0 {
		end = len(rest)
	}
	start, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 1
	}
	return start
}

// JSON output

type jsonQuery struct {
	Query struct {
		Size      int              `json:"size"`
		LowSignal bool             `json:"lowSignal"`
		Language  string           `json:"language"`
		Lines     []int            `json:"lines,omitempty"`
		Origin    *aroma.MethodRef `json:"origin,omitempty"`
	} `json:"query"`
	Results         []aroma.SearchResult   `json:"results,omitempty"`
	Recommendations []aroma.Recommendation `json:"recommendations,omitempty"`
}

type jsonOutput struct {
	SchemaVersion int         `json:"schema_version"`
	Command       string      `json:"command"`
	Queries       []jsonQuery `json:"queries"`
}

func writeJSON(stdout io.Writer, command string, index *aroma.Index, queries []aroma.Query, options runOptions) int {
	out := jsonOutput{SchemaVersion: SchemaVersion, Command: command, Queries: []jsonQuery{}}
	for _, query := range queries {
		entry := jsonQuery{}
		entry.Query.Size = query.Size
		entry.Query.LowSignal = query.Size < 30
		entry.Query.Language = query.Language
		entry.Query.Lines = query.Lines
		entry.Query.Origin = query.Origin
		if command == "search" {
			entry.Results = aroma.Search(index, query, aroma.SearchOptions{MinSimilarity: options.min, Limit: options.limit})
		} else {
			entry.Recommendations = aroma.Recommend(index, query, aroma.RecommendOptions{
				SearchOptions: aroma.SearchOptions{},
				MinSimilarity: options.min,
				Limit:         options.limit,
				MinSupport:    options.minSupport,
			})
		}
		out.Queries = append(out.Queries, entry)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "pylon aroma: %v\n", err)
		return 2
	}
	return 0
}

func writeHuman(stdout io.Writer, command string, index *aroma.Index, queries []aroma.Query, options runOptions) {
	for _, query := range queries {
		fmt.Fprintf(stdout, "query %d features", query.Size)
		if query.Size < 30 {
			fmt.Fprint(stdout, " (low signal)")
		}
		if query.Origin != nil {
			fmt.Fprintf(stdout, " from %s:%d-%d", query.Origin.Path, query.Origin.StartLine, query.Origin.EndLine)
		}
		fmt.Fprintln(stdout)
		if command == "search" {
			results := aroma.Search(index, query, aroma.SearchOptions{MinSimilarity: options.min, Limit: options.limit})
			if len(results) == 0 {
				fmt.Fprintln(stdout, "  no results above the threshold")
				continue
			}
			for _, result := range results {
				fmt.Fprintf(stdout, "  %s:%s:%d-%d  %.2f  lines %s\n",
					result.Method.Path, result.Method.Name,
					result.Method.StartLine, result.Method.EndLine,
					result.Similarity, formatLines(result.MatchedLines))
			}
			continue
		}
		recommendations := aroma.Recommend(index, query, aroma.RecommendOptions{
			SearchOptions: aroma.SearchOptions{},
			MinSimilarity: options.min,
			Limit:         options.limit,
			MinSupport:    options.minSupport,
		})
		if len(recommendations) == 0 {
			fmt.Fprintln(stdout, "  no recommendations above the threshold")
			continue
		}
		for _, recommendation := range recommendations {
			fmt.Fprintf(stdout, "  %s:%s:%d-%d  %.2f  shared by %d methods\n",
				recommendation.Method.Path, recommendation.Method.Name,
				recommendation.Method.StartLine, recommendation.Method.EndLine,
				recommendation.Similarity, len(recommendation.Members))
			printLines(stdout, recommendation.Lines)
		}
	}
}

func formatLines(lines []int) string {
	if len(lines) == 0 {
		return "-"
	}
	var parts []string
	start := lines[0]
	previous := lines[0]
	for _, line := range lines[1:] {
		if line == previous+1 {
			previous = line
			continue
		}
		parts = append(parts, formatRange(start, previous))
		start, previous = line, line
	}
	parts = append(parts, formatRange(start, previous))
	return strings.Join(parts, ",")
}

func formatRange(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

func printLines(stdout io.Writer, lines []aroma.RecommendedLine) {
	previous := 0
	for _, line := range lines {
		if previous > 0 && line.Line > previous+1 {
			fmt.Fprintln(stdout, "    ...")
		}
		previous = line.Line
		marker := "  "
		if line.Role == aroma.RoleAdded {
			marker = "+ "
		}
		fmt.Fprintf(stdout, "    %s%s\n", marker, line.Text)
	}
}

// defaultCorpus resolves the corpus roots: the registered projects of the
// nearest ledger when there is one, else the current directory.
func defaultCorpus(roots []string) []string {
	if len(roots) > 0 {
		return roots
	}
	if kbRoot := findPylonsRoot("."); kbRoot != "" {
		if cfg, err := config.Load(kbRoot); err == nil && len(cfg.Projects) > 0 {
			paths := make([]string, 0, len(cfg.Projects))
			for alias := range cfg.Projects {
				paths = append(paths, cfg.ProjectPath(kbRoot, alias))
			}
			slices.Sort(paths)
			return paths
		}
	}
	return []string{"."}
}
