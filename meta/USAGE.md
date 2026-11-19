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
| `pylon lint` | Validate frontmatter, folder lifecycle, links, STE-100, and contracts (`--changed`, `--doc-only`, `--fix`, `--ci`). macOS AppleDouble `._*` docs are skipped when `.gitignore` covers them; otherwise they fail with a fix hint. |
| `pylon contract <plan>` | Validate 1 plan's `contract.stable` entries against tracked projects. |
| `pylon map` | Filterable corpus overview ranked by BM25 (`--type`, `--package`, `--tag`, `--status`, `--folder`, `--query`, `--limit`, `--all`). |
| `pylon search <query>` | Full-text BM25 search across non-archived docs (`--limit`, `--all`). |
| `pylon staleness` | Flag `packages/*.md` older than 90 days (`--package`, `--fix`). |
| `pylon review plan <path>` | Check proposal/plan structure and STE-100 compliance. |
| `pylon review postmortem [path]` | Check quirk sections and `sources:` existence (`--check-sources`). |
| `pylon review-brief <plan>` | Generate `.pylon-review-brief.md` or validate review packets (`--validate`, `--validate-resolutions`). |
| `pylon protocol [list|<name>]` | List or render CLI protocols (`pylon reflect` is shorthand for `pylon protocol reflect`). |
| `pylon manifest [rules|packages]` | Generate or `--check` `manifests/rules.json` and `manifests/packages.json`. |
| `pylon explain <rule_id>` | Print rule description and fix hint for any `PYLON*` rule. |
| `pylon install` | Write `.githooks/` and enable `core.hooksPath`; idempotent. |
| `pylon init [dir]` | Scaffold a new KB; wires gate hooks when run inside a git repository. |
