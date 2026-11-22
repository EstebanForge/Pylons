// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package rules

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestImportBoundarySkipsNonRegular(t *testing.T) {
	// A FIFO named *.go must not hang the walk; a symlink is not read.
	kb := t.TempDir()
	svc, helper := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(helper, "go.mod"), "module example.com/helper\n")
	mustWrite(t, filepath.Join(svc, "go.mod"), "module example.com/svc\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nextensions: [go]\nprojects:\n  svc:\n    path: "+svc+"\n    ref: main\n  helper:\n    path: "+helper+"\n    ref: main\n")
	if err := syscall.Mkfifo(filepath.Join(svc, "poison.go"), 0o600); err != nil {
		t.Skip("fifo unsupported")
	}
	mustWrite(t, filepath.Join(kb, "packages/svc.md"), "---\ntitle: \"Svc\"\ntype: package\nstatus: active\npackage: svc\ncomponents: [svc]\nupdated: 2026-11-20\ndepends_on: []\n---\n\n# Svc\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		Lint(kb, LintOptions{HasCode: true})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("lint hung on a FIFO named *.go")
	}
}
