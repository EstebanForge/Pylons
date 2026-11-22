---
title: "Using Pylons and the pylon CLI"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-19
---

# Using Pylons and the `pylon` CLI

Pylons organizes knowledge by rate of change and verifies every claim against tracked repositories.

## Daily workflow

1. Orient: run `./bin/pylon brief` at session start, then `pylon map --type plan --status in-progress` or `pylon search "<query>"` to find existing context before starting work.
2. Check collisions: before editing code, run `./bin/pylon check <paths...>` (or `--stop-block` for agents). Run `pylon context --preamble <paths...>` to get active plan collisions and protected globs.
3. Draft work: track bugs, tasks, and improvements in `tickets/` from `templates/ticket.md`. Draft new feature or refactor plans in `proposals/` from `templates/plan.md`. When accepted, move the plan to `plans/`, add `ticket:` (`PYLON016`), and add `wrong_if:` to every `contract.stable` entry (`PYLON014`).
4. Record traps and decisions: document non-obvious integration traps in `quirks/` from `templates/quirk.md` with `contracts:` and `sources:`, and set `guard:` within 30 days (`PYLON028`). Record architecture decisions in `decisions/proposals/` and number them `DNNNN-<slug>.md` on acceptance (`PYLON010`). Record complex bug root-cause analyses in `investigations/YYYY-MM-DD-<slug>.md` (`PYLON030`).
5. Ship and archive: set `status: shipped` with `artifact:` (`PYLON009`) on completed plans, `done` on completed tickets, and move terminal docs to `<zone>/archive/` (or run `pylon lint --fix`).
6. Verify: run `./bin/pylon lint --ci` before committing.

## Command reference

| Command | Purpose |
|---|---|
| `pylon brief` | Non-blocking session summary (always exits 0). |
| `pylon check <paths...>` | Intersect target paths with `contract.editing`, `contract.unstable`, and `protected:` globs. |
| `pylon context <paths...>` | Emit architectural slice for target files (`--preamble` adds workspace header). |
| `pylon context --slice <plan>` | Print only one plan's contracts with resolution state, linked quirks, and linked domain terms (`--json` supported). |
| `pylon context --diff-since <ref> [--manifest]` | Report declared surfaces the diff touched whose docs did not move in the same range; uncommitted edits count on both sides, warnings name unreadable projects, `--manifest` emits the JSON document. Report-only. Glob contract ids are out of scope. |
| `pylon lint` | Validate frontmatter, folder lifecycle, links, STE-100, and contracts (`--changed`, `--parity` (declared-vs-code parity: domain `states:` against the enum at `states_symbol`; `machines:` transition triples against the marked code map via the go, php, and rust profiles or external plugins; route exposes against project source, advisory) plus `--doc-only`, `--fix`, `--ci`). Findings render a remediation line from the rule catalog. macOS AppleDouble `._*` docs are skipped when `.gitignore` covers them; otherwise they fail with a fix hint. With `--ci`, any finding at all escalates to exit 1. |
| `pylon contract <plan>` | Validate 1 plan's `contract.stable` entries against tracked projects. |
| `pylon map` | Filterable corpus overview ranked by BM25 (`--type`, `--package`, `--tag`, `--status`, `--folder`, `--query`, `--limit`, `--all`). |
| `pylon search <query>` | Full-text BM25 search across non-archived docs (`--limit`, `--all`). |
| `pylon aroma search\|recommend` | Structural code similarity: methods resembling changed lines, a snippet, or a diff, plus the code they share (`--lines`, `--snippet`, `--diff`, `--min`, `--json`). Corpus is registered projects by default, else `--corpus`. Advisory; never gates. |
| `pylon staleness` | Flag `packages/*.md` older than 90 days (`--package`, `--fix`). |
| `pylon review plan <path>` | Check proposal/plan structure and STE-100 compliance. |
| `pylon review postmortem [path]` | Check quirk sections and `sources:` existence (`--check-sources`). |
| `pylon review ratchet` | Compare advisory counts per package/zone against `manifests/ratchet.json`; rising counts block (`--update` re-baselines after review). |
| `pylon review rework` | Count `fix:`/`refactor:` commits touching paths a `feat:` commit changed, per registered project, report-only (`--window`, `--package`). |
| `pylon lint --update-baseline` | Rewrite `manifests/baseline.json` from the current FORCED findings. When the baseline exists, matched findings stop blocking while new ones still fail; entries that no longer match are reported so the file only shrinks. |
| `pylon self-update --from <dir> [--dry-run] [--force]` | Apply a staged release to an installed pylon. Staged files verify against the release manifest first; unmodified product files are replaced, your edits land as `.pylons-new` copies (never overwritten), user files are untouched, replacements are backed up under `_pylons-backup/`, and the binary swaps last. |
| `pylon review annotations --from <file|->` | Convert lint findings (SARIF or JSON) into GitHub Check annotations plus a summary comment body; blocking errors only, capped at 50 with the total kept. Findings without a usable path stay in the totals but never annotate, because GitHub rejects a whole batch over one bad entry. `--path-prefix <p>` makes paths repo-relative when the KB lives in a subdirectory. The engine emits the payload; the CI workflow posts it. |
| `pylon review merge-danger` | Gate one `Merge danger:` body line against the closed door vocabulary and the diff's derived minimum door (`--body <file|->`, `--base <ref>`; blocking on missing, invalid, duplicate, or downgraded lines). |
| `pylon review release-surface` | Block when a pinned deployment consumes a public surface removed or changed against its pinned release tag (`--pinned-releases <dir>`, `--base <ref>` overrides pinning; blocking on removed, changed, unknown pins, or a missing tag). |
| `pylon review gate-surface` | Block when a diff touches a surface the checker itself relies on: suppression and closed-set manifests, the shipped hooks, the declared-schema templates, or a widened gate knob in `pylons.yaml` (`--base <ref>`, default `origin/main`; under `--ci` an unreadable base fails the run). |
| `pylon sync --detached` | Start a detached background fetch when the last one is over an hour old, then fast-forward onto `origin/main` on a clean tree; never blocks. |
| `pylon review-brief <plan>` | Generate `.pylon-review-brief.md` or validate review packets (`--validate`, `--validate-resolutions`). |
| `pylon protocol [list|<name>]` | List or render CLI protocols (`pylon reflect` is the hygiene pass; `pylon protocol agent` is the session loop). |
| `pylon manifest [rules|packages|product]` | Generate or `--check` `manifests/rules.json` and `manifests/packages.json` (a `generate:` list in pylons.yaml disables either; a disabled manifest never regenerates and is never stale). `product` renders `manifests/product.json`, the release-time hash list of the product layer. |
| `pylon explain <rule_id>` | Print rule description and fix hint for any `PYLON*` rule. |
| `pylon install` | Write `.githooks/` and enable `core.hooksPath`; idempotent. Refuses a pylon below the repository root, refuses to replace another tool's `core.hooksPath` unless `--force`, and reports hooks that drifted from the shipped assets. |
| `pylon init [dir]` | Scaffold a new pylon; wires gate hooks when run inside a git repository. |
