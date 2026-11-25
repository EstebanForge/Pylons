// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// LintOptions controls corpus lint passes.
type LintOptions struct {
	HasCode       bool
	Fix           bool
	ChangedOnly   bool
	AllowExternal bool
	// Parity runs the opt-in declared-vs-code set checks on top of the
	// standard pass.
	Parity bool
}

var (
	mdLinkRe      = regexp.MustCompile(`\[[^\]]+\]\(([^)#\s]+)(?:#[^)]*)?\)`)
	inlineCodeRe  = regexp.MustCompile("`[^`]+`")
	machinePathRe = regexp.MustCompile(`(?:^|[\s(])(/(?:Users|home|var|etc|opt|tmp)/[^\s)]+|~/[^\s)]+|[A-Za-z]:\\[^\s)]+)`)
)

// isRootSkippedDir skips non-KB top-level directories without skipping nested folders of the same name.
// isNestedLedgerDir reports whether dirPath (a child of the ledger
// being walked) holds its own ledger manifest, making it a separate
// ledger: parent-ledger walks never enter it. The ledger root itself
// holds the manifest and is never a nested ledger.
func isNestedLedgerDir(kbRoot, dirPath string) bool {
	rel, err := filepath.Rel(kbRoot, dirPath)
	if err != nil || filepath.ToSlash(rel) == "." {
		return false
	}
	info, err := os.Stat(filepath.Join(dirPath, "manifests", "pylons.yaml"))
	return err == nil && info.Mode().IsRegular()
}

func isRootSkippedDir(kbRoot, dirPath string) bool {
	rel, err := filepath.Rel(kbRoot, dirPath)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.Contains(rel, "/") {
		return filepath.Base(dirPath) == ".git"
	}
	switch rel {
	case ".git", ".github", "cli", "bin", "templates", "manifests", "protocols", "extensions", "pylons-docs":
		return true
	default:
		return false
	}
}

// Lint runs deterministic checks across all markdown files in kbRoot.
func Lint(kbRoot string, opts LintOptions) []report.Finding {
	kbRoot = resolve.EvalRoot(kbRoot)
	cfg, err := config.Load(kbRoot)
	if err != nil {
		return []report.Finding{{
			RuleID:   "PYLON001",
			Severity: report.SeverityError,
			Grade:    report.GradeForced,
			File:     "manifests/pylons.yaml",
			Message:  "invalid manifests/pylons.yaml: " + err.Error(),
		}}
	}

	var changedSet map[string]bool
	if opts.ChangedOnly {
		changedSet = stagedMarkdownFiles(kbRoot)
	}

	var findings []report.Finding
	var docs []corpusDoc
	var rawDocs []corpusDoc
	var junk []string
	_ = filepath.WalkDir(kbRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isRootSkippedDir(kbRoot, p) || strings.HasPrefix(d.Name(), "_") || isNestedLedgerDir(kbRoot, p) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		// Excluded paths are outside the engine entirely: never
		// linted, never graph docs.
		excluded := false
		for _, pat := range cfg.Exclude {
			if resolve.MatchGlob(pat, rel) {
				excluded = true
				break
			}
		}
		if excluded {
			return nil
		}
		if strings.HasPrefix(d.Name(), "._") {
			junk = append(junk, rel)
			return nil
		}
		// lstat type: a doc symlink pointing outside the KB is never read.
		if !d.Type().IsRegular() {
			return nil
		}
		if changedSet != nil && !changedSet[rel] {
			return nil
		}
		// Root-level docs and prompts/skills skip strict frontmatter/STE scans.
		if !strings.Contains(rel, "/") || strings.HasPrefix(rel, "prompts/") || strings.HasPrefix(rel, "skills/") {
			return nil
		}

		content, readErr := readBounded(p)
		if readErr != nil {
			return nil
		}
		if opts.Fix {
			if moved := tryFixFolderMismatch(kbRoot, rel, content); moved != "" {
				rel = moved
			}
		}
		findings = append(findings, lintDoc(kbRoot, rel, content, cfg, opts)...)
		// Raw content rides along even when frontmatter fails strict
		// parsing: the secret scan reads bodies, not metadata.
		rawDocs = append(rawDocs, corpusDoc{rel: rel, content: content})
		var meta resolve.DocMeta
		if resolve.ParseDocMeta(content, &meta) {
			docs = append(docs, corpusDoc{rel: rel, content: content, meta: meta})
		}
		return nil
	})

	if len(junk) > 0 {
		covered := IgnoredByGit(kbRoot, junk)
		for _, j := range junk {
			if !covered[j] {
				findings = append(findings, errFinding("PYLON001", j, 1, "AppleDouble junk not covered by .gitignore; add ._ to .gitignore"))
			}
		}
	}

	if !opts.ChangedOnly {
		findings = append(findings, scanGraphRules(kbRoot, docs)...)
	}
	findings = append(findings, scanPlanAndQuirkRules(kbRoot, docs, cfg, opts)...)
	findings = append(findings, scanLifecycleRules(kbRoot, docs, cfg, opts.HasCode)...)
	findings = append(findings, scanMachineDeclarations(docs)...)
	findings = append(findings, scanSecrets(rawDocs)...)
	if opts.HasCode {
		findings = append(findings, lintImportBoundaries(kbRoot, cfg, changedSet)...)
	}
	if opts.Parity && opts.HasCode {
		findings = append(findings, lintParity(kbRoot, cfg, opts.AllowExternal)...)
	}
	return findings
}

