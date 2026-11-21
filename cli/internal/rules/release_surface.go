// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// releaseSurfaceRuleID backs the pinned-release-diff gate.
const releaseSurfaceRuleID = "PYLON048"

// ReleaseSurfaceOptions carries the pinned-releases directory and the
// optional explicit base that overrides pinning.
type ReleaseSurfaceOptions struct {
	PinnedDir string
	Base      string
}

// deploymentIndex is the consumption evidence for one deployment
// directory, walked once per run: vendored file base names plus bounded
// text contents for word-boundary matching.
type deploymentIndex struct {
	names    map[string]bool
	contents []string
}

// deploymentSkipDirs are dependency and VCS trees the walk never
// reads. vendor/ is deliberately absent: vendored copies are exactly
// the consumption evidence the gate exists to find.
var deploymentSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true,
	"target": true, "__pycache__": true, ".venv": true, "venv": true,
	"build": true, "bin": true, "obj": true,
}

// deploymentByteBudget caps how much text one deployment dir may
// contribute to the word-boundary index.
const deploymentByteBudget = 64 << 20

// ReleaseSurface judges public surfaces against the versions pinned
// deployments actually run. Each deployment directory carries lockfiles
// (one `alias version` or `alias=version` line per registered project,
// `#` comments allowed); the pinned version resolves to a release tag in
// the project's repository, and a surface the deployment consumes may
// not be removed or changed between that tag and HEAD. A --base
// replaces the tag comparison; it must be explicit, never silent.
func ReleaseSurface(kbRoot string, opts ReleaseSurfaceOptions) ([]report.Finding, error) {
	if strings.HasPrefix(opts.Base, "-") {
		return nil, fmt.Errorf("--base %q is not a ref", opts.Base)
	}
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
	index := buildDeploymentIndex(opts.PinnedDir)
	surfaces := publicContractSurfaces(kbRoot)
	aliases := slices.Sorted(maps.Keys(pins))
	evaluated := false
	for _, alias := range aliases {
		projPath := cfg.ProjectPath(kbRoot, alias)
		if projPath == "" {
			continue
		}
		files := surfaceFiles(surfaces[alias])
		if len(files) == 0 {
			continue
		}
		healthy := gitHealthy(projPath)
		for _, pin := range pins[alias] {
			base := opts.Base
			if base == "" {
				resolved, err := releaseTagRef(projPath, pin.version)
				if err != nil {
					findings = append(findings, releaseFinding(pin, "", "RELEASE_TAG_MISSING",
						fmt.Sprintf("pinned version %q resolves to no release tag in project %q; a skipped deployment must never read as parity", pin.version, alias)))
					continue
				}
				base = resolved
			}
			if !healthy {
				findings = append(findings, releaseFinding(pin, "", "RELEASE_SURFACE_NOT_EVALUATED",
					fmt.Sprintf("project %q's repository is unreadable; its pinned surfaces are unjudged", alias)))
				continue
			}
			for _, surface := range slices.Sorted(maps.Keys(files)) {
				file := files[surface]
				atBase, baseOK := blobAt(projPath, base, file)
				if !baseOK {
					continue
				}
				evaluated = true
				atHead, headOK := blobAt(projPath, "HEAD", file)
				if !headOK {
					if index.consumes(surfaceName(surface, surKind(surfaces, alias, surface))) {
						findings = append(findings, releaseFinding(pin, surface, "RELEASE_SURFACE_REMOVED",
							fmt.Sprintf("pinned deployment consumes %s, which exists at %s but is gone at HEAD", surface, base)))
					}
					continue
				}
				if atBase != atHead && index.consumes(surfaceName(surface, surKind(surfaces, alias, surface))) {
					findings = append(findings, releaseFinding(pin, surface, "RELEASE_SURFACE_CHANGED",
						fmt.Sprintf("pinned deployment consumes %s, whose content differs between %s and HEAD", surface, base)))
				}
			}
		}
	}
	if !evaluated && len(findings) == 0 {
		return append(findings, notEvaluatedRelease(opts.PinnedDir)), nil
	}
	return findings, nil
}

// surKind looks up the base kind of one surface for name extraction.
func surKind(surfaces map[string]map[string]string, alias, surface string) string {
	byAlias := surfaces[alias]
	if byAlias == nil {
		return ""
	}
	return byAlias[surface]
}

// surfaceFiles reduces the alias's surfaces to one entry per file: two
// symbols in one file drift together and must not double-report.
func surfaceFiles(byAlias map[string]string) map[string]string {
	files := map[string]string{}
	for surface := range byAlias {
		files[uriSurfacePath(surface)] = surface
	}
	// Keep one representative surface per file for the finding text.
	out := map[string]string{}
	for file, surface := range files {
		out[surface] = file
	}
	return out
}

