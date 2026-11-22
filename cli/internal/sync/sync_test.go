// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package sync

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitKb builds a commit-less repo (enough for rev-parse and status) at
// dir.
func gitKb(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
}

func TestSyncStateLifecycle(t *testing.T) {
	plain := t.TempDir()
	if _, err := Run(plain); err == nil {
		t.Fatal("sync outside a git repo must error")
	}
	dir := t.TempDir()
	gitKb(t, dir)
	// A commit-less repo passes rev-parse; Run with no remote fails
	// on the fetch child, but state writes are the unit here.
	if _, ok := readState(dir + "/.git"); ok {
		t.Fatal("no state expected before a run")
	}
	writeState(dir+"/.git", state{LastStart: time.Now(), LastFinish: time.Now(), LastExit: 1})
	failedAt, ok, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("a recorded fetch failure must surface")
	}
	if failedAt.IsZero() {
		t.Fatal("the failure time must be recorded")
	}
}

func TestSyncStateJSON(t *testing.T) {
	dir := t.TempDir()
	gitKb(t, dir)
	now := time.Now().Truncate(time.Second)
	writeState(dir+"/.git", state{LastStart: now, LastFinish: now, LastExit: 0})
	st, ok := readState(dir + "/.git")
	if !ok || st.LastExit != 0 {
		t.Fatalf("state round-trip failed: %+v ok=%v", st, ok)
	}
	// The state file is valid JSON with the documented fields.
	raw, err := os.ReadFile(filepath.Join(dir, ".git", "pylon-sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatalf("state file must be JSON: %s", raw)
	}
	if _, ok := doc["last_exit"]; !ok {
		t.Fatalf("state file missing last_exit: %s", raw)
	}
}

func TestFetchChildWritesState(t *testing.T) {
	dir := t.TempDir()
	gitKb(t, dir)
	// No remote: the fetch fails, and the failure must be on record,
	// never silent.
	code := FetchChild(dir + "/.git")
	if code == 0 {
		t.Fatal("fetch without a remote must fail")
	}
	failedAt, ok, err := Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok || failedAt.IsZero() {
		t.Fatal("the failed fetch must surface through Status")
	}
}

// gitCommit makes a commit in repo with the tracked file.
func gitCommit(t *testing.T, repo, file, content, msg string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(file)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	run("add", "-f", file)
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", msg)
}

// gitOut runs git and trims stdout.
func gitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatal(string(out), err)
	}
	return strings.TrimSpace(string(out))
}

// remoteKb builds a KB repo with a bare origin and returns the bare
// path.
func remoteKb(t *testing.T) (kb, bare string) {
	t.Helper()
	kb, bare = t.TempDir(), t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	if out, err := exec.Command("git", "-C", kb, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	gitCommit(t, kb, "f.txt", "a\n", "feat: base")
	if out, err := exec.Command("git", "-C", kb, "remote", "add", "origin", bare).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	if out, err := exec.Command("git", "-C", kb, "push", "-q", "origin", "main").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	return kb, bare
}

// pushFrom pushes a new commit to bare through a scratch clone, so
// origin/main is ahead without touching the KB repo.
func pushFrom(t *testing.T, bare, file, content, msg string) {
	t.Helper()
	scratch := t.TempDir()
	if out, err := exec.Command("git", "clone", "-q", bare, scratch).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	gitCommit(t, scratch, file, content, msg)
	if out, err := exec.Command("git", "-C", scratch, "push", "-q", "origin", "main").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
}

func markFetched(t *testing.T, kb string) {
	t.Helper()
	if err := writeState(filepath.Join(kb, ".git"), state{LastStart: time.Now(), LastFinish: time.Now(), LastExit: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestRunAppliesFastForwardOnMain(t *testing.T) {
	kb, bare := remoteKb(t)
	pushFrom(t, bare, "g.txt", "b\n", "feat: second")
	// Repoint origin inside the KB to the same bare directory the
	// helper used, then fake a completed fetch.
	if out, err := exec.Command("git", "-C", kb, "fetch", "origin").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	markFetched(t, kb)
	res, err := Run(kb)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FastForwarded || res.Behind != 0 {
		t.Fatalf("clean main must fast-forward: %+v", res)
	}
	if head := gitOut(t, kb, "rev-parse", "--short", "HEAD"); gitOut(t, kb, "rev-parse", "--short", "origin/main") != head {
		t.Fatalf("HEAD must equal origin/main after the fast-forward")
	}
}

func TestRunRefusesDivergedMain(t *testing.T) {
	kb, bare := remoteKb(t)
	pushFrom(t, bare, "g.txt", "origin-side\n", "feat: origin-side")
	gitCommit(t, kb, "h.txt", "local-side\n", "feat: local-side")
	if out, err := exec.Command("git", "-C", kb, "fetch", "origin").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	markFetched(t, kb)
	res, err := Run(kb)
	if err != nil {
		t.Fatalf("a diverged tree is report-only, got error: %v", err)
	}
	if !res.Diverged || res.Behind != 1 || res.FastForwarded {
		t.Fatalf("diverged main must be reported, not merged: %+v", res)
	}
	if head := gitOut(t, kb, "rev-parse", "--short", "HEAD"); head == gitOut(t, kb, "rev-parse", "--short", "origin/main") {
		t.Fatal("a diverged tree must never merge")
	}
}

func TestRunRefusesFeatureBranch(t *testing.T) {
	kb, bare := remoteKb(t)
	pushFrom(t, bare, "g.txt", "b\n", "feat: second")
	gitOut(t, kb, "checkout", "-q", "-b", "feature")
	if out, err := exec.Command("git", "-C", kb, "fetch", "origin").CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	markFetched(t, kb)
	res, err := Run(kb)
	if err != nil {
		t.Fatal(err)
	}
	if res.FastForwarded {
		t.Fatal("a feature branch must never be fast-forwarded")
	}
	if branch := gitOut(t, kb, "symbolic-ref", "--short", "HEAD"); branch != "feature" {
		t.Fatalf("HEAD must stay on the feature branch, got %s", branch)
	}
}

func TestFailedViewStuckRunning(t *testing.T) {
	old := time.Now().Add(-2 * StuckWindow)
	if at, failed := failedView(state{LastStart: old, LastExit: -1}, true); !failed {
		t.Fatal("a running marker older than the stuck window must count as failed")
	} else if at != old {
		t.Fatalf("the stuck failure time must be the start, got %v", at)
	}
	if _, failed := failedView(state{LastStart: time.Now(), LastExit: -1}, true); failed {
		t.Fatal("a fresh running marker is in flight, not failed")
	}
	if _, failed := failedView(state{LastStart: time.Now(), LastExit: 1, FailedFinish: time.Now()}, true); !failed {
		t.Fatal("a recorded failure must surface")
	}
}

func TestRunCorruptStateIsSafe(t *testing.T) {
	kb, _ := remoteKb(t)
	if err := os.WriteFile(filepath.Join(kb, ".git", "pylon-sync.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(kb); err != nil {
		t.Fatalf("corrupt state must degrade to a fresh launch, got %v", err)
	}
	// The launch rewrote the state file with valid JSON.
	if _, ok := readState(filepath.Join(kb, ".git")); !ok {
		t.Fatal("the launch must record valid state")
	}
}

func TestRunSkipsFetchWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("init: %v: %s", err, out)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.FetchStarted {
		t.Fatal("a repo with no remote must not report a fetch started")
	}
}