// maxDocBytes caps one corpus read; a runaway doc cannot exhaust memory.
const maxDocBytes = 4 * 1024 * 1024

func readBounded(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxDocBytes))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stagedMarkdownFiles(kbRoot string) map[string]bool {
	out, err := resolve.GitRun(kbRoot, nil, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if strings.HasSuffix(line, ".md") {
			set[line] = true
		}
	}
	return set
}

func tryFixFolderMismatch(kbRoot, rel, content string) string {
	var meta resolve.DocMeta
	if !resolve.ParseDocMeta(content, &meta) {
		return ""
	}
	rootZone, _, _ := strings.Cut(rel, "/")
	switch {
	case rootZone == "plans" && (meta.Status == "shipped" || meta.Status == "abandoned"):
	case rootZone == "tickets" && (meta.Status == "done" || meta.Status == "wontfix"):
	default:
		return ""
	}
	return archiveToZone(kbRoot, rel, rootZone)
}

func archiveToZone(kbRoot, rel, rootZone string) string {
	if strings.Contains(rel, "/archive/") {
		return ""
	}
	dstRel := filepath.ToSlash(filepath.Join(rootZone, "archive", filepath.Base(rel)))
	src := filepath.Join(kbRoot, filepath.FromSlash(rel))
	dst := filepath.Join(kbRoot, filepath.FromSlash(dstRel))
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
			if err := os.Rename(src, dst); err == nil {
				return dstRel
			}
		}
	}
	return ""
}

