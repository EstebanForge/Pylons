// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/install"
	"github.com/estebanforge/pylons/cli/internal/protocols"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
	"github.com/estebanforge/pylons/cli/internal/rules"
)

const usage = `pylon validates and queries a Pylons knowledge base.

Usage:
  pylon lint             [--changed] [--doc-only] [--fix] [--allow-external-extensions] [--format=human|json|sarif] [--json] [--ci] [--pylons <path>]
  pylon contract <plan>  [--allow-external-extensions] [--format=human|json|sarif] [--json] [--ci] [--pylons <path>]
  pylon check <paths...> [--stop-block] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon brief            [--format=human|json] [--json] [--pylons <path>]
  pylon context <paths...> [--preamble] [--format=human|json] [--json] [--pylons <path>]
  pylon staleness        [--package <pkg>] [--fix] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review plan <path> [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review postmortem [path] [--check-sources] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review staleness [--package <pkg>] [--fix] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review-brief <plan> [--stdout] [--base <ref>] [--validate <file> | --validate-resolutions <file>] [--format=human|json] [--pylons <path>]
  pylon protocol [list|<name>] [--template <path>] [--pylons <path>]
  pylon reflect          [--template <path>] [--pylons <path>]
  pylon extensions [list|schema] [--format=human|json] [--json] [--pylons <path>]
  pylon map              [--type <t>] [--package <p>] [--tag <t>] [--status <s>] [--folder <dir>] [--query <q>] [--limit <n>] [--all] [--format=human|json] [--json] [--pylons <path>]
  pylon search <query>   [--limit <n>] [--all] [--format=human|json] [--json] [--pylons <path>]
  pylon manifest [rules|packages] [--check] [--pylons <path>]
  pylon explain <rule_id>
  pylon install         [--pylons <path>]
  pylon init [dir]

Exit codes:
  0 Clean or advisory-only
  1 Blocking findings present
  2 Usage error
  3 Cannot locate Pylons root`

