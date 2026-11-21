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
	"maps"
	"os"
	"path/filepath"
	"regexp"
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
  --lang <id>          language front end (go); needed for stdin snippets
  --min <float>        results must be more similar than this; search 0.71, recommend 0.65
  --limit <n>          most results (search 100) or recommendations (5)
  --min-support <n>    fewest methods behind a recommendation; 2, or 1 for the paper's behavior
  --min-query-size <n> drop queries with fewer features than this; default 0
  --include-tests      keep _test.go files in the corpus
  --json               JSON output (schema_version 1)

Exit codes: 0 ran (even with no results). 2 usage error, unreadable corpus,
or a corpus with no files of a known language. Exit 1 is never used, so the
command can never read as a blocking verdict.
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
	if options.help {
		fmt.Fprint(stdout, aromaUsage)
		return 0
	}

	languages := []aroma.Language{aroma.GoLanguage}
	roots, err := resolveCorpus(options.corpus, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "pylon aroma: %v\n", err)
		return 2
	}
	var files []aroma.SourceFile
	for _, root := range roots {
		collected := walk(root, roots, languages, options.includeTests, stderr)
		files = append(files, collected...)
	}
	index := aroma.BuildIndex(files, aroma.IndexOptions{Languages: languages})
	if index.Stats.Files == 0 {
		fmt.Fprintln(stderr, "pylon aroma: the corpus has no files of a known language")
		return 2
	}

	queries, err := options.queries(index, stdin, roots, languages, stderr)
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
		return writeJSON(stdout, stderr, command, index, queries, options)
	}
	writeHuman(stdout, command, index, queries, options)
	return 0
}

