// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Package sync implements detached-freshness for a KB repository: a
// detached background fetch that never blocks, fast-forward-only
// updates on a clean main, and a state file that surfaces failures.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// StalenessWindow gates a new fetch: a state older than this launches
// another detached fetch.
const StalenessWindow = 60 * time.Minute

// StuckWindow judges a lost child: a running marker older than this
// counts as a failed fetch.
const StuckWindow = 10 * time.Minute

// fetchTimeout bounds the synchronous fetch in the detached child; it
// is longer than the generic git call timeout because a fetch can
// legitimately walk the network.
const fetchTimeout = 60 * time.Second

// ErrNotGit mirrors the engine's fail-closed sentinel for callers
// outside a repository.
var ErrNotGit = errors.New("not a git repository")

// Result reports what one Run did.
type Result struct {
	FetchStarted  bool      `json:"fetch_started"`
	FetchFailed   bool      `json:"fetch_failed"`
	FailedAt      time.Time `json:"failed_at"`
	FastForwarded bool      `json:"fast_forwarded"`
	Behind        int       `json:"behind"`
	Diverged      bool      `json:"diverged"`
}

// state is the durable record the detached fetch child writes. The
// running marker carries FailedFinish across, so a failed fetch
// surfaces until a later fetch succeeds.
type state struct {
	LastStart    time.Time `json:"last_start"`
	LastFinish   time.Time `json:"last_finish"`
	LastExit     int       `json:"last_exit"`
	FailedFinish time.Time `json:"last_failed_finish"`
}

// Run launches a detached fetch when the last one is stale, then
// applies a fast-forward update on a clean main. Core commands stay
// offline: only sync talks to the network.
func Run(kbRoot string) (Result, error) {
	gitDir, err := gitDirOf(kbRoot)
	if err != nil {
		return Result{}, err
	}
	self, err := os.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("resolve executable: %w", err)
	}

	var res Result
	st, hasState := readState(gitDir)

	// No remote means nothing to fetch; reporting a launched fetch
	// would be a lie.
	if out, err := resolve.GitRun(kbRoot, nil, "remote"); err == nil && strings.TrimSpace(string(out)) == "" {
		return res, nil
	}

	// Launch phase: a missing or stale state starts one detached
	// fetch and records the launch first, so a fast-failing child
	// cannot be overwritten by the parent's write.
	if !hasState || time.Since(st.LastStart) >= StalenessWindow {
		next := state{LastStart: time.Now(), LastExit: -1, FailedFinish: st.FailedFinish}
		if err := writeState(gitDir, next); err != nil {
			return res, fmt.Errorf("record fetch launch: %w", err)
		}
		if err := acquireSyncLock(gitDir); err != nil {
			return res, fmt.Errorf("lock detached fetch: %w", err)
		}
		child := exec.Command(self, "sync", "--fetch-child", gitDir)
		detach(child)
		if err := child.Start(); err != nil {
			releaseSyncLock(gitDir)
			res.FetchFailed = true
			res.FailedAt = next.LastStart
			return res, fmt.Errorf("launch detached fetch: %w", err)
		}
		if err := child.Process.Release(); err != nil {
			return res, fmt.Errorf("release detached fetch: %w", err)
		}
		res.FetchStarted = true
		st, hasState = next, true
	}

	// Failure surfacing: the last completed fetch must never fail
	// silently, and a child that died without a final write counts as
	// failed once the start is old enough.
	failedAt, failed := failedView(st, hasState)
	res.FetchFailed = failed
	res.FailedAt = failedAt

	// Apply phase: a clean main that is a strict ancestor of
	// origin/main fast-forwards. Anything else is reported, never
	// merged.
	if hasState && st.LastExit == 0 {
		behind, ahead, clean, ffErr := ffState(kbRoot)
		if ffErr != nil {
			return res, ffErr
		}
		res.Behind = behind
		if behind == 0 {
			return res, nil
		}
		if ahead > 0 {
			res.Diverged = true
			return res, nil
		}
		if !clean {
			return res, nil
		}
		branch, err := resolve.GitRun(kbRoot, nil, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return res, fmt.Errorf("resolve current branch: %w", err)
		}
		if strings.TrimSpace(string(branch)) != "main" {
			res.Diverged = true
			return res, nil
		}
		if _, err := resolve.GitRun(kbRoot, nil, "merge", "--ff-only", "origin/main"); err != nil {
			return res, fmt.Errorf("fast-forward: %w", err)
		}
		res.FastForwarded = true
		res.Behind = 0
	}
	return res, nil
}

