// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func statusKB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests", "pylons.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"plans/proposals", "plans/archive", "investigations/archive", "decisions/proposals", "decisions/archive", "packages/archive", "tickets/archive"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	docs := map[string]string{
		"plans/proposals/feature-p.md":          "---\ntitle: P\ntype: plan\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2000-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# P\n",
		"plans/feature-x.md":                    "---\ntitle: X\ntype: plan\nstatus: in-progress\ncreated: 2026-01-01\nupdated: 2000-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# X\n",
		"plans/feature-y.md":                    "---\ntitle: Y\ntype: plan\nstatus: in-progress\ncreated: 2026-01-01\nupdated: 2000-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# Y\n",
		"plans/feature-c.md":                    "---\ntitle: C\ntype: plan\nstatus: in-progress\ncreated: 2026-01-01\nupdated: 2000-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# C\n",
		"investigations/2026-11-01-b.md":        "---\ntitle: B\ntype: investigation\nstatus: active\ncreated: 2026-11-01\nupdated: 2000-01-01\nowners: [o]\nlinks: []\ndescription: d\ntags: [i]\n---\n\n# B\n",
		"decisions/proposals/not-a-decision.md": "---\ntitle: D\ntype: decision\nstatus: proposed\ncreated: 2026-01-01\nupdated: 2000-01-01\nowners: [o]\nsupersedes: []\nartifact: x\nlinks: []\ndescription: d\ntags: [d]\n---\n\n# D\n",
	}
	for rel, body := range docs {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestStatusCommand(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	t.Run("refuses shipped without artifact", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-x.md")
		body, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.Replace(string(body), "artifact: x\n", "", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-x.md", "shipped", "--pylons", dir}); code != 1 {
			t.Fatalf("shipped without artifact must refuse with exit 1, got %d", code)
		}
	})

	t.Run("shipped with artifact moves to archive and bumps updated", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "plans/feature-x.md", "shipped", "--pylons", dir}); code != 0 {
			t.Fatalf("shipped with artifact must exit 0, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "plans", "feature-x.md")); !os.IsNotExist(err) {
			t.Fatal("old path must be gone after the archive move")
		}
		data, err := os.ReadFile(filepath.Join(dir, "plans", "archive", "feature-x.md"))
		if err != nil {
			t.Fatalf("archive target missing: %v", err)
		}
		if !strings.Contains(string(data), "status: shipped") || !strings.Contains(string(data), "updated: "+today) {
			t.Fatal("moved doc must carry the new status and today's updated date")
		}
	})

	t.Run("refuses paused without state section", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "plans/feature-y.md", "paused", "--pylons", dir}); code != 1 {
			t.Fatalf("paused without ## State must refuse with exit 1, got %d", code)
		}
	})

	t.Run("paused with state stays in place", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-y.md")
		body, _ := os.ReadFile(p)
		if err := os.WriteFile(p, append(body, []byte("\n## State\n\ndone: scoping; next: build; blocker: none\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-y.md", "paused", "--pylons", dir}); code != 0 {
			t.Fatalf("paused with ## State must exit 0, got %d", code)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatal("paused is not an archive status; the doc must stay in place")
		}
	})

	t.Run("refuses closed investigation without sections", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "investigations/2026-11-01-b.md", "resolved", "--pylons", dir}); code != 1 {
			t.Fatalf("resolved without sections must refuse with exit 1, got %d", code)
		}
	})

	t.Run("resolved with sections moves to archive", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "investigations", "2026-11-01-b.md")
		if err := os.WriteFile(p, []byte("---\ntitle: B\ntype: investigation\nstatus: active\ncreated: 2026-11-01\nupdated: 2000-01-01\nowners: [o]\nlinks: []\ndescription: d\ntags: [i]\n---\n\n# B\n\n## Root Cause\n\nr\n\n## Resolution\n\nx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "investigations/2026-11-01-b.md", "resolved", "--pylons", dir}); code != 0 {
			t.Fatalf("resolved with sections must exit 0, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "investigations", "archive", "2026-11-01-b.md")); err != nil {
			t.Fatal("closed investigation must move to investigations/archive/")
		}
	})

	t.Run("promotion moves proposal to zone root", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "plans/proposals/feature-p.md", "accepted", "--pylons", dir}); code != 0 {
			t.Fatalf("proposal promotion must exit 0, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "plans", "feature-p.md")); err != nil {
			t.Fatal("accepted plan must sit at the zone root")
		}
	})

	t.Run("refuses unknown status for zone", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "plans/feature-x.md", "resolved", "--pylons", dir}); code != 2 {
			t.Fatalf("status outside the zone set must exit 2, got %d", code)
		}
	})

	t.Run("refuses archive collision", func(t *testing.T) {
		dir := statusKB(t)
		dst := filepath.Join(dir, "plans", "archive", "feature-c.md")
		if err := os.WriteFile(dst, []byte("occupied\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-c.md", "shipped", "--pylons", dir}); code != 1 {
			t.Fatalf("existing archive target must refuse with exit 1, got %d", code)
		}
		data, _ := os.ReadFile(dst)
		if string(data) != "occupied\n" {
			t.Fatal("collision refusal must never overwrite the target")
		}
	})

	t.Run("refuses accepted decision with wrong filename", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "decisions/proposals/not-a-decision.md", "accepted", "--pylons", dir}); code != 1 {
			t.Fatalf("accepted decision with a non-conforming filename must refuse with exit 1, got %d", code)
		}
	})

	t.Run("abandoned does not demand artifact", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-x.md")
		body, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.Replace(string(body), "artifact: x\n", "", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-x.md", "abandoned", "--pylons", dir}); code != 0 {
			t.Fatalf("abandoned plans need no artifact (lint ships PYLON009 for shipped only), got %d", code)
		}
	})

	t.Run("crlf doc transitions fully", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-x.md")
		body, _ := os.ReadFile(p)
		crlf := strings.ReplaceAll(string(body), "\n", "\r\n")
		if err := os.WriteFile(p, []byte(crlf), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-x.md", "shipped", "--pylons", dir}); code != 0 {
			t.Fatalf("crlf doc transition must exit 0, got %d", code)
		}
		data, err := os.ReadFile(filepath.Join(dir, "plans", "archive", "feature-x.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "status: shipped") || !strings.Contains(string(data), "updated: "+today) {
			t.Fatal("crlf patch must actually land: new status and today's date required")
		}
	})

	t.Run("refuses doc without a status line", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-x.md")
		body, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.Replace(string(body), "status: in-progress\n", "", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-x.md", "shipped", "--pylons", dir}); code != 2 {
			t.Fatalf("missing status line must exit 2, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "plans", "archive", "feature-x.md")); !os.IsNotExist(err) {
			t.Fatal("refusal must not move the file")
		}
	})

	t.Run("refuses empty status value", func(t *testing.T) {
		dir := statusKB(t)
		p := filepath.Join(dir, "plans", "feature-x.md")
		body, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.Replace(string(body), "status: in-progress", "status:", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-x.md", "shipped", "--pylons", dir}); code != 2 {
			t.Fatalf("empty status value must exit 2, got %d", code)
		}
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), "shipped") {
			t.Fatal("refusal must not write anything")
		}
	})

	t.Run("creates a missing archive dir", func(t *testing.T) {
		dir := statusKB(t)
		if err := os.WriteFile(filepath.Join(dir, "tickets", "t-1.md"), []byte("---\ntitle: T\ntype: ticket\nstatus: in-progress\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [o]\nlinks: []\ndescription: d\ntags: [t]\n---\n\n# T\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(dir, "tickets", "archive")); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "tickets/t-1.md", "done", "--pylons", dir}); code != 0 {
			t.Fatalf("done with missing archive dir must succeed, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "tickets", "archive", "t-1.md")); err != nil {
			t.Fatal("the archive move must create the missing archive dir")
		}
	})

	t.Run("moves proposed plans into proposals", func(t *testing.T) {
		dir := statusKB(t)
		if err := os.WriteFile(filepath.Join(dir, "plans", "feature-dem.md"), []byte("---\ntitle: Dem\ntype: plan\nstatus: accepted\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# Dem\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-dem.md", "proposed", "--pylons", dir}); code != 0 {
			t.Fatalf("proposed transition must exit 0, got %d", code)
		}
		if _, err := os.Stat(filepath.Join(dir, "plans", "proposals", "feature-dem.md")); err != nil {
			t.Fatal("proposed plans belong in plans/proposals/")
		}
	})

	t.Run("accepts heading suffixes like the linter", func(t *testing.T) {
		dir := statusKB(t)
		if err := os.WriteFile(filepath.Join(dir, "investigations", "2026-11-02-s.md"), []byte("---\ntitle: S\ntype: investigation\nstatus: active\ncreated: 2026-11-02\nupdated: 2026-11-02\nowners: [o]\nlinks: []\ndescription: d\ntags: [i]\n---\n\n# S\n\n## Root Cause: cache\n\nr\n\n## Resolution (final)\n\nx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "investigations/2026-11-02-s.md", "resolved", "--pylons", dir}); code != 0 {
			t.Fatalf("heading suffixes must be accepted exactly like the linter accepts them, got %d", code)
		}
	})

	t.Run("audit investigations skip the section obligation", func(t *testing.T) {
		dir := statusKB(t)
		if err := os.WriteFile(filepath.Join(dir, "investigations", "2026-11-03-a.md"), []byte("---\ntitle: A\ntype: audit\nstatus: active\ncreated: 2026-11-03\nupdated: 2026-11-03\nowners: [o]\nlinks: []\ndescription: d\ntags: [i]\n---\n\n# A\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "investigations/2026-11-03-a.md", "resolved", "--pylons", dir}); code != 0 {
			t.Fatalf("audit docs are not held to PYLON031 by the linter, got %d", code)
		}
	})

	t.Run("refuses symlinked docs", func(t *testing.T) {
		dir := statusKB(t)
		real := filepath.Join(t.TempDir(), "real.md")
		if err := os.WriteFile(real, []byte("---\ntitle: R\ntype: plan\nstatus: in-progress\ncreated: 2026-01-01\nupdated: 2026-01-01\nowners: [o]\nartifact: x\nlinks: []\ndescription: d\ntags: [p]\n---\n\n# R\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "plans", "feature-link.md")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		if code := run([]string{"status", "plans/feature-link.md", "shipped", "--pylons", dir}); code != 2 {
			t.Fatalf("symlinked docs must refuse with exit 2, got %d", code)
		}
		data, _ := os.ReadFile(real)
		if strings.Contains(string(data), "shipped") {
			t.Fatal("refusal must not write through the symlink")
		}
	})

	t.Run("refuses zone entry guides", func(t *testing.T) {
		dir := statusKB(t)
		if code := run([]string{"status", "packages/README.md", "retired", "--pylons", dir}); code != 2 {
			t.Fatalf("README/AGENTS entry guides must refuse with exit 2, got %d", code)
		}
	})
}