type runOptions struct {
	help         bool
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
			if options.limit <= 0 {
				return options, fmt.Errorf("--limit needs a positive number, got %d", options.limit)
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
			if options.minSupport < 1 {
				return options, fmt.Errorf("--min-support needs at least 1, got %d", options.minSupport)
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
		case arg == "-h" || arg == "--help":
			options.help = true
		case arg == "--json":
			options.json = true
		default:
			return options, fmt.Errorf("unknown flag %q", arg)
		}
	}
	// Defaults were set before parsing, so explicit values, including
	// zeros, survive untouched.
	if !options.help && len(options.lines) == 0 && options.snippet == "" && options.diff == "" {
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
		if end-start > 100_000 {
			return "", nil, fmt.Errorf("--lines range %q is larger than 100000 lines", part)
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
// corpusRoot is one indexed corpus directory. label prefixes every indexed
// path so several roots cannot collide on the same relative path; a single
// unlabeled root keeps bare relative paths.
type corpusRoot struct {
	label string
	path  string
	abs   string
}

// resolveCorpus turns the requested roots into labeled, symlink-resolved,
// deduplicated corpus roots. With no explicit roots it uses the ledger's
// registered projects, else the current directory.
func resolveCorpus(explicit []string, stderr io.Writer) ([]corpusRoot, error) {
	var roots []corpusRoot
	if len(explicit) > 0 {
		for _, root := range explicit {
			label := ""
			if len(explicit) > 1 {
				label = strings.TrimPrefix(filepath.Clean(root), "./")
			}
			roots = append(roots, corpusRoot{label: label, path: root})
		}
	} else if kbRoot := findPylonsRoot(""); kbRoot != "" {
		cfg, err := config.Load(kbRoot)
		if err != nil {
			fmt.Fprintf(stderr, "pylon aroma: warning: invalid manifests/pylons.yaml: %v\n", err)
		}
		for _, alias := range slices.Sorted(maps.Keys(cfg.Projects)) {
			path := cfg.ProjectPath(kbRoot, alias)
			if info, err := os.Stat(path); err != nil || !info.IsDir() {
				fmt.Fprintf(stderr, "pylon aroma: skipping project %s: %q is not a readable directory\n", alias, path)
				continue
			}
			roots = append(roots, corpusRoot{label: alias, path: path})
		}
	}
	if len(roots) == 0 {
		roots = []corpusRoot{{path: "."}}
	}
	for i := range roots {
		abs, err := filepath.Abs(roots[i].path)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve corpus %q: %v", roots[i].path, err)
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		roots[i].abs = abs
	}
	// Duplicate and nested roots would index files twice; keep the outer
	// root and drop the rest.
	kept := roots[:0:0]
	for i, root := range roots {
		outer := true
		for j, other := range roots {
			if i != j && root.abs != other.abs && strings.HasPrefix(root.abs, other.abs+string(filepath.Separator)) {
				outer = false
				break
			}
		}
		duplicate := false
		for _, other := range kept {
			if other.abs == root.abs {
				duplicate = true
				break
			}
		}
		if outer && !duplicate {
			kept = append(kept, root)
		}
	}
	return kept, nil
}

// walk collects the corpus files under root: regular files of a known
// language extension, under 1MB, not generated, with VCS and dependency
// trees skipped. Per-entry errors are reported and the walk continues.
func walk(root corpusRoot, all []corpusRoot, languages []aroma.Language, includeTests bool, stderr io.Writer) []aroma.SourceFile {
	extensions := map[string]bool{}
	for _, language := range languages {
		for _, ext := range language.Extensions {
			extensions[ext] = true
		}
	}
	var files []aroma.SourceFile
	err := filepath.WalkDir(root.abs, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if path == root.abs {
				return err
			}
			fmt.Fprintf(stderr, "pylon aroma: skipping %s: %v\n", path, err)
			return nil
		}
		if entry.IsDir() {
			if path != root.abs && (skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "._") {
			return nil
		}
		if !extensions[filepath.Ext(entry.Name())] {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Another corpus root owns this file (nested root); skip it there.
		for _, other := range all {
			if other.abs != root.abs && strings.HasPrefix(path, other.abs+string(filepath.Separator)) {
				return nil
			}
		}
		info, err := entry.Info()
		if err != nil {
			fmt.Fprintf(stderr, "pylon aroma: skipping %s: %v\n", path, err)
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
			fmt.Fprintf(stderr, "pylon aroma: skipping %s: %v\n", path, err)
			return nil
		}
		rel, err := filepath.Rel(root.abs, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if root.label != "" {
			rel = root.label + "/" + rel
		}
		files = append(files, aroma.SourceFile{Path: rel, Source: string(source)})
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "pylon aroma: cannot read corpus %q: %v\n", root.path, err)
		return nil
	}
	slices.SortFunc(files, func(a, b aroma.SourceFile) int { return strings.Compare(a.Path, b.Path) })
	return files
}

// isGenerated reports a Go file whose header says it was generated, in the
// `^// Code generated .* DO NOT EDIT\.$` convention line or its single-line
// block-comment form.
var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$|^/\* Code generated .* DO NOT EDIT\. \*/$`)

func isGenerated(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	chunk := make([]byte, 4096)
	read, _ := file.Read(chunk)
	for _, line := range strings.Split(string(chunk[:read]), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if generatedRe.MatchString(line) {
			return true
		}
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, " *") {
			continue
		}
		break
	}
	return false
}

// queries builds the run's queries from the requested sources. Every query
// file is resolved against the corpus roots, so origin exclusion and result
// paths agree with the index.
func (options runOptions) queries(index *aroma.Index, stdin io.Reader, roots []corpusRoot, languages []aroma.Language, stderr io.Writer) ([]aroma.Query, error) {
	var queries []aroma.Query
	if len(options.lines) > 0 {
		paths := make([]string, 0, len(options.lines))
		for path := range options.lines {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			source, indexed, err := readCorpusFile(path, roots)
			if err != nil {
				return nil, err
			}
			file := aroma.SourceFile{Path: indexed, Source: source}
			queries = append(queries, aroma.QueriesFromLines(index, file, options.lines[path])...)
		}
	}
	if options.snippet != "" {
		source, path, err := readInput(options.snippet, stdin)
		if err != nil {
			return nil, err
		}
		lang := options.lang
		if lang == "" {
			if len(languages) == 1 {
				lang = languages[0].ID
			} else {
				return nil, errors.New("cannot infer the snippet language; pass --lang")
			}
		}
		// A snippet read from a corpus file carries its indexed path, so
		// origin exclusion applies.
		if path != "" {
			if _, indexed, err := resolveQueryPath(path, roots); err == nil {
				path = indexed
			}
		}
		query, ok := aroma.QueryFromSnippet(index, source, aroma.SnippetOptions{Path: path}, lang)
		if ok {
			queries = append(queries, query)
		}
	}
	if options.diff != "" {
		source, _, err := readInput(options.diff, stdin)
		if err != nil {
			return nil, err
		}
		for _, change := range diffLines(source) {
			resolved, indexed, err := resolveQueryPath(change.path, roots)
			if err != nil {
				// A diff can name paths outside the corpus or deleted
				// files; skip them with a note instead of failing.
				fmt.Fprintf(stderr, "pylon aroma: skipping %s: %v\n", change.path, err)
				continue
			}
			source, err := readBounded(resolved)
			if err != nil {
				fmt.Fprintf(stderr, "pylon aroma: skipping %s: %v\n", change.path, err)
				continue
			}
			file := aroma.SourceFile{Path: indexed, Source: source}
			queries = append(queries, aroma.QueriesFromLines(index, file, change.lines)...)
		}
	}
	if options.minQuerySize > 0 {
		kept := queries[:0:0]
		for _, query := range queries {
			if query.Size >= options.minQuerySize {
				kept = append(kept, query)
			}
		}
		queries = kept
	}
	if len(queries) == 0 {
		return nil, errNoTokens
	}
	return queries, nil
}

// lowSignalQuerySize is the feature count under which a query says little:
// all its locals read as #VAR and the structural context is thin.
const lowSignalQuerySize = 30

// errNoTokens says the requested sources produced no query tokens. It is not
// a failure: the run finishes with an empty result set.
var errNoTokens = errors.New("the query has no names or literals to search for")

// readCorpusFile resolves a query path against the corpus roots and reads it
// with the same bounds the corpus walk applies.
func readCorpusFile(path string, roots []corpusRoot) (string, string, error) {
	resolved, indexed, err := resolveQueryPath(path, roots)
	if err != nil {
		return "", "", err
	}
	source, err := readBounded(resolved)
	if err != nil {
		return "", "", fmt.Errorf("cannot read %q: %v", path, err)
	}
	return source, indexed, nil
}

// resolveQueryPath maps a cwd-relative file to its indexed path: the owning
// root's label plus the root-relative path. A file under no root is an error,
// which also keeps diff paths from escaping the corpus.
func resolveQueryPath(path string, roots []corpusRoot) (string, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve %q: %v", path, err)
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root.abs, abs)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		rel = filepath.ToSlash(rel)
		if root.label == "" {
			return filepath.Join(root.abs, rel), rel, nil
		}
		return filepath.Join(root.abs, rel), root.label + "/" + rel, nil
	}
	return "", "", fmt.Errorf("%q is not inside any corpus root", path)
}

// readBounded reads a regular file of at most 1MB.
func readBounded(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	source, err := io.ReadAll(io.LimitReader(file, 1_000_001))
	if err != nil {
		return "", err
	}
	if len(source) > 1_000_000 {
		return "", errors.New("file exceeds the 1MB limit")
	}
	return string(source), nil
}

func readInput(spec string, stdin io.Reader) (string, string, error) {
	if spec == "-" {
		source, err := io.ReadAll(io.LimitReader(stdin, 1_000_001))
		if err != nil {
			return "", "", err
		}
		if len(source) > 1_000_000 {
			return "", "", errors.New("stdin exceeds the 1MB limit")
		}
		return string(source), "", nil
	}
	source, err := readBounded(spec)
	if err != nil {
		return "", "", fmt.Errorf("cannot read %q: %v", spec, err)
	}
	return source, spec, nil
}

type diffChange struct {
	path  string
	lines []int
}

// diffLines extracts the added lines of every file in a unified diff.
func diffLines(diff string) []diffChange {
	var changes []diffChange
	var current *diffChange
	inHunk := false
	newLine := 0
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff "):
			if current != nil {
				changes = append(changes, *current)
				current = nil
			}
			inHunk = false
		case strings.HasPrefix(line, "+++ ") && !inHunk:
			path := strings.TrimPrefix(line[4:], "b/")
			// Git appends a tab and a timestamp to the path.
			if at := strings.IndexByte(path, '\t'); at >= 0 {
				path = path[:at]
			}
			current = &diffChange{path: path}
		case strings.HasPrefix(line, "@@"):
			newLine = hunkStart(line, '+')
			inHunk = true
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

// hunkStart reads the new-file start line from a hunk header. The regex is
// anchored to the second @@ pair so a section heading after it cannot move
// the number.
var hunkStartRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func hunkStart(header string, side byte) int {
	_ = side
	if match := hunkStartRe.FindStringSubmatch(header); match != nil {
		start, err := strconv.Atoi(match[1])
		if err == nil {
			return start
		}
	}
	return 1
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
	Results         []aroma.SearchResult   `json:"results"`
	Recommendations []aroma.Recommendation `json:"recommendations,omitempty"`
}

type jsonOutput struct {
	SchemaVersion int         `json:"schema_version"`
	Command       string      `json:"command"`
	Queries       []jsonQuery `json:"queries"`
}

func writeJSON(stdout, stderr io.Writer, command string, index *aroma.Index, queries []aroma.Query, options runOptions) int {
	out := jsonOutput{SchemaVersion: SchemaVersion, Command: command, Queries: []jsonQuery{}}
	for _, query := range queries {
		entry := jsonQuery{}
		entry.Query.Size = query.Size
		entry.Query.LowSignal = query.Size < lowSignalQuerySize
		entry.Query.Language = query.Language
		entry.Query.Lines = query.Lines
		entry.Query.Origin = query.Origin
		if command == "search" {
			entry.Results = aroma.Search(index, query, aroma.SearchOptions{MinSimilarity: options.min, Limit: options.limit})
			if entry.Results == nil {
				entry.Results = []aroma.SearchResult{}
			}
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
		fmt.Fprintf(stderr, "pylon aroma: %v\n", err)
		return 2
	}
	return 0
}

func writeHuman(stdout io.Writer, command string, index *aroma.Index, queries []aroma.Query, options runOptions) {
	for _, query := range queries {
		fmt.Fprintf(stdout, "query %d features", query.Size)
		if query.Size < lowSignalQuerySize {
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
					cleanText(result.Method.Path), cleanText(result.Method.Name),
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
				cleanText(recommendation.Method.Path), cleanText(recommendation.Method.Name),
				recommendation.Method.StartLine, recommendation.Method.EndLine,
				recommendation.Similarity, len(recommendation.Members))
			printLines(stdout, recommendation.Lines)
		}
	}
}

// cleanText strips control characters, including ANSI escape sequences,
// from corpus-derived text before it reaches the terminal.
func cleanText(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, text)
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
		fmt.Fprintf(stdout, "    %s%s\n", marker, cleanText(line.Text))
	}
}
