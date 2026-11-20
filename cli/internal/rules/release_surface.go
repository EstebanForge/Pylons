// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

import (
	"maps"
	"slices"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// releaseSurfaceRuleID backs the pinned-release-diff gate.
const releaseSurfaceRuleID = "PYLON048"

// ReleaseSurfaceOptions carries the pinned-releases directory and the
// optional explicit base that overrides pinning.
type ReleaseSurfaceOptions struct {
	PinnedDir string
	Base      string
}

// ReleaseSurface judges public surfaces against the versions pinned
// deployments actually run. Each deployment directory carries lockfiles
// (one `alias version` or `alias=version` line per registered project,
// `#` comments allowed); the pinned version resolves to a release tag in
// the project's repository, and a surface a deployment consumes may not
// be removed or changed between that tag and HEAD. A --base replaces
// the tag comparison; it must be explicit, never silent.
func ReleaseSurface(kbRoot string, opts ReleaseSurfaceOptions) ([]report.Finding, error) {
	var findings []report.Finding
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return nil, err
	}
	pins, errs := readPins(opts.PinnedDir, cfg)
	findings = append(findings, errs...)
	if len(pins) == 0 && len(errs) == 0 {
		return append(findings, notEvaluatedRelease(opts.PinnedDir)), nil
	}
	surfaces := publicContractSurfaces(kbRoot)
	aliases := slices.Sorted(maps.Keys(pins))
	evaluated := false
	for _, alias := range aliases {
		projPath := cfg.ProjectPath(kbRoot, alias)
		if projPath == "" {
			continue
		}
		names := make([]string, 0, len(surfaces[alias]))
		files := make(map[string]string, len(surfaces[alias]))
		for surface, kind := range surfaces[alias] {
			names = append(names, surfaceName(surface, kind))
			files[surface] = uriSurfacePath(surface)
		}
		for _, pin := range pins[alias] {
			base := opts.Base
			if base == "" {
				resolved, err := releaseTagRef(projPath, pin.version)
				if err != nil {
					findings = append(findings, report.Finding{
						RuleID:   releaseSurfaceRuleID,
						Severity: report.SeverityError,
						Grade:    report.GradeForced,
						File:     pin.file,
						Line:     pin.line,
						Message:  fmt.Sprintf("RELEASE_TAG_MISSING: pinned version %q resolves to no release tag in project %q; a skipped deployment must never read as parity", pin.version, alias),
					})
					continue
				}
				base = resolved
			}
			evaluated = true
			for surface, file := range files {
				atBase, baseOK := blobAt(projPath, base, file)
				atHead, headOK := blobAt(projPath, "HEAD", file)
				if !baseOK {
					continue
				}
				if !headOK {
					if consumed(opts.PinnedDir, names) {
						findings = append(findings, releaseFinding(pin, surface, "RELEASE_SURFACE_REMOVED", fmt.Sprintf("pinned deployment %q consumes %s, which exists at %s but is gone at HEAD", pin.version, surface, base)))
					}
					continue
				}
				if atBase != atHead && consumed(opts.PinnedDir, names) {
					findings = append(findings, releaseFinding(pin, surface, "RELEASE_SURFACE_CHANGED", fmt.Sprintf("pinned deployment %q consumes %s, whose content differs between %s and HEAD", pin.version, surface, base)))
				}
			}
		}
	}
	if !evaluated && len(findings) == 0 {
		return append(findings, notEvaluatedRelease(opts.PinnedDir)), nil
	}
	return findings, nil
}

func releaseFinding(pin pinRef, surface, code, message string) report.Finding {
	return report.Finding{
		RuleID:   releaseSurfaceRuleID,
		Severity: report.SeverityError,
		Grade:    report.GradeForced,
		File:     pin.file,
		Line:     pin.line,
		Message:  code + ": " + message,
	}
}

func notEvaluatedRelease(dir string) report.Finding {
	return report.Finding{
		RuleID:   releaseSurfaceRuleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     dir,
		Line:     1,
		Message:  "RELEASE_SURFACE_NOT_EVALUATED: no pin could be evaluated; the pinned surfaces are unjudged (missing directory, no lockfiles, or unreadable project)",
	}
}

// pinRef is one lockfile line with its provenance for finding anchors.
type pinRef struct {
	file    string
	line    int
	version string
}

