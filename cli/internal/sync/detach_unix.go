// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package sync

import (
	"os/exec"
	"syscall"
)

// detach gives the fetch child its own session so it survives the
// parent process exit.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