func lintDoc(kbRoot, rel, content string, cfg config.Config, opts LintOptions) []report.Finding {
	var out []report.Finding
	var meta resolve.DocMeta
	if !resolve.ParseDocMeta(content, &meta) || strings.TrimSpace(meta.Title) == "" || strings.TrimSpace(meta.Type) == "" {
		msg := "missing or invalid frontmatter (requires title and type)"
		if len(meta.ParseErrors) > 0 {
			msg = "invalid frontmatter: " + meta.ParseErrors[0]
		}
		out = append(out, errFinding("PYLON001", rel, 1, msg))
		return out
	}
	if len(meta.Unknown) > 0 {
		out = append(out, advisoryFinding("PYLON064", rel, 1,
			"unknown frontmatter keys for type "+meta.Type+": "+strings.Join(meta.Unknown, ", ")))
	}

	// Zone entry guides (AGENTS.md / README.md) are reference docs, not zone lifecycle items.
	base := filepath.Base(rel)
	if base == "README.md" || base == "AGENTS.md" {
		return out
	}

	rootZone, _, _ := strings.Cut(rel, "/")
	if allowed, knownRoot := allowedStatuses[rootZone]; knownRoot {
		if !slices.Contains(allowed, meta.Status) {
			out = append(out, errFinding("PYLON008", rel, 1, "unknown status "+meta.Status+" for "+rootZone+"/"))
		} else if !statusMatchesFolder(rootZone, rel, meta.Status) {
			out = append(out, errFinding("PYLON007", rel, 1, "status "+meta.Status+" does not match folder "+rel))
		}
	}

	if rootZone == "plans" && meta.Status == "shipped" && !meta.Artifact.NonEmpty() {
		out = append(out, errFinding("PYLON009", rel, 1, "shipped plan must declare artifact:"))
	}

	if rootZone == "decisions" && meta.Status == "accepted" {
		prefix := cfg.DecisionPrefixOrDefault()
		nameRe := regexp.MustCompile(`^` + prefix + `\d{4}-.+\.md$`)
		if !nameRe.MatchString(filepath.Base(rel)) {
			out = append(out, errFinding("PYLON010", rel, 1, "accepted decision must be named "+prefix+"NNNN-slug.md"))
		}
	}

	body := resolve.WithoutFrontMatter(content)
	out = append(out, checkMarkdownLinks(cfg.Schemes, kbRoot, rel, body)...)
	out = append(out, checkMachinePaths(rel, body)...)
	out = append(out, checkSTE100(rel, body)...)

	if opts.HasCode {
		out = append(out, checkContracts(kbRoot, rel, content, cfg, opts.AllowExternal)...)
	}
	return out
}

var allowedStatuses = map[string][]string{
	"proposals":      {"proposed"},
	"plans":          {"proposed", "accepted", "in-progress", "paused", "shipped", "abandoned"},
	"conventions":    {"proposed", "accepted", "enforced", "superseded"},
	"decisions":      {"proposed", "accepted", "superseded", "deprecated"},
	"investigations": {"active", "resolved", "abandoned", "superseded", "withdrawn", "retired"},
	"packages":       {"active", "deprecated", "retired"},
	"references":     {"active", "superseded", "retired"},
	"quirks":         {"active"},
	"domain":         {"proposed", "accepted", "changed"},
	"tickets":        {"open", "in-progress", "blocked", "done", "wontfix"},
}

func statusMatchesFolder(rootZone, rel, status string) bool {
	inArchive := strings.Contains(rel, "/archive/")
	inProposals := rootZone == "proposals" || strings.Contains(rel, "/proposals/")
	switch rootZone {
	case "plans":
		switch status {
		case "proposed":
			return inProposals
		case "shipped", "abandoned":
			return inArchive
		default:
			return !inArchive && !inProposals
		}
	case "decisions":
		switch status {
		case "proposed":
			return inProposals
		case "superseded", "deprecated":
			return inArchive
		default:
			return !inArchive && !inProposals
		}
	case "conventions":
		switch status {
		case "proposed":
			return inProposals
		case "superseded":
			return inArchive
		default:
			return !inArchive && !inProposals
		}
	case "investigations":
		switch status {
		case "resolved", "abandoned", "superseded", "withdrawn", "retired":
			return inArchive
		default:
			return !inArchive
		}
	case "packages":
		if status == "retired" {
			return inArchive
		}
		return !inArchive
	case "tickets":
		if status == "done" || status == "wontfix" {
			return inArchive
		}
		return !inArchive
	default:
		return true
	}
}

