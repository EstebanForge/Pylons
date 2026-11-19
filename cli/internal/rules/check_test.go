// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// A target under two registered roots must always resolve to the deepest
// one, regardless of Go's randomized map iteration order.
func TestNormalizeCheckTargetDeterministic(t *testing.T) {
	tmp := t.TempDir()
	outer := filepath.Join(tmp, "outer")
	inner := filepath.Join(outer, "inner")
	for _, d := range []string{outer, inner} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{Projects: map[string]config.ProjectConfig{
		"outer": {Path: outer},
		"inner": {Path: inner},
	}}
	target := filepath.Join(inner, "f.md")
	for i := 0; i < 200; i++ {
		nt := normalizeCheckTarget(tmp, target, cfg)
		if nt.alias != "inner" || nt.path != "f.md" {
			t.Fatalf("run %d: got alias=%q path=%q, want inner f.md", i, nt.alias, nt.path)
		}
	}
}
