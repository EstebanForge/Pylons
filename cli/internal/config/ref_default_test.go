// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

// An unset ref must mean working-tree reads. The old default of
// "main" made every project inside a monorepo silently resolve
// against the committed tree, so a lint before commit checked stale
// code and uncommitted files "did not exist".
func TestUnsetRefMeansWorktree(t *testing.T) {
	kb := t.TempDir()
	writeToggleManifest(t, kb, "version: 1\nprojects:\n  svc:\n    path: svc\n  pinned:\n    path: other\n    ref: v1\n")
	cfg, err := Load(kb)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ProjectRef("svc"); got != "" {
		t.Errorf("unset ref = %q, want empty (working tree)", got)
	}
	if got := cfg.ProjectRef("pinned"); got != "v1" {
		t.Errorf("explicit ref = %q, want v1", got)
	}
}