func checkMarkdownLinks(schemes []string, kbRoot, rel, body string) []report.Finding {
	var out []report.Finding
	dir := filepath.Dir(filepath.Join(kbRoot, filepath.FromSlash(rel)))
	inFence := false
	for idx, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range mdLinkRe.FindAllStringSubmatch(line, -1) {
			target := m[1]
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if scheme, rest, ok := strings.Cut(target, "://"); ok {
				if !slices.Contains(schemes, scheme) {
					continue
				}
				// A declared scheme is recognized: the path must be
				// well formed, resolution semantics stay external.
				if !wellFormedURIPath(rest) {
					out = append(out, errFinding("PYLON012", rel, idx+1, "malformed "+scheme+" link: "+target))
				}
				continue
			}
			resolved := filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))
			if _, err := os.Stat(resolved); err != nil {
				out = append(out, errFinding("PYLON012", rel, idx+1, "broken internal link: "+target))
			}
		}
	}
	return out
}

// wellFormedURIPath judges the path half of scheme://path: non-empty,
// no leading slash, and no .. traversal segment.
func wellFormedURIPath(rest string) bool {
	if rest == "" || strings.HasPrefix(rest, "/") {
		return false
	}
	for _, seg := range strings.Split(rest, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func checkMachinePaths(rel, body string) []report.Finding {
	var out []report.Finding
	inFence := false
	for idx, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		stripped := inlineCodeRe.ReplaceAllString(line, " ")
		if m := machinePathRe.FindStringSubmatch(stripped); len(m) > 1 {
			out = append(out, errFinding("PYLON033", rel, idx+1, "raw machine path "+m[1]+" (use repo:// or pylon://)"))
		}
	}
	return out
}

func checkSTE100(rel, body string) []report.Finding {
	var out []report.Finding
	inFence := false
	for idx, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "|") || strings.HasPrefix(trim, "<!--") {
			continue
		}
		limit := 25
		if isProceduralLine(trim) {
			limit = 20
		}
		cleaned := inlineCodeRe.ReplaceAllString(trim, "code")
		for _, sentence := range strings.Split(cleaned, ". ") {
			words := strings.Fields(sentence)
			if len(words) > limit {
				head := words
				if len(head) > 8 {
					head = head[:8]
				}
				out = append(out, errFinding("PYLON006", rel, idx+1, "sentence exceeds "+strconv.Itoa(limit)+" words under ASD-STE100: "+strings.Join(head, " ")))
				break
			}
		}
	}
	return out
}

func isProceduralLine(trim string) bool {
	if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") {
		return true
	}
	dot := strings.Index(trim, ". ")
	if dot <= 0 || dot > 4 {
		return false
	}
	for _, r := range trim[:dot] {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func checkContracts(kbRoot, rel, content string, cfg config.Config, allowExternal bool) []report.Finding {
	var out []report.Finding
	extDir := filepath.Join(kbRoot, "extensions")

	var pf resolve.PlanFrontMatter
	if resolve.ParsePlanFrontMatter(content, &pf) && len(pf.Contract.Stable) > 0 {
		for _, entry := range pf.Contract.Stable {
			if isBoolTrue(entry["external"]) {
				continue
			}
			kindStr, _ := entry["kind"].(string)
			idStr, _ := entry["id"].(string)
			projAlias, projRoot, exts, ref, reason := resolveProjectTarget(kbRoot, idStr, pf.Components, cfg)
			if projRoot == "" {
				if reason != "" {
					out = append(out, errFinding("PYLON003", rel, 1, "unresolved contract "+kindStr+" "+idStr+": "+reason))
				}
				continue
			}
			_ = projAlias
			reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{
				Extensions:    exts,
				ExtDir:        extDir,
				Ref:           ref,
				AllowExternal: allowExternal,
			})
			v := reg.Resolve(projRoot, resolve.Kind(kindStr), idStr)
			if !v.Resolved {
				out = append(out, errFinding("PYLON003", rel, 1, "unresolved contract "+kindStr+" "+idStr+": "+v.Detail))
			}
		}
	}

	var pkg resolve.PackageFrontMatter
	if resolve.ParsePackageFrontMatter(content, &pkg) && pkg.Type == "package" && len(pkg.Exposes) > 0 {
		for _, entry := range pkg.Exposes {
			if isBoolTrue(entry["pending"]) || isBoolTrue(entry["external"]) {
				continue
			}
			kindStr, _ := entry["kind"].(string)
			idStr, _ := entry["id"].(string)
			_, projRoot, exts, ref, reason := resolveProjectTarget(kbRoot, idStr, []string{pkg.Package}, cfg)
			if projRoot == "" {
				if reason != "" {
					out = append(out, errFinding("PYLON003", rel, 1, "unresolved exposes "+kindStr+" "+idStr+": "+reason))
				}
				continue
			}
			if pkg.Ref != "" {
				ref = pkg.Ref
			}
			reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{
				Extensions:    exts,
				ExtDir:        extDir,
				Ref:           ref,
				AllowExternal: allowExternal,
			})
			v := reg.Resolve(projRoot, resolve.Kind(kindStr), idStr)
			if !v.Resolved {
				out = append(out, errFinding("PYLON003", rel, 1, "unresolved exposes "+kindStr+" "+idStr+": "+v.Detail))
			}
		}
	}
	return out
}