// readPins parses every lockfile under the pinned-releases directory.
// Unknown aliases fail closed: a deployment naming an unregistered
// project cannot be judged.
func readPins(dir string, cfg config.Config) (map[string][]pinRef, []report.Finding) {
	var findings []report.Finding
	pins := map[string][]pinRef{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return pins, nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := readBounded(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "alias ")
			alias, version, ok := strings.Cut(line, "=")
			if !ok {
				alias, version, ok = cutFields(line)
			}
			if !ok {
				continue
			}
			alias = strings.TrimSpace(alias)
			version = strings.TrimSpace(version)
			if version == "" {
				continue
			}
			if _, registered := cfg.Projects[alias]; !registered {
				findings = append(findings, report.Finding{
					RuleID:   releaseSurfaceRuleID,
					Severity: report.SeverityError,
					Grade:    report.GradeForced,
					File:     path,
					Line:     i + 1,
					Message:  fmt.Sprintf("RELEASE_PIN_UNKNOWN: lockfile pins %q, which is not a registered project; the deployment cannot be judged", alias),
				})
				continue
			}
			pins[alias] = append(pins[alias], pinRef{file: path, line: i + 1, version: version})
		}
	}
	return pins, findings
}

// cutFields splits "alias version" on the first space.
func cutFields(line string) (string, string, bool) {
	alias, rest, ok := strings.Cut(line, " ")
	if !ok {
		return "", "", false
	}
	return alias, strings.TrimSpace(rest), strings.TrimSpace(rest) != ""
}

// releaseTagRef resolves a pinned version to a ref the project's
// repository can answer: the version itself, a v-prefixed tag, or a raw
// commit id.
func releaseTagRef(projPath, version string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	if len(version) == 40 {
		if _, err := resolve.GitRun(projPath, nil, "rev-parse", "--verify", version+"^{commit}"); err == nil {
			return version, nil
		}
	}
	for _, tag := range []string{version, "v" + version} {
		if _, err := resolve.GitRun(projPath, nil, "rev-parse", "--verify", tag+"^{commit}"); err == nil {
			return tag, nil
		}
	}
	return "", fmt.Errorf("no tag for %q", version)
}

// blobAt reads the blob id of one project-relative file at a ref. A
// missing file reports ok=false rather than an error: absence is the
// removed-surface signal, not a broken gate.
func blobAt(projPath, ref, file string) (string, bool) {
	out, err := resolve.GitRun(projPath, nil, "rev-parse", ref+":"+file)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// consumed reports whether the deployment directory references one of
// the surfaces: a file naming the surface, or a vendored copy of the
// surface's own file. The deployment runs what its tree carries.
func consumed(dir string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	found := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if slices.Contains(names, filepath.Base(path)) {
			found = true
			return fs.SkipAll
		}
		data, err := readBounded(path)
		if err != nil {
			return nil
		}
		for _, name := range names {
			if name != "" && strings.Contains(string(data), name) {
				found = true
				return fs.SkipAll
			}
		}
		return nil
	})
	return found
}

// surfaceName extracts the name consumers reference: the ::fragment for
// symbols, the file base name otherwise.
func surfaceName(surface, kind string) string {
	if kind == "symbol" {
		if _, name, ok := strings.Cut(surface, "::"); ok {
			return name
		}
	}
	base := uriSurfacePath(surface)
	if i := strings.LastIndex(base, "/"); i >= 0 {
		return base[i+1:]
	}
	return base
}

// publicContractSurfaces collects the symbol, route, and cli contract
// surfaces of accepted and in-progress plans, keyed by project alias.
func publicContractSurfaces(kbRoot string) map[string]map[string]string {
	out := map[string]map[string]string{}
	_ = filepath.WalkDir(filepath.Join(kbRoot, "plans"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := d.Name()
		if name == "README.md" || name == "AGENTS.md" {
			return nil
		}
		data, err := readManifest(path)
		if err != nil {
			return nil
		}
		var pf resolve.PlanFrontMatter
		if !resolve.ParsePlanFrontMatter(string(data), &pf) || pf.Type != "plan" {
			return nil
		}
		if pf.Status != "accepted" && pf.Status != "in-progress" {
			return nil
		}
		for _, entry := range pf.Contract.Stable {
			kindStr, _ := entry["kind"].(string)
			kind := kindStr
			if i := strings.LastIndex(kindStr, "-"); i >= 0 {
				kind = kindStr[i+1:]
			}
			if kind != "symbol" && kind != "route" && kind != "cli" {
				continue
			}
			idStr, _ := entry["id"].(string)
			u, ok := resolve.ParseURI(idStr)
			if !ok || u.Scheme != "repo" || u.Path == "" {
				continue
			}
			if out[u.Alias] == nil {
				out[u.Alias] = map[string]string{}
			}
			out[u.Alias][u.Path] = kind
		}
		return nil
	})
	return out
}
