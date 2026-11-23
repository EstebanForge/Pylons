// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox builds a tiny corpus with two similar methods and runs the CLI in it.
func sandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/a.go", "package a\n\nfunc process(items []string) error {\n\tfor _, item := range items {\n\t\tif item == \"\" {\n\t\t\tcontinue\n\t\t}\n\t\tprintln(item)\n\t}\n\treturn nil\n}\n")
	write("b/b.go", "package b\n\nfunc handle(rows []string) error {\n\tfor _, row := range rows {\n\t\tif row == \"\" {\n\t\t\tcontinue\n\t\t}\n\t\tprintln(row)\n\t}\n\treturn nil\n}\n")
	write("notes.txt", "nothing here")
	return dir
}

func runCmd(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runAroma(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func TestSearchLinesEndToEnd(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, stdout, stderr := runCmd(t, dir, "search", "--corpus", ".", "--lines", "a/a.go:4-7")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "b/b.go:handle:3-11") {
		t.Fatalf("stdout = %q, want handle", stdout)
	}
	if !strings.Contains(stderr, "indexed 2 files") {
		t.Fatalf("stats = %q", stderr)
	}
}

func TestSearchJSONSchema(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, stdout, _ := runCmd(t, dir, "search", "--corpus", ".", "--lines", "a/a.go:4-7", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var out struct {
		SchemaVersion int `json:"schema_version"`
		Queries       []struct {
			Query struct {
				Size      int  `json:"size"`
				LowSignal bool `json:"lowSignal"`
				Origin    *struct {
					Name string `json:"name"`
				} `json:"origin"`
			} `json:"query"`
			Results []struct {
				Method struct {
					Name string `json:"name"`
				} `json:"method"`
				Similarity float64 `json:"similarity"`
			} `json:"results"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json = %q: %v", stdout, err)
	}
	if out.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d", out.SchemaVersion)
	}
	if len(out.Queries) != 1 || len(out.Queries[0].Results) == 0 {
		t.Fatalf("queries = %+v", out.Queries)
	}
	if out.Queries[0].Query.Origin == nil || out.Queries[0].Query.Origin.Name != "process" {
		t.Fatalf("origin = %+v", out.Queries[0].Query.Origin)
	}
	if out.Queries[0].Results[0].Method.Name != "handle" {
		t.Fatalf("results = %+v", out.Queries[0].Results)
	}
}

func TestRecommendJSONEndToEnd(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, stdout, _ := runCmd(t, dir, "recommend", "--corpus", ".", "--lines", "a/a.go:4-7", "--json", "--min-support", "1")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var out struct {
		Queries []struct {
			Recommendations []struct {
				Method struct {
					Name string `json:"name"`
				} `json:"method"`
				Lines []struct {
					Role string `json:"role"`
				} `json:"lines"`
			} `json:"recommendations"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json = %q: %v", stdout, err)
	}
	if len(out.Queries) != 1 || len(out.Queries[0].Recommendations) == 0 {
		t.Fatalf("out = %+v", out)
	}
	recommendation := out.Queries[0].Recommendations[0]
	if recommendation.Method.Name != "handle" {
		t.Fatalf("recommendation = %+v", recommendation)
	}
	var added int
	for _, line := range recommendation.Lines {
		if line.Role == "added" {
			added++
		}
	}
	if added == 0 {
		t.Fatalf("no added lines: %+v", recommendation.Lines)
	}
}

func TestDiffStdinQueries(t *testing.T) {
	// The diff must match the checkout on disk: the query reads the file's
	// current content at the changed lines.
	dir := sandbox(t)
	added := "package a\n\nfunc process(items []string) error {\n\tfor _, item := range items {\n\t\tif item == \"\" {\n\t\t\tcontinue\n\t\t}\n\t\tprintln(item)\n\t\tprintln(item)\n\t}\n\treturn nil\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a", "a.go"), []byte(added), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	diff := "diff --git a/a/a.go b/a/a.go\n" +
		"--- a/a/a.go\n" +
		"+++ b/a/a.go\n" +
		"@@ -3,6 +3,7 @@ func process(items []string) error {\n" +
		" \tfor _, item := range items {\n" +
		" \t\tif item == \"\" {\n" +
		" \t\t\tcontinue\n" +
		" \t\t}\n" +
		" \t\tprintln(item)\n" +
		"+\t\tprintln(item)\n" +
		" \t}\n"
	var stdout, stderr bytes.Buffer
	code := runAroma([]string{"search", "--corpus", ".", "--diff", "-", "--json"}, strings.NewReader(diff), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "handle") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSnippetStdin(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	code := runAroma([]string{"search", "--corpus", ".", "--snippet", "-", "--lang", "go", "--min", "0.4"}, strings.NewReader("for _, item := range items {\nprintln(item)\n}\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "process") || !strings.Contains(stdout.String(), "handle") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUnknownFlagExits2(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--wat")
	if code != 2 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestEmptyCorpusExits2(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	// An empty corpus is an environment error like an unreadable corpus,
	// not a blocking verdict, so it shares exit 2.
	code, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--lines", "x.go:1-2")
	if code != 2 || !strings.Contains(stderr, "no files of a known language") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestUnreadableCorpusExits2(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", "missing", "--lines", "a/a.go:4-7")
	if code != 2 || !strings.Contains(stderr, "cannot read corpus") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestNoTokensExits0WithNote(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	code := runAroma([]string{"search", "--corpus", ".", "--snippet", "-", "--lang", "go"}, strings.NewReader("}\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no names or literals") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTestFilesExcludedByDefault(t *testing.T) {
	dir := sandbox(t)
	if err := os.WriteFile(filepath.Join(dir, "a", "a_test.go"), []byte("package a\n\nfunc TestX() {\n\tprocess(nil)\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	_, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--lines", "a/a.go:4-7")
	if strings.Contains(stderr, "3 methods") {
		t.Fatalf("test file was indexed: %q", stderr)
	}
	_, _, stderr = runCmd(t, dir, "search", "--corpus", ".", "--include-tests", "--lines", "a/a.go:4-7")
	if !strings.Contains(stderr, "3 methods") {
		t.Fatalf("--include-tests did not index the test file: %q", stderr)
	}
}

// collidingCorpus builds two corpus roots that both hold cmd/x.go with one
// similar method each, the multi-root collision case.
func collidingCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("left/cmd/x.go", "package x\n\nfunc process(items []string) error {\n\tfor _, item := range items {\n\t\tprintln(item)\n\t}\n\treturn nil\n}\n")
	write("right/cmd/x.go", "package x\n\nfunc handle(rows []string) error {\n\tfor _, row := range rows {\n\t\tprintln(row)\n\t}\n\treturn nil\n}\n")
	return dir
}

func TestMultiRootCollidingPathsStayDistinct(t *testing.T) {
	dir := collidingCorpus(t)
	chdir(t, dir)
	code, stdout, stderr := runCmd(t, dir, "search", "--corpus", "left", "--corpus", "right", "--lines", "left/cmd/x.go:4-8")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "2 methods") {
		t.Fatalf("a colliding path overwrote a project: %q", stderr)
	}
	if !strings.Contains(stdout, "left/cmd/x.go") || !strings.Contains(stdout, "right/cmd/x.go") {
		t.Fatalf("results lost a root's method: %q", stdout)
	}
}

func TestOriginExcludedAcrossRoots(t *testing.T) {
	dir := collidingCorpus(t)
	chdir(t, dir)
	// The query file lives under the left root, named from the cwd. The
	// origin method must not answer its own query.
	code, stdout, stderr := runCmd(t, dir, "search", "--corpus", "left", "--corpus", "right", "--min", "0.3", "--lines", "left/cmd/x.go:4-8")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	// The query header names the origin; a result line for it must not
	// appear (result lines are indented).
	if strings.Contains(stdout, "\n  left/cmd/x.go") {
		t.Fatalf("the origin method answered its own query: %q", stdout)
	}
	if !strings.Contains(stdout, "right/cmd/x.go") {
		t.Fatalf("expected only the other root's method: %q", stdout)
	}
}

func TestQueryPathOutsideCorpusFails(t *testing.T) {
	dir := collidingCorpus(t)
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", "left", "--lines", "right/cmd/x.go:4-8")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "not inside any corpus root") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAromaHelpAfterSubcommand(t *testing.T) {
	code, stdout, _ := runCmd(t, "", "search", "--help")
	if code != 0 || !strings.Contains(stdout, "structural code similarity") {
		t.Fatalf("exit = %d, stdout = %q", code, stdout)
	}
}

func TestAromaDispatchHolesExit2(t *testing.T) {
	// Flags before the subcommand skip the early route; the shared parser
	// then reports an unknown command. Documented, not silently different.
	if code := run([]string{"--", "aroma"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestLimitAndSupportRejectNonPositive(t *testing.T) {
	code, _, stderr := runCmd(t, "", "search", "--corpus", ".", "--limit", "0", "--lines", "a/a.go:4-7")
	if code != 2 || !strings.Contains(stderr, "positive") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	code, _, stderr = runCmd(t, "", "recommend", "--corpus", ".", "--min-support", "0", "--lines", "a/a.go:4-7")
	if code != 2 || !strings.Contains(stderr, "at least 1") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestMinZeroIsExpressible(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--min", "0", "--lines", "a/a.go:4-7")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestStdinSnippetDefaultsToGo(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	var stdout, stderr bytes.Buffer
	code := runAroma([]string{"search", "--corpus", ".", "--snippet", "-"}, strings.NewReader("for _, item := range items {\nprintln(item)\n}\n"), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "query") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestDiffParsesHeadersNotBodies(t *testing.T) {
	// A hunk body line that starts with +++ must not open a new file, and
	// a section heading after @@ must not move the hunk start. The disk
	// file matches the diff's new side.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	newSide := "package a\nfunc g() {}\nfunc process(items []string) error {\n\tprintln(1)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a", "a.go"), []byte(newSide), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	diff := "diff --git a/a/a.go b/a/a.go\n" +
		"--- a/a.go\n" +
		"+++ b/a/a.go\t2026-01-01 00:00:00.000000000 +0000\n" +
		"@@ -1,3 +1,5 @@ func withHeading(x int) int {\n" +
		" package a\n" +
		"-func f() {}\n" +
		"+++ not a file header\n" +
		"+func g() {}\n" +
		"+func process(items []string) error {\n" +
		"+\tprintln(1)\n" +
		"+}\n"
	var stdout, stderr bytes.Buffer
	code := runAroma([]string{"search", "--corpus", ".", "--diff", "-", "--json"}, strings.NewReader(diff), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "a/a.go") {
		t.Fatalf("the +++ body line broke the file path: %q", stdout.String())
	}
}

func TestReleaseSurfaceFlagPath(t *testing.T) {
	// The flag path itself: --pinned-releases is required, --base is a
	// value flag, and the advisory renders with exit 0.
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	pin := filepath.Join(dir, "deploy")
	if err := os.MkdirAll(pin, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"review", "release-surface", "--pylons", dir}); code != 2 {
		t.Fatalf("missing --pinned-releases must exit 2, got %d", code)
	}
	code := run([]string{"review", "release-surface", "--pinned-releases", pin, "--pylons", dir})
	if code != 0 {
		t.Fatalf("no pins must be advisory exit 0, got %d", code)
	}
	// --base is a value flag; it parses even against a refless KB.
	if code := run([]string{"review", "release-surface", "--pinned-releases", pin, "--base", "origin/main", "--pylons", dir}); code != 0 {
		t.Fatalf("--base must be a value flag, got %d", code)
	}
}

func TestCaptureFlagPath(t *testing.T) {
	dir := t.TempDir()
	if code := run([]string{"init", dir}); code != 0 {
		t.Fatalf("init returned %d, want 0", code)
	}
	if code := run([]string{"context", "--diff-since", "--pylons", dir}); code != 2 {
		t.Fatalf("--diff-since without a value must exit 2, got %d", code)
	}
	// A KB outside git reports every doc as unmoved; capture is
	// report-only and exits 0.
	if code := run([]string{"context", "--diff-since", "HEAD", "--pylons", dir}); code != 0 {
		t.Fatalf("capture must exit 0, got %d", code)
	}
	code := run([]string{"context", "--diff-since", "HEAD", "--manifest", "--pylons", dir})
	_ = code
}

func TestLinesSpecReversedOrOutOfRangeIsUsage(t *testing.T) {
	dir := sandbox(t)
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--lines", "a/a.go:9-1")
	if code != 2 {
		t.Fatalf("a reversed range is a usage error, exit = %d, stderr = %q", code, stderr)
	}
	code, _, stderr = runCmd(t, dir, "search", "--corpus", ".", "--lines", "a/a.go:9999-10001")
	if code != 2 {
		t.Fatalf("a range past the file end is a usage error, exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "past the end") {
		t.Fatalf("the out-of-range error must say so, stderr = %q", stderr)
	}
}

// A symlinked corpus root and diff paths written against the realpath must
// meet: resolveQueryPath evaluates query symlinks while roots are already
// evaluated, so both spellings of one directory resolve to the same files.
func TestAromaSymlinkedRootLinesAndDiff(t *testing.T) {
	real := t.TempDir()
	src := filepath.Join(real, "a.go")
	// The diff change is already applied on disk: diff queries read the
	// checked-out file at the new line numbers.
	if err := os.WriteFile(src, []byte("package a\n\nfunc Alpha() int {\n\treturn 1\n}\n\nfunc Beta() int {\n\treturn Alpha() + 1\n}\n\nfunc Gamma() int {\n\treturn 3\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	// Lines query through the symlinked root.
	code, stdout, stderr := runCmd(t, "", "search", "--corpus", link, "--lines", link+"/a.go:3-5", "--json")
	if code != 0 {
		t.Fatalf("lines through symlinked root: code %d stderr %s", code, stderr)
	}
	if !strings.Contains(stdout, "a.go") {
		t.Fatalf("no result for symlinked lines query: %s", stdout)
	}
	// Diff whose b/ side is the realpath while the corpus is the link.
	diff := "diff --git a/a.go b/a.go\nindex 111..222 100644\n--- a/a.go\n+++ b/a.go\n@@ -10,2 +10,5 @@\n\n+func Gamma() int {\n+\treturn 3\n+}\n"
	diffFile := filepath.Join(t.TempDir(), "sample.diff")
	if err := os.WriteFile(diffFile, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	// Applied from a foreign working directory: the relative b/ path must
	// resolve against the corpus root, not the caller's CWD.
	chdir(t, t.TempDir())
	code, stdout, stderr = runCmd(t, "", "search", "--corpus", link, "--diff", diffFile, "--json")
	if code != 0 {
		t.Fatalf("diff from foreign cwd: code %d stderr %s", code, stderr)
	}
	if !strings.Contains(stdout, "Gamma") {
		t.Fatalf("diff query lost the added symbol: %s stderr %s", stdout, stderr)
	}
}
