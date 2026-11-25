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
}
