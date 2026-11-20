// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package sync

import "os/exec"

// detach is a no-op on Windows: children survive parent exit by
// default.
func detach(cmd *exec.Cmd) {}
