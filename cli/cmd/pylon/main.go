// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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
	"github.com/estebanforge/pylons/cli/internal/sync"
	"github.com/estebanforge/pylons/cli/internal/version"
)

const usage = `pylon validates and queries a Pylons pylon: the plain-text map of a platform, checked against its code.

Usage:
  pylon lint             [--changed] [--doc-only] [--fix] [--parity] [--allow-external-extensions] [--format=human|json|sarif] [--json] [--ci] [--pylons <path>]
  pylon contract <plan>  [--allow-external-extensions] [--format=human|json|sarif] [--json] [--ci] [--pylons <path>]
  pylon check <paths...> [--stop-block] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon brief            [--format=human|json] [--json] [--pylons <path>]
  pylon context <paths...> [--preamble] [--format=human|json] [--json] [--pylons <path>]
  pylon context --diff-since <ref> [--manifest] [--format=human|json] [--json] [--pylons <path>]
  pylon context --graph <package> [--depth N] [--format=human|json] [--json] [--pylons <path>]
  pylon staleness        [--package <pkg>] [--fix] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review plan <path> [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review postmortem [path] [--check-sources] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review staleness [--package <pkg>] [--fix] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review ratchet   [--update] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review rework    [--window <period>] [--package <pkg>] [--json] [--pylons <path>]
  pylon review merge-danger --body <file|-> [--base <ref>] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review release-surface --pinned-releases <dir> [--base <ref>] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review gate-surface [--base <ref>] [--format=human|json|sarif] [--json] [--pylons <path>]
  pylon review annotations --from <file|-> [--path-prefix <p>] [--format=human|json] [--json] [--pylons <path>]
  pylon sync             [--detached] [--pylons <path>]
  pylon review-brief <plan> [--stdout] [--base <ref>] [--validate <file> | --validate-resolutions <file>] [--format=human|json] [--pylons <path>]
  pylon protocol [list|<name>] [--template <path>] [--pylons <path>]
  pylon reflect          [--template <path>] [--pylons <path>]
  pylon extensions [list|schema] [--format=human|json] [--json] [--pylons <path>]
  pylon map              [--type <t>] [--package <p>] [--tag <t>] [--status <s>] [--folder <dir>] [--query <q>] [--limit <n>] [--all] [--format=human|json] [--json] [--pylons <path>]
  pylon search <query>   [--limit <n>] [--all] [--format=human|json] [--json] [--pylons <path>]
  pylon manifest [rules|packages|product] [--check] [--release-version <v>] [--pylons <path>]
  pylon self-update --from <staged-release-dir> [--dry-run] [--force] [--pylons <path>]
  pylon explain <rule_id>
  pylon aroma search|recommend --corpus <dir> (--lines <path>:a-b | --snippet <file|-> | --diff <file|->) [--json]
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

	// aroma owns its own flag surface; route it before the shared parser.
	if args[0] == "aroma" {
		return cmdAroma(args[1:])
	}

	format := report.FormatHuman
	docOnly, changed, fix, ci, syncDetached := false, false, false, false, false
	checkManifest, stopBlock, preamble, checkSources := false, false, false, false
	rbStdout, mapAll := false, false
	allowExternal := os.Getenv("PYLON_ALLOW_EXTERNAL_EXTENSIONS") == "1"
	forceHooks, ratchetUpdate, lintParity := false, false, false
	releaseVersion := ""
	dryRunUpdate := false
	updateBaseline := false
	pylonsPath, pkgArg, diffBase, rbValidate, rbValidateRes, protoTmpl, reworkWindow, slicePlan, mergeBody, syncChild := "", "", "", "", "", "", "", "", "", ""
	pinnedReleases, sessionSince := "", ""
	sessionManifest, diffSinceSet := false, false
	graphPkg, annFrom, annPrefix := "", "", ""
	graphDepth := 0
	graphSet, depthSet := false, false
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
		case arg == "--version":
			fmt.Printf("pylon %s\n", version.Version)
			return 0
		case arg == "-h" || arg == "--help":
			fmt.Fprintln(os.Stdout, usage)
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
		case arg == "--force":
			forceHooks = true
		case arg == "--update":
			ratchetUpdate = true
		case arg == "--manifest":
			sessionManifest = true
		case arg == "--parity":
			lintParity = true
		case arg == "--update-baseline":
			updateBaseline = true
		case arg == "--dry-run":
			dryRunUpdate = true
		case arg == "--detached":
			syncDetached = true
		case strings.HasPrefix(arg, "--format="):
			format = report.Format(strings.TrimPrefix(arg, "--format="))
		case arg == "--format" || arg == "--pylons" || arg == "--package" || arg == "--validate" || arg == "--validate-resolutions" || arg == "--template" || arg == "--type" || arg == "--tag" || arg == "--status" || arg == "--folder" || arg == "--query" || arg == "--limit" || arg == "--window" || arg == "--slice" || arg == "--body" || arg == "--base" || arg == "--fetch-child" || arg == "--pinned-releases" || arg == "--diff-since" || arg == "--graph" || arg == "--from" || arg == "--depth" || arg == "--path-prefix" || arg == "--release-version":
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
			case "--window":
				reworkWindow = val
			case "--slice":
				slicePlan = val
			case "--body":
				mergeBody = val
			case "--pinned-releases":
				pinnedReleases = val
			case "--diff-since":
				sessionSince = val
				diffSinceSet = true
			case "--fetch-child":
				syncChild = val
			case "--graph":
				graphPkg = val
				graphSet = true
			case "--from":
				annFrom = val
			case "--path-prefix":
				annPrefix = val
			case "--release-version":
				releaseVersion = val
			case "--depth":
				n, err := strconv.Atoi(val)
				if err != nil || n < 0 {
					fmt.Fprintln(os.Stderr, "pylon: --depth must be a non-negative integer")
					return 2
				}
				graphDepth = n
				depthSet = true
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
	// Only findings-producing commands can render SARIF.
	switch cmd {
	case "map", "search", "brief", "extensions", "protocol", "manifest", "install", "init", "review-brief":
		if format == report.FormatSARIF {
			fmt.Fprintf(os.Stderr, "pylon: format sarif is not supported for %s\n", cmd)
			return 2
		}
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
		return cmdInstall(findPylonsRoot(pylonsPath), forceHooks)
	}

	// The detached fetch child must never depend on a KB lookup: it
	// runs with the parent's cwd and its own gitDir argument.
	if cmd == "sync" && syncChild != "" {
		return sync.FetchChild(syncChild)
	}

	kbRoot := findPylonsRoot(pylonsPath)
	if kbRoot == "" {
		if cmd == "brief" {
			fmt.Fprintln(os.Stderr, "pylon brief: no manifests/pylons.yaml found")
			return 3
		}
		fmt.Fprintln(os.Stderr, "pylon: could not locate manifests/pylons.yaml")
		return 3
	}

	switch cmd {
	case "lint":
		if lintParity && docOnly {
			fmt.Fprintln(os.Stderr, "warning: --parity is ignored with --doc-only; parity checks need the project source")
		}
		findings := rules.Lint(kbRoot, rules.LintOptions{
			HasCode:       !docOnly,
			Fix:           fix,
			ChangedOnly:   changed,
			AllowExternal: allowExternal,
			Parity:        lintParity,
		})
		if updateBaseline {
			if err := rules.UpdateBaseline(kbRoot, findings); err != nil {
				fmt.Fprintf(os.Stderr, "pylon: write baseline: %v\n", err)
				return 1
			}
			fmt.Fprintln(os.Stderr, "pylon: wrote manifests/baseline.json from the current FORCED findings")
			return 0
		}
		findings, accepted, stale := rules.ApplyBaseline(kbRoot, findings)
		if accepted > 0 {
			fmt.Printf("lint: %d finding(s) accepted by manifests/baseline.json\n", accepted)
		}
		if stale > 0 {
			fmt.Fprintf(os.Stderr, "pylon: %d baseline entr(ies) no longer match; shrink with pylon lint --update-baseline\n", stale)
		}
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
		rules.Remediate(findings)
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
		if diffSinceSet && sessionSince == "" {
			fmt.Fprintln(os.Stderr, "pylon: context --diff-since requires a ref")
			return 2
		}
		modes := 0
		for _, set := range []bool{len(rest) > 0, slicePlan != "", graphSet, sessionSince != ""} {
			if set {
				modes++
			}
		}
		if modes > 1 {
			fmt.Fprintln(os.Stderr, "pylon: context takes one of paths, --slice, --graph, or --diff-since")
			return 2
		}
		if graphSet && graphPkg == "" {
			fmt.Fprintln(os.Stderr, "pylon: context --graph requires a package")
			return 2
		}
		if depthSet && !graphSet {
			fmt.Fprintln(os.Stderr, "pylon: --depth only applies to context --graph")
			return 2
		}
		if sessionSince != "" {
			if slicePlan != "" || len(rest) != 0 {
				fmt.Fprintln(os.Stderr, "pylon: context --diff-since takes no other target")
				return 2
			}
			if format == report.FormatSARIF {
				fmt.Fprintln(os.Stderr, "pylon: format sarif is not supported for session capture")
				return 2
			}
			return cmdCapture(kbRoot, sessionSince, sessionManifest, format)
		}
		if graphPkg != "" {
			if format == report.FormatSARIF {
				fmt.Fprintln(os.Stderr, "pylon: format sarif is not supported for the dependency graph")
				return 2
			}
			return cmdGraph(kbRoot, graphPkg, graphDepth, format)
		}
		if slicePlan != "" {
			if len(rest) != 0 {
				fmt.Fprintln(os.Stderr, "pylon: context --slice takes a plan, not paths")
				return 2
			}
			if format == report.FormatSARIF {
				fmt.Fprintln(os.Stderr, "pylon: format sarif is not supported for slice context")
				return 2
			}
			return cmdSliceContext(kbRoot, slicePlan, allowExternal, format)
		}
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: context requires <paths...>")
			return 2
		}
		if preamble && format == report.FormatHuman {
			cfg := loadConfigOrWarn(kbRoot)
			fmt.Printf("=== Pylons Context (%s | projects: %d | extensions: %v) ===\n", kbRoot, len(cfg.Projects), cfg.Extensions)
		}
		findings := rules.Check(kbRoot, rest)
		rules.Remediate(findings)
		if err := report.Render(os.Stdout, format, findings); err != nil {
			return 2
		}
		return 0

	case "staleness":
		findings := rules.CheckStaleness(kbRoot, pkgArg, fix)
		return renderFindings(findings, format, ci)

	case "review":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: review requires subcommand: plan | postmortem | staleness | ratchet | rework | merge-danger | release-surface | gate-surface")
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
		case "annotations":
			return cmdAnnotations(annFrom, annPrefix, format)
		case "ratchet":
			findings, err := rules.RunRatchet(kbRoot, rules.RatchetOptions{Update: ratchetUpdate, AllowExternal: allowExternal})
			if err != nil {
				fmt.Fprintf(os.Stderr, "pylon: ratchet: %v\n", err)
				return 2
			}
			return renderFindings(findings, format, ci)
		case "rework":
			return cmdReworkRate(kbRoot, pkgArg, reworkWindow, format)
		case "merge-danger":
			return cmdMergeDanger(kbRoot, mergeBody, diffBase, format, ci)
		case "release-surface":
			if pinnedReleases == "" {
				fmt.Fprintln(os.Stderr, "pylon: review release-surface requires --pinned-releases <dir>")
				return 2
			}
			findings, err := rules.ReleaseSurface(kbRoot, rules.ReleaseSurfaceOptions{PinnedDir: pinnedReleases, Base: diffBase})
			if err != nil {
				fmt.Fprintf(os.Stderr, "pylon: review release-surface: %v\n", err)
				return 2
			}
			rules.Remediate(findings)
			if err := report.Render(os.Stdout, format, findings); err != nil {
				return 2
			}
			if report.HasBlocking(findings) {
				return 1
			}
			return 0
		case "gate-surface":
			findings := rules.GateSurface(kbRoot, diffBase)
			rules.Remediate(findings)
			return renderFindings(findings, format, ci)
		default:
			fmt.Fprintf(os.Stderr, "pylon: unknown review subcommand %s\n", rest[0])
			return 2
		}

	case "sync":
		if !syncDetached {
			fmt.Fprintln(os.Stderr, "pylon: sync requires --detached (fetches never block a session)")
			return 2
		}
		res, err := sync.Run(kbRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: sync: %v\n", err)
			return 1
		}
		if format == report.FormatJSON {
			_ = json.NewEncoder(os.Stdout).Encode(res)
			return 0
		}
		if res.FetchStarted {
			fmt.Println("detached fetch started; rerun shortly to apply")
		}
		if res.FastForwarded {
			fmt.Println("fast-forwarded onto origin/main")
		}
		if res.Diverged {
			fmt.Printf("behind origin/main by %d commit(s); the tree diverged, nothing was applied\n", res.Behind)
		} else if res.Behind > 0 {
			fmt.Printf("behind origin/main by %d commit(s); tree is dirty, clean it to apply\n", res.Behind)
		}
		return 0

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
		fetchFailedAt, fetchOK, _ := sync.Status(kbRoot)
		if format == report.FormatJSON {
			doc := map[string]any{
				"root":       kbRoot,
				"projects":   len(cfg.Projects),
				"extensions": cfg.Extensions,
				"documents":  len(docs),
			}
			if !fetchOK {
				doc["fetch_failed"] = fetchFailedAt.UTC().Format(time.RFC3339)
			}
			_ = json.NewEncoder(os.Stdout).Encode(doc)
			return 0
		}
		machines := 0
		for _, d := range docs {
			machines += d.Machines
		}
		fmt.Printf("Pylons: %s | projects: %d | extensions: %d | docs: %d | machines: %d\n", kbRoot, len(cfg.Projects), len(cfg.Extensions), len(docs), machines)
		if !fetchOK {
			fmt.Printf("FETCH-FAILED %s\n", fetchFailedAt.UTC().Format(time.RFC3339))
		}
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

	case "self-update":
		return cmdSelfUpdate(kbRoot, annFrom, forceHooks, dryRunUpdate)

	case "manifest":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "pylon: manifest requires 'rules' or 'packages'")
			return 2
		}
		switch rest[0] {
		case "rules":
			if !generatesManifest(kbRoot, "rules") {
				fmt.Fprintln(os.Stderr, "pylon: rules manifest generation is disabled by manifests.generate")
				return 0
			}
			return cmdManifestJSON(filepath.Join(kbRoot, "manifests", "rules.json"), rules.AllRules(), checkManifest)
		case "product":
			if releaseVersion == "" {
				fmt.Fprintln(os.Stderr, "pylon: manifest product requires --release-version <v>")
				return 2
			}
			m, err := install.BuildProductManifest(kbRoot, releaseVersion)
			if err != nil {
				fmt.Fprintf(os.Stderr, "pylon: manifest product: %v\n", err)
				return 1
			}
			out, err := install.WriteProductManifest(m)
			if err != nil {
				fmt.Fprintf(os.Stderr, "pylon: manifest product: %v\n", err)
				return 1
			}
			os.Stdout.Write(out)
			return 0
		case "packages":
			if !generatesManifest(kbRoot, "packages") {
				fmt.Fprintln(os.Stderr, "pylon: packages manifest generation is disabled by manifests.generate")
				return 0
			}
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

func cmdSliceContext(kbRoot, planArg string, allowExternal bool, format report.Format) int {
	rep, err := rules.SliceContext(kbRoot, planArg, allowExternal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: context: %v\n", err)
		return 2
	}
	if format == report.FormatJSON {
		if err := json.NewEncoder(os.Stdout).Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: context: encode report: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Printf("=== Slice %s (%s | components: %v) ===\n", rep.Title, rep.Plan, rep.Components)
	for _, c := range rep.Contracts {
		state := "unresolved"
		if c.Status != "stable" {
			state = c.Status
		} else if c.Resolved {
			state = "resolved"
		}
		detail := ""
		if c.Detail != "" {
			detail = " (" + c.Detail + ")"
		}
		fmt.Printf("  [%s] %s %s: %s%s\n", c.Status, c.Kind, c.ID, state, detail)
	}
	for _, q := range rep.Quirks {
		fmt.Printf("  quirk: %s\n", q)
	}
	for _, term := range rep.Terms {
		fmt.Printf("  term: %s\n", term)
	}
	return 0
}

func cmdMergeDanger(kbRoot, bodyPath, base string, format report.Format, ci bool) int {
	if bodyPath == "" {
		fmt.Fprintln(os.Stderr, "pylon: merge-danger requires --body <file|->")
		return 2
	}
	findings, err := rules.MergeDanger(kbRoot, rules.MergeDangerOptions{Body: bodyPath, Base: base})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: merge-danger: %v\n", err)
		return 2
	}
	return renderFindings(findings, format, ci)
}

func cmdReworkRate(kbRoot, pkgFilter, window string, format report.Format) int {
	if format == report.FormatSARIF {
		fmt.Fprintln(os.Stderr, "pylon: format sarif is not supported for rework")
		return 2
	}
	if err := rules.ValidateWindow(window); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: rework: %v\n", err)
		return 2
	}
	reports, err := rules.ReworkRate(kbRoot, pkgFilter, window)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: rework: %v\n", err)
		return 2
	}
	if reports == nil {
		reports = []rules.ReworkReport{}
	}
	if format == report.FormatJSON {
		_ = json.NewEncoder(os.Stdout).Encode(reports)
		return 0
	}
	for _, r := range reports {
		fmt.Printf("%s: fix rework %d/%d, refactor rework %d/%d (hot paths %d, window %s)\n",
			r.Project, r.ReworkFix, r.Fix, r.ReworkRefactor, r.Refactor, r.HotPaths, r.Window)
	}
	fmt.Printf("rework-rate: %d projects, report-only\n", len(reports))
	return 0
}

func renderFindings(findings []report.Finding, format report.Format, ci bool) int {
	rules.Remediate(findings)
	if err := report.Render(os.Stdout, format, findings); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: render failed: %v\n", err)
		return 2
	}
	if report.HasBlocking(findings) || (ci && len(findings) > 0) {
		return 1
	}
	return 0
}

var scaffoldDirs = []string{"packages", "packages/archive", "proposals", "plans/archive", "decisions/proposals", "decisions/archive", "conventions", "conventions/proposals", "conventions/archive", "quirks/archive", "domain/entities", "domain/archive", "investigations/archive", "references/archive", "tickets/archive", "prompts", "skills", "templates", "manifests", "protocols", "extensions"}

func cmdInit(dir string) int {
	cfgPath := filepath.Join(dir, "manifests", "pylons.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("pylon init: %s already exists (run pylon install to wire gate hooks)\n", cfgPath)
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
		"AGENTS.md": "# Guidelines\n\nA pylon: the plain-text map of this platform. See `manifests/vocabulary.json` and `meta/docs/`.\n",
	}
	for _, dir := range scaffoldDirs {
		zone, _, _ := strings.Cut(dir, "/")
		switch zone {
		case "templates", "manifests", "protocols", "extensions":
			continue
		}
		title := strings.ToUpper(zone[:1]) + zone[1:]
		seedFiles[zone+"/AGENTS.md"] = "---\ntitle: \"" + title + "\"\ntype: reference\nstatus: active\n---\n\n# " + title + "\n"
	}
	seedFiles["conventions/AGENTS.md"] = "---\ntitle: \"Conventions\"\ntype: reference\nstatus: active\n---\n\n# Conventions\n"
	for rel, body := range seedFiles {
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if err := os.WriteFile(dst, []byte(body), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "pylon init: %v\n", err)
				return 1
			}
		}
	}
	giPath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(giPath); os.IsNotExist(err) {
		if err := os.WriteFile(giPath, []byte("._*\n.DS_Store\n/bin/\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "pylon init: %v\n", err)
			return 1
		}
	}
	for _, sub := range scaffoldDirs {
		path := filepath.Join(dir, sub)
		entries, err := os.ReadDir(path)
		if err == nil && len(entries) == 0 {
			if err := os.WriteFile(filepath.Join(path, ".gitkeep"), nil, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "pylon init: %v\n", err)
				return 1
			}
		}
	}
	if res, err := install.EnsureHooks(dir, false); err != nil {
		if errors.Is(err, install.ErrNotGit) {
			fmt.Printf("pylon init: not a git repository; run pylon install after git init\n")
		} else {
			fmt.Fprintf(os.Stderr, "pylon init: hooks not wired: %v\n", err)
		}
	} else if len(res.WroteHooks) > 0 {
		fmt.Printf("pylon init: wired gate hooks (core.hooksPath .githooks)\n")
	}
	fmt.Printf("pylon init: initialized %s\n", dir)
	return 0
}

func cmdInstall(kbRoot string, force bool) int {
	if kbRoot == "" {
		fmt.Fprintln(os.Stderr, "pylon: could not locate manifests/pylons.yaml")
		return 3
	}
	res, err := install.EnsureHooks(kbRoot, force)
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
	for _, name := range res.DriftedHooks {
		fmt.Printf("pylon install: kept modified .githooks/%s (differs from shipped asset; --force overwrites)\n", name)
	}
	// Every gate hook foreign means none of the pylon gates run.
	// Quiet success here is how an adopter believes they are gated
	// when they are not; refuse and point at the takeover flag.
	if len(res.DriftedHooks) > 0 && len(res.SkippedHooks) == 0 && len(res.WroteHooks) == 0 {
		fmt.Fprintf(os.Stderr, "pylon install: every gate hook is foreign; the pylon gates will not run\n")
		fmt.Fprintf(os.Stderr, "pylon install: %s in .githooks differ from the shipped gates; inspect them, or take over with pylon install --force\n", strings.Join(res.DriftedHooks, ", "))
		return 1
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

// generatesManifest loads the KB config and asks whether the gates
// maintain the named manifest; an unreadable config falls back to
// generation on, matching the pre-toggle behavior.
func cmdSelfUpdate(kbRoot, from string, force, dryRun bool) int {
	if from == "" {
		fmt.Fprintln(os.Stderr, "pylon: self-update requires --from <staged-release-dir>")
		return 2
	}
	res, err := install.SelfUpdate(kbRoot, from, force, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon self-update: %v\n", err)
		return 1
	}
	label := "plan"
	if !dryRun {
		label = "update"
	}
	fmt.Printf("self-update %s: release %s\n", label, res.Version)
	for _, rel := range res.Replaced {
		fmt.Printf("  replaced %s\n", rel)
	}
	for _, rel := range res.Drifted {
		fmt.Printf("  drifted  %s (your edit kept; new copy at %s.pylons-new)\n", rel, rel)
	}
	for _, rel := range res.Removed {
		fmt.Printf("  removed  %s\n", rel)
	}
	for _, rel := range res.KeptDirty {
		fmt.Printf("  kept     %s (you edited it; the new release dropped it)\n", rel)
	}
	if res.BinaryNew {
		fmt.Printf("  binary   bin/pylon updated\n")
	}
	if res.BackupDir != "" {
		fmt.Printf("  backups  %s/\n", res.BackupDir)
	}
	if dryRun {
		fmt.Printf("  dry run: nothing written\n")
	}
	if len(res.Drifted) > 0 || len(res.KeptDirty) > 0 {
		fmt.Fprintf(os.Stderr, "pylon self-update: %d file(s) carry your edits; inspect the .pylons-new copies and kept files, then commit\n", len(res.Drifted)+len(res.KeptDirty))
		return 1
	}
	fmt.Fprintln(os.Stderr, "pylon self-update: review with git status, then commit the update")
	return 0
}

func generatesManifest(kbRoot, name string) bool {
	cfg, _ := config.Load(kbRoot)
	return cfg.GeneratesManifest(name)
}

func cmdManifestJSON(outPath string, payload any, checkOnly bool) int {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: marshal manifest: %v\n", err)
		return 1
	}
	data = append(data, '\n')
	if checkOnly {
		existing, err := os.ReadFile(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: %s is missing; run the manifest command without --check\n", outPath)
			return 1
		}
		// Normalize CRLF so autocrlf checkouts do not report phantom drift.
		if strings.ReplaceAll(string(existing), "\r\n", "\n") != string(data) {
			fmt.Fprintf(os.Stderr, "pylon: %s is out of date\n", outPath)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
		return 1
	}
	tmp, err := os.CreateTemp(filepath.Dir(outPath), ".pylon-manifest-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
		return 1
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
		return 1
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
		return 1
	}
	if err := os.Rename(tmp.Name(), outPath); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: %v\n", err)
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
	Machines   int      `json:"-"`
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
			if d.Name() == ".git" || (!includeArchive && d.Name() == "archive") || strings.HasPrefix(d.Name(), "_") {
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
			Machines:   len(meta.Machines),
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

func cmdGraph(kbRoot, pkgName string, depth int, format report.Format) int {
	graph, err := rules.DependencyGraph(kbRoot, pkgName, depth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: context: %v\n", err)
		return 2
	}
	if format == report.FormatJSON {
		if err := json.NewEncoder(os.Stdout).Encode(graph); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: context: encode graph: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Printf("=== Dependency graph for %s (depth %d) ===\n", report.CleanText(graph.Package), graph.Depth)
	for _, w := range graph.Warnings {
		fmt.Printf("  warning: %s\n", report.CleanText(w))
	}
	fmt.Printf("  packages: %v\n", slices.Sorted(maps.Keys(graph.Declared)))
	for _, pkg := range slices.Sorted(maps.Keys(graph.Declared)) {
		if deps := graph.Declared[pkg]; len(deps) > 0 {
			fmt.Printf("  %s declares depends_on: %v\n", pkg, deps)
		} else {
			fmt.Printf("  %s declares no dependencies\n", pkg)
		}
	}
	for _, e := range graph.Edges {
		fmt.Printf("  edge %s -> %s\n", e.From, e.To)
	}
	for _, e := range graph.Undeclared {
		fmt.Printf("  undeclared: %s -> %s\n", report.CleanText(e.From), report.CleanText(e.To))
	}
	for _, u := range graph.Unused {
		fmt.Printf("  unused: %s\n", u)
	}
	return 0
}

func cmdAnnotations(from, pathPrefix string, format report.Format) int {
	if format == report.FormatSARIF {
		fmt.Fprintln(os.Stderr, "pylon: format sarif is not supported for annotations; they already are Check payloads")
		return 2
	}
	if from == "" {
		fmt.Fprintln(os.Stderr, "pylon: review annotations requires --from <file|->")
		return 2
	}
	input, err := readAnnotationInput(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: review annotations: %v\n", err)
		return 2
	}
	out, err := rules.Annotations(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: review annotations: %v\n", err)
		return 2
	}
	// Findings are KB-relative; a KB living in a repo subdirectory
	// needs the prefix to make annotation paths repo-relative.
	if pathPrefix != "" {
		for i := range out.Annotations {
			out.Annotations[i].Path = pathPrefix + "/" + out.Annotations[i].Path
		}
	}
	if format == report.FormatJSON {
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: review annotations: encode: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Print(out.Summary)
	for _, a := range out.Annotations {
		fmt.Printf("  %s:%d %s %s\n", report.CleanText(a.Path), a.StartLine, report.CleanText(a.Title), report.CleanText(firstAnnLine(a.Message)))
	}
	return 0
}

// readAnnotationInput applies the shared bounded-regular-read guard to
// the findings document, so a FIFO named lint.sarif cannot hang the CI
// job and a runaway file cannot exhaust memory.
func readAnnotationInput(from string) ([]byte, error) {
	const maxInputBytes = 4 * 1024 * 1024
	if from == "-" {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxInputBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxInputBytes {
			return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
		}
		return data, nil
	}
	f, err := os.Open(from)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", from)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}
	return data, nil
}

func firstAnnLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func cmdCapture(kbRoot, since string, emitManifest bool, format report.Format) int {
	manifest, err := rules.SessionCapture(kbRoot, since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: context: %v\n", err)
		return 2
	}
	// --manifest emits the manifest document itself: JSON form, exit 0.
	if emitManifest || format == report.FormatJSON {
		if err := json.NewEncoder(os.Stdout).Encode(manifest); err != nil {
			fmt.Fprintf(os.Stderr, "pylon: context: encode manifest: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Printf("=== Session capture since %s (%d declared surfaces) ===\n", manifest.Since, manifest.Examined)
	for _, w := range manifest.Warnings {
		fmt.Printf("  warning: %s\n", w)
	}
	if len(manifest.Surfaces) == 0 {
		fmt.Println("  no stale surfaces; declared docs moved with the code")
		return 0
	}
	for _, s := range manifest.Surfaces {
		fmt.Printf("  [%s] %s %s in %s: doc %s did not move with the code\n", s.Kind, s.Alias, s.Path, s.Alias, s.Doc)
	}
	return 0
}