func releaseFinding(pin pinRef, surface, code, message string) report.Finding {
	if surface != "" {
		message = code + ": " + message + " (" + surface + ")"
	} else {
		message = code + ": " + message
	}
	return report.Finding{
		RuleID:   releaseSurfaceRuleID,
		Severity: report.SeverityError,
		Grade:    report.GradeForced,
		File:     pin.file,
		Line:     pin.line,
		Message:  message,
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
// Unknown aliases and unparsable lines fail closed: a deployment whose
// pins cannot be read must never read as parity.
func readPins(dir string, cfg config.Config) (map[string][]pinRef, []report.Finding) {
	var findings []report.Finding
	pins := map[string][]pinRef{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return pins, nil
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := readManifest(path)
		if err != nil {
			findings = append(findings, report.Finding{
				RuleID:   releaseSurfaceRuleID,
				Severity: report.SeverityError,
				Grade:    report.GradeForced,
				File:     path,
				Line:     1,
				Message:  "RELEASE_PIN_MALFORMED: the lockfile cannot be read; the deployment cannot be judged",
			})
			continue
		}
		findings = append(findings, parseLockfile(path, string(data), cfg, pins)...)
	}
	return pins, findings
}

// parseLockfile reads one lockfile's pin lines into pins, failing closed
// on lines it cannot parse.
func parseLockfile(path, data string, cfg config.Config, pins map[string][]pinRef) []report.Finding {
	var findings []report.Finding
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		alias, version, ok := cutFields(line)
		if !ok {
			alias, version, ok = strings.Cut(line, "=")
		}
		if !ok {
			findings = append(findings, report.Finding{
				RuleID:   releaseSurfaceRuleID,
				Severity: report.SeverityError,
				Grade:    report.GradeForced,
				File:     path,
				Line:     i + 1,
				Message:  fmt.Sprintf("RELEASE_PIN_MALFORMED: pin line %q is not `alias version` or `alias=version`", line),
			})
			continue
		}
		alias = strings.TrimSpace(alias)
		version = strings.TrimSpace(version)
		if alias == "" || version == "" {
			findings = append(findings, report.Finding{
				RuleID:   releaseSurfaceRuleID,
				Severity: report.SeverityError,
				Grade:    report.GradeForced,
				File:     path,
				Line:     i + 1,
				Message:  fmt.Sprintf("RELEASE_PIN_MALFORMED: pin line %q names an empty alias or version", line),
			})
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
	return findings
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
// repository can answer. Tags are addressed under refs/tags so a
// same-named branch cannot shadow them; a 40-hex version is tried as a
// raw commit id. A leading dash is a lockfile injection attempt and
// fails closed without reaching git as an option.
func releaseTagRef(projPath, version string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	if version == "" || strings.HasPrefix(version, "-") {
		return "", fmt.Errorf("invalid pinned version %q", version)
	}
	if len(version) == 40 {
		if _, err := resolve.GitRun(projPath, nil, "rev-parse", "--verify", version+"^{commit}"); err == nil {
			return version, nil
		}
	}
	for _, tag := range []string{version, "v" + version} {
		if _, err := resolve.GitRun(projPath, nil, "rev-parse", "--verify", "refs/tags/"+tag+"^{commit}"); err == nil {
			return tag, nil
		}
	}
	return "", fmt.Errorf("no tag for %q", version)
}

// gitHealthy proves the project repository answers git at all, so a
// broken checkout never reads as a missing file.
func gitHealthy(projPath string) bool {
	_, err := resolve.GitRun(projPath, nil, "rev-parse", "--git-dir")
	return err == nil
}

// blobAt reads the blob id of one project-relative file at a ref. The
// ./ prefix resolves the path from the project directory, so projects
// nested under their repository root are judged the same as root
// projects. A missing file reports ok=false rather than an error:
// absence is the removed-surface signal, not a broken gate.
func blobAt(projPath, ref, file string) (string, bool) {
	out, err := resolve.GitRun(projPath, nil, "rev-parse", "--verify", ref+":./"+file)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// buildDeploymentIndex walks the deployment directory once. Non-regular
// entries are never opened, dependency and VCS trees are skipped, and
// the text budget bounds the index; the base-name set is unbounded.
func buildDeploymentIndex(dir string) *deploymentIndex {
	idx := &deploymentIndex{names: map[string]bool{}}
	if dir == "" {
		return idx
	}
	budget := deploymentByteBudget
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && (deploymentSkipDirs[d.Name()] || strings.HasPrefix(d.Name(), "_")) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		idx.names[filepath.Base(path)] = true
		if budget <= 0 {
			return nil
		}
		data, err := readManifest(path)
		if err != nil {
			return nil
		}
		if len(data) > budget {
			budget = 0
			return nil
		}
		budget -= len(data)
		idx.contents = append(idx.contents, string(data))
		return nil
	})
	return idx
}

// consumes reports whether the deployment references the surface name:
// a vendored file with the surface's base name, or a word-boundary
// mention in the indexed text. Substring matching would hit Running for
// Run, so names match on identifier boundaries.
func (idx *deploymentIndex) consumes(name string) bool {
	if name == "" {
		return false
	}
	if idx.names[name] {
		return true
	}
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		return false
	}
	for _, content := range idx.contents {
		if re.MatchString(content) {
			return true
		}
	}
	return false
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
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
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
			kind := kindOr(kindStr)
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
