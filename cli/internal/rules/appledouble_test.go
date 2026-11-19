package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

func gitRepoKB(t *testing.T) string {
	t.Helper()
	// Isolate from global/system gitignore config: machine-level exclusions
	// must not decide test outcomes. core.excludesFile (XDG ignore) needs an
	// explicit override; GIT_CONFIG_* alone does not touch it.
	empty := filepath.Join(t.TempDir(), "empty-config")
	os.WriteFile(empty, nil, 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_SYSTEM", empty)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.excludesFile")
	t.Setenv("GIT_CONFIG_VALUE_0", empty)
	kb := t.TempDir()
	if out, err := exec.Command("git", "-C", kb, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return kb
}

func appleDoubleKB(t *testing.T) string {
	t.Helper()
	kb := gitRepoKB(t)
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects: {}\n")
	mustWrite(t, filepath.Join(kb, "tickets/AGENTS.md"), "---\ntitle: \"Tickets\"\ntype: reference\nstatus: active\n---\n\n# Tickets\n")
	mustWrite(t, filepath.Join(kb, "tickets/._AGENTS.md"), "\x00\x01\x02macOS AppleDouble junk\x00")
	mustWrite(t, filepath.Join(kb, "quirks/AGENTS.md"), "---\ntitle: \"Quirks\"\ntype: reference\nstatus: active\n---\n\n# Quirks\n")
	mustWrite(t, filepath.Join(kb, "quirks/._broken-trap.md"), "\x00\x01\x02junk\x00")
	return kb
}

func findingsOnAppleDouble(findings []report.Finding) []report.Finding {
	var hits []report.Finding
	for _, f := range findings {
		if strings.Contains(filepath.ToSlash(f.File), "/._") || strings.HasPrefix(filepath.ToSlash(f.File), "._") {
			hits = append(hits, f)
		}
	}
	return hits
}

func TestLintIgnoresAppleDoubleCoveredByGitignore(t *testing.T) {
	kb := appleDoubleKB(t)
	mustWrite(t, filepath.Join(kb, ".gitignore"), "._*\n")
	if findings := findingsOnAppleDouble(Lint(kb, LintOptions{})); len(findings) != 0 {
		t.Errorf("gitignored AppleDouble flagged: %+v", findings)
	}
}

func TestLintFlagsAppleDoubleWithoutGitignore(t *testing.T) {
	kb := appleDoubleKB(t)
	findings := findingsOnAppleDouble(Lint(kb, LintOptions{}))
	if len(findings) == 0 {
		t.Fatal("uncovered AppleDouble not flagged")
	}
	for _, f := range findings {
		if f.Grade != report.GradeForced {
			t.Errorf("grade = %v, want FORCED", f.Grade)
		}
		if !strings.Contains(f.Message, ".gitignore") {
			t.Errorf("message %q lacks .gitignore guidance", f.Message)
		}
	}
}

func TestPostmortemSkipsCoveredAppleDouble(t *testing.T) {
	kb := appleDoubleKB(t)
	mustWrite(t, filepath.Join(kb, ".gitignore"), "._*\n")
	if findings := findingsOnAppleDouble(ReviewPostmortem(kb, "", true)); len(findings) != 0 {
		t.Errorf("covered junk reviewed: %+v", findings)
	}
	if err := os.Remove(filepath.Join(kb, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if findings := findingsOnAppleDouble(ReviewPostmortem(kb, "", false)); len(findings) == 0 {
		t.Error("uncovered junk skipped by postmortem scan")
	}
}

func TestIgnoredByGit(t *testing.T) {
	kb := gitRepoKB(t)
	mustWrite(t, filepath.Join(kb, ".gitignore"), "._*\n")
	got := IgnoredByGit(kb, []string{"a/._x.md", "b/keep.md"})
	if !got["a/._x.md"] || got["b/keep.md"] {
		t.Errorf("IgnoredByGit = %v, want only a/._x.md", got)
	}
	if m := IgnoredByGit(kb, nil); m != nil {
		t.Errorf("empty input: got %v, want nil", m)
	}
	plain := t.TempDir()
	if m := IgnoredByGit(plain, []string{"._x.md"}); len(m) != 0 {
		t.Errorf("non-repo: got %v, want empty", m)
	}
}