// failedView reports the last fetch failure: the recorded one, or a
// running marker old enough to mean a lost child.
func failedView(st state, hasState bool) (time.Time, bool) {
	if !hasState {
		return time.Time{}, false
	}
	switch {
	case st.LastExit == 0:
		return time.Time{}, false
	case st.LastExit == -1 && st.FailedFinish.IsZero() && time.Since(st.LastStart) < StuckWindow:
		return time.Time{}, false
	default:
		if !st.FailedFinish.IsZero() {
			return st.FailedFinish, true
		}
		if st.LastExit == -1 {
			return st.LastStart, true
		}
		return st.LastFinish, true
	}
}

// Status reads the last fetch state for brief: (failedAt, ok) where ok
// means no failed fetch is on record.
func Status(kbRoot string) (time.Time, bool, error) {
	gitDir, err := gitDirOf(kbRoot)
	if err != nil {
		return time.Time{}, true, err
	}
	st, hasState := readState(gitDir)
	failedAt, failed := failedView(st, hasState)
	return failedAt, !failed, nil
}

// FetchChild runs the synchronous half inside the detached child: the
// fetch, then the durable status write. It returns the process exit.
func FetchChild(gitDir string) int {
	st := state{LastStart: time.Now(), LastExit: -1}
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return 1
	}
	if prev, ok := readState(gitDir); ok {
		st.FailedFinish = prev.FailedFinish
	}
	releaseSyncLock(gitDir)

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", filepath.Dir(gitDir), "fetch", "origin", "main")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	err := cmd.Run()
	st.LastExit = 1
	st.FailedFinish = time.Now()
	if err == nil {
		st.LastExit = 0
		st.FailedFinish = time.Time{}
	}
	st.LastFinish = time.Now()
	if writeState(gitDir, st) != nil {
		return 1
	}
	return st.LastExit
}

// GitDirOf resolves the absolute git directory: a real directory even
// in worktrees and submodules, where .git is a file.
func GitDirOf(kbRoot string) (string, error) {
	return gitDirOf(kbRoot)
}

// gitDirOf resolves the absolute git directory: a real directory even
// in worktrees and submodules, where .git is a file.
func gitDirOf(kbRoot string) (string, error) {
	out, err := resolve.GitRun(kbRoot, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", ErrNotGit
	}
	return strings.TrimSpace(string(out)), nil
}

// ffState reports how far behind origin/main the local main is, how
// far ahead, and whether the working tree is clean.
func ffState(kbRoot string) (behind, ahead int, clean bool, err error) {
	out, err := resolve.GitRun(kbRoot, nil, "rev-list", "--left-right", "--count", "main...origin/main")
	if err != nil {
		return 0, 0, false, fmt.Errorf("compare with origin/main: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 {
		return 0, 0, false, fmt.Errorf("unexpected rev-list output: %q", strings.TrimSpace(string(out)))
	}
	ahead, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, false, fmt.Errorf("unexpected rev-list output: %q", strings.TrimSpace(string(out)))
	}
	behind, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, false, fmt.Errorf("unexpected rev-list output: %q", strings.TrimSpace(string(out)))
	}
	status, err := resolve.GitRun(kbRoot, nil, "status", "--porcelain")
	if err != nil {
		return 0, 0, false, err
	}
	return behind, ahead, strings.TrimSpace(string(status)) == "", nil
}

func statePath(gitDir string) string { return filepath.Join(gitDir, "pylon-sync.json") }

func readState(gitDir string) (state, bool) {
	raw, err := os.ReadFile(statePath(gitDir))
	if err != nil {
		return state{}, false
	}
	var st state
	if json.Unmarshal(raw, &st) != nil {
		return state{}, false
	}
	return st, true
}

// writeState stores the record atomically; a partial state file must
// never be readable.
func writeState(gitDir string, st state) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(gitDir, "pylon-sync-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), statePath(gitDir))
}

// acquireSyncLock keeps two concurrent sync runs from launching two
// children. A lock left by a dead process is taken over; a lock from
// a live fetch skips the launch. The child releases it.
func acquireSyncLock(gitDir string) error {
	lock := filepath.Join(gitDir, "pylon-sync.lock")
	if raw, err := os.ReadFile(lock); err == nil {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil {
			if pidAlive(pid) {
				return fmt.Errorf("a fetch is already running (pid %d)", pid)
			}
		}
	}
	if err := os.WriteFile(lock, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return err
	}
	return nil
}

// releaseSyncLock removes the lock if it belongs to this process; a
// lock taken over by another run stays.
func releaseSyncLock(gitDir string) {
	lock := filepath.Join(gitDir, "pylon-sync.lock")
	if raw, err := os.ReadFile(lock); err == nil {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid == os.Getpid() {
			_ = os.Remove(lock)
		}
	}
}
