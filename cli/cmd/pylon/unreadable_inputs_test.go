// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unreadable input must never read as empty: a directory in place of
// a file fails closed with exit 2, and an unreadable base or pins dir
// degrades to the not-evaluated advisory instead of a clean pass.
func TestUnreadableInputsFailClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory in place of the merge-danger body.
	if code := run([]string{"review", "merge-danger", "--body", dir, "--pylons", dir}); code != 2 {
		t.Fatalf("a directory as --body must exit 2, got %d", code)
	}
	// A directory in place of the annotations input.
	if code := run([]string{"review", "annotations", "--from", dir, "--pylons", dir}); code != 2 {
		t.Fatalf("a directory as --from must exit 2, got %d", code)
	}
	// A missing pinned-releases directory stays report-only.
	missing := filepath.Join(dir, "no-such-pins")
	if code := run([]string{"review", "release-surface", "--pinned-releases", missing, "--pylons", dir}); code != 0 {
		t.Fatalf("a missing pins dir must stay advisory at exit 0, got %d", code)
	}
}

func TestReviewBriefCarriesSeverityRubric(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := `---
title: Auth
type: plan
status: accepted
components: [svc]
contract:
  editing:
    - auth/**
---

# Auth Plan

## Problem

Auth problem.
`
	if err := os.WriteFile(filepath.Join(dir, "plans", "feature-auth.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"review-brief", "plans/feature-auth.md", "--pylons", dir}); code != 0 {
		t.Fatalf("review-brief returned %d, want 0", code)
	}
	brief, err := os.ReadFile(filepath.Join(dir, ".pylon-review-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(brief), "trigger cannot be shown concretely") {
		t.Fatal("the findings section must carry the severity rubric")
	}
}
