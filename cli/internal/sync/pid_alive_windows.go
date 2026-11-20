// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package sync

import "os"

// pidAlive checks liveness through process lookup: a missing pid
// errors, a live one does not.
func pidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
