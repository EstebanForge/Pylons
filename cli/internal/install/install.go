// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Package install wires the Pylons gate hooks into the surrounding git
// repository: hook scripts in .githooks/ and core.hooksPath pointing there.
package install

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

//go:embed assets/pre-commit assets/pre-push
var assets embed.FS

var hookNames = []string{"pre-commit", "pre-push"}

// ErrNotGit reports that the target is not inside a git repository.
var ErrNotGit = errors.New("not inside a git repository")

// ErrHooksPathConflict reports an existing core.hooksPath owned by
// another tool; --force is required to replace it.
var ErrHooksPathConflict = errors.New("core.hooksPath already points elsewhere")

// Result reports what EnsureHooks changed.
type Result struct {
	WroteHooks   []string
	SkippedHooks []string
	DriftedHooks []string
	HooksPathSet bool
	BinaryOK     bool
	// PreviousHooksPath carries the core.hooksPath value a --force
	// takeover replaced, so the handover stays reversible.
	PreviousHooksPath string
}

// EnsureHooks writes missing .githooks scripts into the git toplevel of
// kbRoot, enables core.hooksPath, and checks for bin/pylon. Existing hook
// files are never overwritten unless force is set. The error is non-nil
// when kbRoot is not inside a git repository, when the KB is not at the
// repository root (hooks would gate the wrong repo), or when another
// tool owns core.hooksPath and force is false.
func EnsureHooks(kbRoot string, force bool) (Result, error) {
	var res Result

	// The toplevel comes back absolute; the KB must be compared in kind.
	if abs, err := filepath.Abs(kbRoot); err == nil {
		kbRoot = abs
	}
	top, err := gitTopLevel(kbRoot)
	if err != nil {
		return res, err
	}
	if filepath.Clean(top) != filepath.Clean(kbRoot) {
		return res, fmt.Errorf("KB is not at the repository root (%s); hooks would gate the wrong repo", top)
	}

	curBytes, err := resolve.GitRun(top, nil, "config", "--get", "core.hooksPath")
	currentPath := strings.TrimSpace(string(curBytes))
	if err == nil && currentPath != "" && currentPath != ".githooks" && !force {
		return res, fmt.Errorf("%w: %s (use pylon install --force to replace)", ErrHooksPathConflict, currentPath)
	}
	if err == nil && currentPath != "" && currentPath != ".githooks" && force {
		res.PreviousHooksPath = currentPath
	}

	if err := os.MkdirAll(filepath.Join(top, ".githooks"), 0o755); err != nil {
		return res, err
	}
	for _, name := range hookNames {
		dst := filepath.Join(top, ".githooks", name)
		body, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return res, err
		}
		existing, statErr := os.ReadFile(dst)
		switch {
		case statErr == nil && string(existing) == string(body):
			res.SkippedHooks = append(res.SkippedHooks, name)
		case statErr == nil && !force:
			// Keep a locally modified hook, but flag the drift.
			res.DriftedHooks = append(res.DriftedHooks, name)
		default:
			// A forced overwrite of an existing hook keeps the old copy
			// beside it, so a takeover is reversible.
			if statErr == nil {
				bak := dst + ".bak." + time.Now().Format("20060102-150405")
				if err := os.WriteFile(bak, existing, 0o755); err != nil {
					return res, err
				}
			}
			if err := os.WriteFile(dst, body, 0o755); err != nil {
				return res, err
			}
			res.WroteHooks = append(res.WroteHooks, name)
		}
		if info, err := os.Stat(dst); err == nil && info.Mode()&0o111 == 0 {
			// Git silently ignores non-executable hooks.
			if err := os.Chmod(dst, 0o755); err != nil {
				return res, err
			}
		}
	}

	want := ".githooks"
	out, err := resolve.GitRun(top, nil, "config", "--get", "core.hooksPath")
	if err != nil || strings.TrimSpace(string(out)) != want {
		if _, err := resolve.GitRun(top, nil, "config", "core.hooksPath", want); err != nil {
			return res, err
		}
		res.HooksPathSet = true
	}

	res.BinaryOK = binaryPresent(top)
	return res, nil
}

// binaryPresent reports whether a built CLI exists beside the KB
// (bin/pylon, or bin/pylon.exe on Windows).
func binaryPresent(top string) bool {
	for _, name := range []string{"pylon", "pylon.exe"} {
		if st, err := os.Stat(filepath.Join(top, "bin", name)); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func gitTopLevel(kbRoot string) (string, error) {
	out, err := resolve.GitRun(kbRoot, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s: %w", kbRoot, ErrNotGit)
	}
	return strings.TrimSpace(string(out)), nil
}
