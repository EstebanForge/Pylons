// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The eighteenth review: diff parsing must be hunk-aware, paths must
// survive git's quoting and tab suffixes, and free-text retro records
// must accumulate on their own text instead of one shared signature.

func TestHarvestDiffContentCannotHijackPaths(t *testing.T) {
	kb := harvestKB(t)
	diff := strings.Join([]string{
		"diff --git a/svc/src/api.go b/svc/src/api.go",
		"--- a/svc/src/api.go",
		"+++ b/svc/src/api.go",
		"@@ -1,2 +1,4 @@",
		" package svc",
		"+func Added() {}",
		"+++ b/svc/evil.go",
		"+func Hijack() {}",
		"++++ b/svc/never.go",
		"+func Never() {}",
		"@@ -9,1 +11,2 @@",
		"+func StillApi() {}",
	}, "\n") + "\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	var names []string
	for _, s := range rep.Symbols {
		names = append(names, s.ID)
	}
	// In-hunk +++/++++ lines are added content of the real file: they
	// may add symbols, but the path itself must never switch.
	for _, s := range rep.Symbols {
		if s.Path != "svc/src/api.go" {
			t.Fatalf("in-hunk +++ lines must stay content, not switch paths: %+v", s)
		}
	}
	if len(rep.Symbols) != 4 {
		t.Fatalf("Added, Hijack, Never, StillApi all belong to api.go: %+v", rep.Symbols)
	}
}

func TestHarvestPathSuffixesAndQuotes(t *testing.T) {
	kb := harvestKB(t)
	for _, f := range []string{"svc/my file.go", "svc/uni.go", "svc/src/Gate.php"} {
		if err := os.WriteFile(filepath.Join(kb, filepath.FromSlash(f)), []byte("package svc\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	diff := strings.Join([]string{
		"diff --git a/svc/my file.go b/svc/my file.go",
		"--- a/svc/my file.go",
		"+++ b/svc/my file.go\t2026-01-01",
		"@@ -1,1 +1,2 @@",
		"+func Spaced() {}",
		"diff --git a/svc/uni.go b/svc/uni.go",
		"--- a/svc/uni.go",
		"+++ \"b/svc/uni.go\"",
		"@@ -1,1 +1,2 @@",
		"+func Quoted() {}",
	}, "\n") + "\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	joined := ""
	for _, s := range rep.Symbols {
		joined += s.ID + "\n"
	}
	if !strings.Contains(joined, "::Spaced") {
		t.Fatalf("tab-suffixed path must extract: %s", joined)
	}
	if !strings.Contains(joined, "::Quoted") {
		t.Fatalf("quoted path must extract: %s", joined)
	}
}

func TestHarvestBinaryWarningCarriesPath(t *testing.T) {
	kb := harvestKB(t)
	diff := strings.Join([]string{
		"diff --git a/svc/logo.png b/svc/logo.png",
		"index 123..456 100644",
		"Binary files a/svc/logo.png and b/svc/logo.png differ",
	}, "\n") + "\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "logo.png") {
		t.Fatalf("binary warning must name the file: %+v", rep.Warnings)
	}
}

func TestGoExtractionHardening(t *testing.T) {
	kb := harvestKB(t)
	diff := strings.Join([]string{
		"diff --git a/svc/src/api.go b/svc/src/api.go",
		"--- a/svc/src/api.go",
		"+++ b/svc/src/api.go",
		"@@ -1,1 +1,9 @@",
		" package svc",
		"+func Map[T any](xs []T, f func(T) T) []T { return xs }",
		"+const (",
		"+\tAlpha = 1",
		"+\tbeta = 2",
		"+)",
		"+var (",
		"+\tGamma = 2",
		"+)",
	}, "\n") + "\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	joined := ""
	for _, s := range rep.Symbols {
		joined += s.ID + "\n"
	}
	if !strings.Contains(joined, "::Map") {
		t.Fatalf("generic function must extract: %s", joined)
	}
	if !strings.Contains(joined, "::Alpha") || !strings.Contains(joined, "::Gamma") {
		t.Fatalf("grouped const/var members must extract: %s", joined)
	}
	if strings.Contains(joined, "::beta") {
		t.Fatalf("unexported block member must stay hidden: %s", joined)
	}
}

func TestRustAndPHPExtractionHardening(t *testing.T) {
	kb := harvestKB(t)
	if err := os.WriteFile(filepath.Join(kb, "svc", "src", "Gate.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := strings.Join([]string{
		"diff --git a/svc/src/store.rs b/svc/src/store.rs",
		"--- a/svc/src/store.rs",
		"+++ b/svc/src/store.rs",
		"@@ -1,1 +1,5 @@",
		"+    pub fn open() {}",
		"+pub async fn load() {}",
		"+pub const fn mk() {}",
		"+    pub(crate) unsafe fn raw() {}",
		"diff --git a/svc/src/Gate.php b/svc/src/Gate.php",
		"--- a/svc/src/Gate.php",
		"+++ b/svc/src/Gate.php",
		"@@ -1,1 +1,2 @@",
		"+    public function open() {}",
	}, "\n") + "\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	joined := ""
	for _, s := range rep.Symbols {
		joined += s.Name + " " + s.ID + "\n"
	}
	for _, want := range []string{"::open", "::load", "::mk", "::raw"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in: %s", want, joined)
		}
	}
	if !strings.Contains(joined, "Gate.php") {
		t.Fatalf("php method must extract: %s", joined)
	}
	if strings.Contains(joined, " fn ") || strings.Contains(joined, "::fn") {
		t.Fatalf("modifier words must not become names: %s", joined)
	}
}

func TestUnattributedDedupes(t *testing.T) {
	kb := harvestKB(t)
	diff := "diff --git a/nowhere/one.go b/nowhere/one.go\n" +
		"+++ b/nowhere/one.go\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+func Lost() {}\n" +
		"+func Lost() {}\n"
	rep, err := HarvestPR(kb, []byte(diff), nil)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	found := 0
	for _, u := range rep.Unattributed {
		if strings.Contains(u.Name, "Lost") {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("identical unattributed entries must dedupe, got %d: %+v", found, rep.Unattributed)
	}
}
