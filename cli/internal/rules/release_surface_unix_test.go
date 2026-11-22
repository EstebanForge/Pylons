// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package rules

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/estebanforge/pylons/cli/internal/report"
)

func TestReleaseSurfaceFifoLockfileNoHang(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "0.1.0", map[string]string{"consumer.go": "Process(1)\n"})
	if err := syscall.Mkfifo(filepath.Join(fx.lockDir, "poison.lock"), 0o600); err != nil {
		t.Skip("fifo unsupported")
	}
	done := make(chan []report.Finding, 1)
	go func() {
		fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir})
		if err != nil {
			fs = nil
		}
		done <- fs
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the gate hung on a FIFO named *.lock")
	}
}