func resolveProjectTarget(kbRoot, id string, components []string, cfg config.Config) (alias, root string, exts []string, ref, reason string) {
	if strings.Contains(id, "://") {
		u, ok := resolve.ParseURI(id)
		if !ok || u.Scheme != "repo" {
			return "", "", nil, "", "invalid repo:// URI"
		}
		p := cfg.ProjectPath(kbRoot, u.Alias)
		if p == "" {
			return u.Alias, "", nil, "", "unregistered project alias " + u.Alias
		}
		if _, err := os.Stat(p); err != nil {
			return u.Alias, "", nil, "", "project checkout missing at " + p
		}
		return u.Alias, p, cfg.ProjectExtensions(u.Alias), cfg.ProjectRef(u.Alias), ""
	}
	for _, comp := range components {
		if p := cfg.ProjectPath(kbRoot, comp); p != "" {
			if _, err := os.Stat(p); err != nil {
				return comp, "", nil, "", "project checkout missing at " + p
			}
			return comp, p, cfg.ProjectExtensions(comp), cfg.ProjectRef(comp), ""
		}
	}
	// Unmatched contracts must not skip silently: a typo in components:
	// would otherwise disable the whole contract.
	if len(components) > 0 {
		return "", "", nil, "", "components do not map to a registered project"
	}
	return "", "", nil, "", "no project: use repo:// or set components:"
}

func isBoolTrue(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

// IgnoredByGit returns the subset of rels covered by the repository's
// ignore rules, per `git check-ignore`. Paths outside a git repository or
// with no matching rule are reported as not ignored.
func IgnoredByGit(kbRoot string, rels []string) map[string]bool {
	if len(rels) == 0 {
		return nil
	}
	out, err := resolve.GitRun(kbRoot, strings.NewReader(strings.Join(rels, "\n")), "check-ignore", "--stdin")
	ignored := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line != "" {
			ignored[line] = true
		}
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil
		}
	}
	return ignored
}

func advFinding(ruleID, file string, msg string) report.Finding {
	return report.Finding{
		RuleID:   ruleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     file,
		Line:     1,
		Message:  msg,
	}
}

func errFinding(ruleID, file string, line int, msg string) report.Finding {
	return report.Finding{
		RuleID:   ruleID,
		Severity: report.SeverityError,
		Grade:    report.GradeForced,
		File:     file,
		Line:     line,
		Message:  msg,
	}
}

// advisoryFinding mirrors errFinding at advisory grade.
func advisoryFinding(ruleID, file string, line int, msg string) report.Finding {
	return report.Finding{
		RuleID:   ruleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     file,
		Line:     line,
		Message:  msg,
	}
}