const aiurArt = `               /\
              /  \
             / /\ \
            / /  \ \
           / / /\ \ \
          / / /  \ \ \
         / / / /\ \ \ \
        / / / /  \ \ \ \
       / / /  \  /  \ \ \
      / /  \   \/   /  \ \
     /  \   \  /\  /   /  \
    /    \   \/  \/   /    \
   /  /\  \  /    \  /  /\  \
  /  /  \  \/      \/  /  \  \
 /  /    \            /    \  \
<  <      >  ( () )  <      >  >
 \  \    /            \    /  /
  \  \  /  /\      /\  \  /  /
   \  \/  /  \    /  \  \/  /
    \    /   /\  /\   \    /
     \  /   /  \/  \   \  /
      \ \  /   /\   \  / /
       \ \ \  /  \  / / /
        \ \ \ \  / / / /
         \ \ \ \/ / / /
          \ \ \  / / /
           \ \ \/ / /
            \ \  / /
             \ \/ /
              \  /
               \/
            .------.
         .-'        '-.
       .'  .--------.  '.
      /  .'          '.  \
     |  /              \  |
     | |   [==]  [==]   | |
     |  \              /  |
      \  '.          .'  /
       '.  '--------'  .'
         '-.________.-'
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	format := report.FormatHuman
	docOnly, changed, fix, ci := false, false, false, false
	checkManifest, stopBlock, preamble, checkSources := false, false, false, false
	rbStdout, mapAll := false, false
	allowExternal := os.Getenv("PYLON_ALLOW_EXTERNAL_EXTENSIONS") == "1"
	pylonsPath, pkgArg, diffBase, rbValidate, rbValidateRes, protoTmpl := "", "", "", "", "", ""
	mapType, mapTag, mapStatus, mapFolder, mapQuery := "", "", "", "", ""
	limit := 0
	var rest []string
	var cmd string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprintln(os.Stderr, usage)
			return 0
		case arg == "--aiur":
			fmt.Print(aiurArt)
			return 0
		case arg == "--json":
			format = report.FormatJSON
		case arg == "--doc-only":
			docOnly = true
		case arg == "--changed":
			changed = true
		case arg == "--fix":
			fix = true
		case arg == "--ci":
			ci = true
		case arg == "--check":
			checkManifest = true
		case arg == "--stop-block":
			stopBlock = true
		case arg == "--preamble":
			preamble = true
		case arg == "--check-sources":
			checkSources = true
		case arg == "--stdout":
			rbStdout = true
		case arg == "--all":
			mapAll = true
		case arg == "--allow-external-extensions":
			allowExternal = true
		case strings.HasPrefix(arg, "--format="):
			format = report.Format(strings.TrimPrefix(arg, "--format="))
		case arg == "--format" || arg == "--pylons" || arg == "--package" || arg == "--base" || arg == "--validate" || arg == "--validate-resolutions" || arg == "--template" || arg == "--type" || arg == "--tag" || arg == "--status" || arg == "--folder" || arg == "--query" || arg == "--limit":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "pylon: flag %s requires a value\n", arg)
				return 2
			}
			i++
			val := args[i]
			switch arg {
			case "--format":
				format = report.Format(val)
			case "--pylons":
				pylonsPath = val
			case "--package":
				pkgArg = val
			case "--base":
				diffBase = val
			case "--validate":
				rbValidate = val
			case "--validate-resolutions":
				rbValidateRes = val
			case "--template":
				protoTmpl = val
			case "--type":
				mapType = val
			case "--tag":
				mapTag = val
			case "--status":
				mapStatus = val
			case "--folder":
				mapFolder = val
			case "--query":
				mapQuery = val
			case "--limit":
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					fmt.Fprintln(os.Stderr, "pylon: --limit must be a non-negative integer")
					return 2
				}
				limit = n
			}
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(os.Stderr, "pylon: unknown flag %s\n", arg)
			return 2
		default:
			if cmd == "" {
				cmd = arg
			} else {
				rest = append(rest, arg)
			}
		}
	}

	if !report.ValidateFormat(format) {
		fmt.Fprintf(os.Stderr, "pylon: unknown format %q (expected human, json, or sarif)\n", format)
		return 2
	}

	if cmd == "explain" {
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "pylon: explain requires <rule_id>")
			return 2
		}
		spec, ok := rules.Catalog[strings.ToUpper(rest[0])]
		if !ok {
			fmt.Fprintf(os.Stderr, "pylon: unknown rule %s\n", rest[0])
			return 2
		}
		fmt.Printf("%s (%s, %s)\n%s\nFix: %s\n", spec.ID, spec.Name, spec.Severity, spec.Description, spec.FixHint)
		return 0
	}

	if cmd == "init" {
		dir := "."
		if len(rest) > 0 {
			dir = rest[0]
		}
		return cmdInit(dir)
	}

	if cmd == "install" {
		return cmdInstall(findPylonsRoot(pylonsPath))
	}

	kbRoot := findPylonsRoot(pylonsPath)
	if kbRoot == "" {
		if cmd == "brief" {
			fmt.Println("pylon brief: no manifests/pylons.yaml found")
			return 0
		}
		fmt.Fprintln(os.Stderr, "pylon: could not locate manifests/pylons.yaml")
		return 3
	}

	switch cmd {
	case "lint":
		findings := rules.Lint(kbRoot, rules.LintOptions{
			HasCode:       !docOnly,
			Fix:           fix,
			ChangedOnly:   changed,
			AllowExternal: allowExternal,
		})
		return renderFindings(findings, format, ci)

	case "contract":
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "pylon: contract requires <plan>")
			return 2
		}
		findings, err := rules.CheckPlanContract(kbRoot, rest[0], allowExternal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: contract: %v\n", err)
			return 2
		}
		return renderFindings(findings, format, ci)

	case "check":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: check requires <paths...>")
			return 2
		}
		findings := rules.Check(kbRoot, rest)
		if stopBlock && format == report.FormatHuman {
			if err := report.RenderStopBlocks(os.Stdout, findings); err != nil {
				return 2
			}
		} else if err := report.Render(os.Stdout, format, findings); err != nil {
			return 2
		}
		if report.HasBlocking(findings) {
			return 1
		}
		return 0

	case "context":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: context requires <paths...>")
			return 2
		}
		if preamble && format == report.FormatHuman {
			cfg := loadConfigOrWarn(kbRoot)
			fmt.Printf("=== Pylons Context (%s | projects: %d | extensions: %v) ===\n", kbRoot, len(cfg.Projects), cfg.Extensions)
		}
		findings := rules.Check(kbRoot, rest)
		if err := report.Render(os.Stdout, format, findings); err != nil {
			return 2
		}
		return 0

	case "staleness":
		findings := rules.CheckStaleness(kbRoot, pkgArg, fix)
		return renderFindings(findings, format, ci)

	case "review":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: review requires subcommand: plan | postmortem | staleness")
			return 2
		}
		switch rest[0] {
		case "plan":
			if len(rest) != 2 {
				fmt.Fprintln(os.Stderr, "pylon: review plan requires <path>")
				return 2
			}
			return renderFindings(rules.ReviewPlan(kbRoot, rest[1]), format, ci)
		case "postmortem":
			target := ""
			if len(rest) > 1 {
				target = rest[1]
			}
			return renderFindings(rules.ReviewPostmortem(kbRoot, target, checkSources), format, ci)
		case "staleness":
			return renderFindings(rules.CheckStaleness(kbRoot, pkgArg, fix), format, ci)
		default:
			fmt.Fprintf(os.Stderr, "pylon: unknown review subcommand %s\n", rest[0])
			return 2
		}

	case "review-brief":
		planArg := ""
		if len(rest) > 0 {
			planArg = rest[0]
		}
		return runReviewBrief(kbRoot, planArg, diffBase, rbStdout, rbValidate, rbValidateRes, format)

	case "protocol":
		if len(rest) == 0 || rest[0] == "list" {
			for _, name := range protocols.List(kbRoot) {
				fmt.Println(name)
			}
			return 0
		}
		return cmdProtocol(kbRoot, rest[0], protoTmpl)

	case "reflect":
		return cmdProtocol(kbRoot, "reflect", protoTmpl)

	case "extensions":
		sub := "list"
		if len(rest) > 0 {
			sub = rest[0]
		}
		return cmdExtensions(kbRoot, sub, format)

	case "brief":
		cfg := loadConfigOrWarn(kbRoot)
		docs := scanDocs(kbRoot, false)
		if format == report.FormatJSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"root":       kbRoot,
				"projects":   len(cfg.Projects),
				"extensions": cfg.Extensions,
				"documents":  len(docs),
			})
			return 0
		}
		fmt.Printf("Pylons: %s | projects: %d | extensions: %d | docs: %d\n", kbRoot, len(cfg.Projects), len(cfg.Extensions), len(docs))
		return 0

	case "map":
		docs := filterDocs(scanDocs(kbRoot, mapAll), mapType, pkgArg, mapTag, mapStatus, mapFolder, mapQuery, limit)
		return renderDocs(docs, format)

	case "search":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: search requires <query>")
			return 2
		}
		docs := filterDocs(scanDocs(kbRoot, mapAll), "", "", "", "", "", strings.Join(rest, " "), limit)
		return renderDocs(docs, format)

	case "manifest":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: manifest requires 'rules' or 'packages'")
			return 2
		}
		switch rest[0] {
		case "rules":
			return cmdManifestJSON(filepath.Join(kbRoot, "manifests", "rules.json"), rules.AllRules(), checkManifest)
		case "packages":
			return cmdManifestJSON(filepath.Join(kbRoot, "manifests", "packages.json"), rules.BuildPackagesManifest(kbRoot), checkManifest)
		default:
			fmt.Fprintln(os.Stderr, "pylon: manifest supports 'rules' or 'packages'")
			return 2
		}

	default:
		fmt.Fprintf(os.Stderr, "pylon: unknown command %s\n", cmd)
		return 2
	}
}

func renderFindings(findings []report.Finding, format report.Format, ci bool) int {
	if err := report.Render(os.Stdout, format, findings); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: render failed: %v\n", err)
		return 2
	}
	if report.HasBlocking(findings) || (ci && len(findings) > 0) {
		return 1
	}
	return 0
}

var scaffoldDirs = []string{"packages", "packages/archive", "proposals", "plans/archive", "decisions/proposals", "decisions/archive", "conventions", "conventions/proposals", "conventions/archive", "quirks/archive", "flows/archive", "domain/entities", "domain/archive", "investigations/archive", "audits/archive", "references/archive", "tickets/archive", "prompts", "skills", "templates", "manifests", "protocols", "extensions"}

func cmdInit(dir string) int {
	cfgPath := filepath.Join(dir, "manifests", "pylons.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("pylon init: %s already exists\n", cfgPath)
		return 0
	}
	for _, sub := range scaffoldDirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "pylon init: %v\n", err)
			return 1
		}
	}
	if err := os.WriteFile(cfgPath, []byte("version: 1\nextensions: []\nprojects: {}\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "pylon init: %v\n", err)
		return 1
	}
	seedFiles := map[string]string{
		"AGENTS.md":                 "# Guidelines\n\nPylons knowledge base. See `conventions/vocabulary.md`.\n",
		"conventions/AGENTS.md":     "---\ntitle: \"Conventions\"\ntype: reference\nstatus: active\n---\n\n# Conventions\n",
		"conventions/vocabulary.md": "---\ntitle: \"Vocabulary\"\ntype: convention\nstatus: accepted\n---\n\n# Vocabulary\n\nClosed frontmatter status values.\n",
	}
	for rel, body := range seedFiles {
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			_ = os.WriteFile(dst, []byte(body), 0o644)
		}
	}
	giPath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		_ = os.WriteFile(giPath, []byte("._*\n.DS_Store\n/bin/\n"), 0o644)
	}
	for _, sub := range scaffoldDirs {
		path := filepath.Join(dir, sub)
		entries, err := os.ReadDir(path)
		if err == nil && len(entries) == 0 {
			_ = os.WriteFile(filepath.Join(path, ".gitkeep"), nil, 0o644)
		}
	}
	if res, err := install.EnsureHooks(dir); err != nil {
		fmt.Printf("pylon init: not a git repository; run pylon install after git init\n")
	} else if len(res.WroteHooks) > 0 {
		fmt.Printf("pylon init: wired gate hooks (core.hooksPath .githooks)\n")
	}
	fmt.Printf("pylon init: initialized %s\n", dir)
	return 0
}

func cmdInstall(kbRoot string) int {
	if kbRoot == "" {
		fmt.Fprintln(os.Stderr, "pylon: could not locate manifests/pylons.yaml")
		return 3
	}
	res, err := install.EnsureHooks(kbRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon install: %v\n", err)
		return 1
	}
	for _, name := range res.WroteHooks {
		fmt.Printf("pylon install: wrote .githooks/%s\n", name)
	}
	for _, name := range res.SkippedHooks {
		fmt.Printf("pylon install: kept existing .githooks/%s\n", name)
	}
	if res.HooksPathSet {
		fmt.Printf("pylon install: enabled core.hooksPath .githooks\n")
	} else {
		fmt.Printf("pylon install: core.hooksPath already set\n")
	}
	if !res.BinaryOK {
		fmt.Fprintf(os.Stderr, "pylon install: bin/pylon missing or not executable; build it: cd cli && go build -o ../bin/pylon ./cmd/pylon\n")
		return 1
	}
	return 0
}

func cmdManifestJSON(outPath string, payload any, checkOnly bool) int {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return 2
	}
	data = append(data, '\n')
	if checkOnly {
		existing, err := os.ReadFile(outPath)
		if err != nil || string(existing) != string(data) {
			fmt.Fprintf(os.Stderr, "pylon: %s is out of date\n", outPath)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 1
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return 1
	}
	fmt.Printf("wrote %s\n", outPath)
	return 0
}

func cmdProtocol(kbRoot, name, overrideFlag string) int {
	cfg := loadConfigOrWarn(kbRoot)
	override := overrideFlag
	if override == "" && cfg.Protocols != nil {
		if rel, ok := cfg.Protocols[name]; ok && rel != "" {
			if filepath.IsAbs(rel) {
				override = rel
			} else {
				override = filepath.Join(kbRoot, rel)
			}
		}
	}

	projects := make([]protocols.ProjectInfo, 0, len(cfg.Projects))
	for alias, proj := range cfg.Projects {
		ref := proj.Ref
		if ref == "" {
			ref = "main"
		}
		projects = append(projects, protocols.ProjectInfo{
			Alias: alias,
			Ref:   ref,
			Tier:  proj.Tier,
		})
	}
	slices.SortFunc(projects, func(a, b protocols.ProjectInfo) int { return strings.Compare(a.Alias, b.Alias) })

	bin := "./bin/pylon"
	if exe, err := os.Executable(); err == nil && exe != "" {
		bin = exe
	}
	out, err := protocols.Render(name, protocols.Data{
		Root:       kbRoot,
		Bin:        bin,
		Extensions: cfg.Extensions,
		Projects:   projects,
	}, override)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
		return 2
	}
	fmt.Print(out)
	return 0
}

func cmdExtensions(kbRoot, sub string, format report.Format) int {
	switch sub {
	case "schema":
		p := filepath.Join(kbRoot, "manifests", "extensions.json")
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: cannot read %s: %v\n", p, err)
			return 1
		}
		fmt.Print(string(data))
		return 0
	case "list":
		cfg := loadConfigOrWarn(kbRoot)
		extDir := filepath.Join(kbRoot, "extensions")
		items := resolve.ListExtensions(cfg.Extensions, extDir)
		if format == report.FormatJSON {
			if err := json.NewEncoder(os.Stdout).Encode(items); err != nil {
				return 2
			}
			return 0
		}
		for _, item := range items {
			state := "disabled"
			if item.Enabled {
				state = "enabled"
			}
			fmt.Printf("%s\t[%s/%s]\tkinds: %s\n", item.Name, item.Source, state, strings.Join(item.Kinds, ","))
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "pylon: extensions supports 'list' or 'schema'\n")
		return 2
	}
}

func findPylonsRoot(explicit string) string {
	if explicit != "" {
		if _, err := os.Stat(filepath.Join(explicit, "manifests", "pylons.yaml")); err == nil {
			return explicit
		}
		return ""
	}
	cur, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(cur, "manifests", "pylons.yaml")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

type docEntry struct {
	Path       string   `json:"path"`
	Type       string   `json:"type"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Package    string   `json:"package,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Components []string `json:"components,omitempty"`
	Body       string   `json:"-"`
}

func scanDocs(kbRoot string, includeArchive bool) []docEntry {
	var docs []docEntry
	_ = filepath.WalkDir(kbRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			relDir, relErr := filepath.Rel(kbRoot, p)
			if relErr == nil {
				relDir = filepath.ToSlash(relDir)
				if !strings.Contains(relDir, "/") {
					switch relDir {
					case ".git", "cli", "bin", "templates", "manifests", "protocols", "extensions":
						return filepath.SkipDir
					}
				}
			}
			if d.Name() == ".git" || (!includeArchive && d.Name() == "archive") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(kbRoot, p)
		rel = filepath.ToSlash(rel)
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(raw), &meta) {
			return nil
		}
		docs = append(docs, docEntry{
			Path:       rel,
			Type:       meta.Type,
			Status:     meta.Status,
			Title:      meta.Title,
			Package:    meta.Package,
			Tags:       meta.Tags,
			Components: meta.Components,
			Body:       string(raw),
		})
		return nil
	})
	slices.SortFunc(docs, func(a, b docEntry) int { return strings.Compare(a.Path, b.Path) })
	return docs
}

func filterDocs(docs []docEntry, typ, pkg, tag, status, folder, query string, limit int) []docEntry {
	var filtered []docEntry
	for _, d := range docs {
		if typ != "" && d.Type != typ {
			continue
		}
		if status != "" && d.Status != status {
			continue
		}
		if folder != "" && !strings.HasPrefix(d.Path, strings.TrimSuffix(folder, "/")+"/") {
			continue
		}
		if pkg != "" && d.Package != pkg && !slices.Contains(d.Components, pkg) {
			continue
		}
		if tag != "" && !slices.Contains(d.Tags, tag) {
			continue
		}
		filtered = append(filtered, d)
	}

	if strings.TrimSpace(query) != "" {
		bmDocs := make([]resolve.BM25Doc, 0, len(filtered))
		byPath := make(map[string]docEntry, len(filtered))
		for _, d := range filtered {
			bmDocs = append(bmDocs, resolve.BM25Doc{ID: d.Path, Text: d.Title + " " + strings.Join(d.Tags, " ") + " " + d.Body})
			byPath[d.Path] = d
		}
		idx := resolve.NewBM25Index(bmDocs)
		hits := idx.Search(query, limit)
		ranked := make([]docEntry, 0, len(hits))
		for _, h := range hits {
			ranked = append(ranked, byPath[h.ID])
		}
		return ranked
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func renderDocs(docs []docEntry, format report.Format) int {
	if docs == nil {
		docs = []docEntry{}
	}
	if format == report.FormatJSON {
		if err := json.NewEncoder(os.Stdout).Encode(docs); err != nil {
			return 2
		}
		return 0
	}
	for _, d := range docs {
		fmt.Printf("%s\t[%s/%s]\t%s\n", d.Path, d.Type, d.Status, d.Title)
	}
	return 0
}

// loadConfigOrWarn keeps commands running on a malformed registry while
// making the degradation visible instead of silently reporting zero projects.
func loadConfigOrWarn(kbRoot string) config.Config {
	cfg, err := config.Load(kbRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: warning: invalid manifests/pylons.yaml: %v\n", err)
	}
	return cfg
}
