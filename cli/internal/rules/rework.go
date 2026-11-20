// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// ErrPackageNotFound names a --package filter that matches no package
// document in packages/.
var ErrPackageNotFound = errors.New("package has no document in packages/")

// ReworkReport is the per-project rework metric for one window. It is
// report-only data; nothing gates on it.
type ReworkReport struct {
	Project        string `json:"project"`
	Window         string `json:"window"`
	Feat           int    `json:"feat"`
	Fix            int    `json:"fix"`
	Refactor       int    `json:"refactor"`
	ReworkFix      int    `json:"rework_fix"`
	ReworkRefactor int    `json:"rework_refactor"`
	HotPaths       int    `json:"hot_paths"`
}

// reworkWindowRe accepts "90d", "12w", "1y", or a bare day count.
var reworkWindowRe = regexp.MustCompile(`^(?:(\d+)d|(\d+)w|(\d+)y|(\d+))$`)

// parseReworkWindow converts a window period into days.
func parseReworkWindow(window string) (int, error) {
	w := strings.TrimSpace(window)
	if w == "" {
		return 90, nil
	}
	m := reworkWindowRe.FindStringSubmatch(w)
	if m == nil {
		return 0, fmt.Errorf("unparsable window %q (want 90d, 12w, 1y, or a day count)", window)
	}
	for i, mult := range []int{1, 7, 365} {
		if m[i+1] != "" {
			n, err := strconv.Atoi(m[i+1])
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("window %q must be a positive period", window)
			}
			days := n * mult
			if days > 36500 {
				return 0, fmt.Errorf("window %q exceeds 100 years", window)
			}
			return days, nil
		}
	}
	return 0, fmt.Errorf("window %q unparsable", window)
}

// reworkCommitRe classifies Conventional Commit subjects. feat! counts
// as feat; a scope is allowed; everything else is unclassified.
var reworkCommitRe = regexp.MustCompile(`^(feat|fix|refactor)(?:\([^)]*\))?!?: `)

// hubFiles are cross-cutting files a feat touches incidentally;
// counting them as hot paths would make almost every later fix a
// rework.
var hubFiles = map[string]bool{
	"CHANGELOG.md": true, "README.md": true, "go.mod": true, "go.sum": true,
	"package-lock.json": true, "pnpm-lock.yaml": true, "yarn.lock": true,
	"Cargo.lock": true, "composer.lock": true, "poetry.lock": true,
	".gitignore": true, "LICENSE": true, "NOTICE": true,
}

// isHubPath reports whether the final path segment is a hub file.
func isHubPath(p string) bool {
	return hubFiles[filepath.Base(p)]
}

// ValidateWindow reports whether the rework window parses.
func ValidateWindow(window string) error {
	_, err := parseReworkWindow(window)
	return err
}

// ReworkRate computes, per registered project, how many fix and
// refactor commits in the window touch paths a feat commit changed.
// Pure function of git history; report-only. Projects whose git log
// cannot run (missing checkout, unborn branch) are skipped with a
// stderr warning.
func ReworkRate(kbRoot, pkgFilter, window string) ([]ReworkReport, error) {
	if strings.TrimSpace(window) == "" {
		window = "90d"
	}
	days, err := parseReworkWindow(window)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return nil, err
	}
	aliases := slices.Sorted(maps.Keys(cfg.Projects))
	if pkgFilter != "" {
		components, err := packageComponents(kbRoot, pkgFilter)
		if err != nil {
			return nil, err
		}
		var filtered []string
		for _, a := range aliases {
			if slices.Contains(components, a) {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "rework: package %s registers no project components\n", pkgFilter)
		}
		aliases = filtered
	}

	var out []ReworkReport
	for _, alias := range aliases {
		projRoot := cfg.ProjectPath(kbRoot, alias)
		if projRoot == "" {
			continue
		}
		rep, err := reworkForProject(alias, projRoot, window, days)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rework: skipping %s: %v\n", alias, err)
			continue
		}
		out = append(out, rep)
	}
	return out, nil
}

// reworkForProject parses one project's git log into the report. git
// log lists newest first, so a commit's list index orders it in time:
// a fix reworks a path only when a feat older than it made the path
// hot.
func reworkForProject(alias, projRoot, window string, days int) (ReworkReport, error) {
	rep := ReworkReport{Project: alias, Window: window}
	// 0x01 starts each commit block: subject line, then --name-only paths.
	out, err := resolve.GitRun(projRoot, nil, "log", "--no-merges",
		"--since="+strconv.Itoa(days)+" days ago",
		"--pretty=format:%x01%s", "--name-only")
	if err != nil {
		return rep, fmt.Errorf("git log: %w", err)
	}
	hotSince := make(map[string]int)
	type commit struct {
		files []string
		idx   int
	}
	var fixCommits, refactorCommits []commit
	for i, record := range strings.Split(string(out), "\x01") {
		lines := strings.Split(strings.Trim(record, "\n"), "\n")
		if len(lines) == 0 || lines[0] == "" {
			continue
		}
		subject, paths := lines[0], lines[1:]
		m := reworkCommitRe.FindStringSubmatch(subject)
		if m == nil {
			continue
		}
		var files []string
		for _, line := range paths {
			line = strings.TrimSpace(line)
			if line != "" && !isHubPath(line) {
				files = append(files, line)
			}
		}
		switch m[1] {
		case "feat":
			rep.Feat++
			for _, f := range files {
				if prev, ok := hotSince[f]; !ok || i > prev {
					hotSince[f] = i
				}
			}
		case "fix":
			rep.Fix++
			fixCommits = append(fixCommits, commit{files: files, idx: i})
		case "refactor":
			rep.Refactor++
			refactorCommits = append(refactorCommits, commit{files: files, idx: i})
		}
	}
	rep.HotPaths = len(hotSince)
	for _, c := range fixCommits {
		for _, f := range c.files {
			if hot, ok := hotSince[f]; ok && hot > c.idx {
				rep.ReworkFix++
				break
			}
		}
	}
	for _, c := range refactorCommits {
		for _, f := range c.files {
			if hot, ok := hotSince[f]; ok && hot > c.idx {
				rep.ReworkRefactor++
				break
			}
		}
	}
	return rep, nil
}

// packageComponents resolves one package's registered project aliases.
func packageComponents(kbRoot, pkgFilter string) ([]string, error) {
	pkgDir := filepath.Join(kbRoot, "packages")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrPackageNotFound
		}
		return nil, err
	}
	var components []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(pkgDir, e.Name()))
		if err != nil {
			continue
		}
		var fm resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(string(raw), &fm) {
			continue
		}
		if fm.Package == pkgFilter || strings.TrimSuffix(e.Name(), ".md") == pkgFilter {
			components = append(components, fm.Components...)
		}
	}
	if len(components) == 0 {
		return nil, ErrPackageNotFound
	}
	return components, nil
}
