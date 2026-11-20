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

func TestEmptyCorpusExits1(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	code, _, stderr := runCmd(t, dir, "search", "--corpus", ".", "--lines", "x.go:1-2")
	if code != 1 || !strings.Contains(stderr, "no files of a known language") {
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
