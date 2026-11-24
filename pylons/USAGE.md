---
title: "Using Pylons and the pylon CLI"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-24
---

# Using Pylons and the `pylon` CLI

Pylons organizes knowledge by rate of change and verifies every claim against tracked repositories.

## Daily workflow

1. Orient: run `./bin/pylon brief` at session start, then `pylon map --type plan --status in-progress` or `pylon search "<query>"` to find existing context before starting work.
2. Check collisions: before editing code, run `./bin/pylon check <paths...>` (or `--stop-block` for agents). Run `pylon context --preamble <paths...>` to get active plan collisions and protected globs. When you work an accepted plan, add `--plan <plan>`: the scope of that plan is yours, and every other plan still stops you.
3. Draft work: track bugs, tasks, and improvements in `tickets/` from `templates/ticket.md`. Draft new feature or refactor plans in `proposals/` from `templates/plan.md`. When accepted, move the plan to `plans/`, add `ticket:` (`PYLON016`), and add `wrong_if:` to every `contract.stable` entry (`PYLON014`).
4. Record traps and decisions: document non-obvious integration traps in `quirks/` from `templates/quirk.md` with `contracts:` and `sources:`, and set `guard:` within 30 days (`PYLON028`). Record architecture decisions in `decisions/proposals/` and number them `DNNNN-<slug>.md` on acceptance (`PYLON010`). Record complex bug root-cause analyses in `investigations/YYYY-MM-DD-<slug>.md` (`PYLON030`).
5. Ship and archive: set `status: shipped` with `artifact:` (`PYLON009`) on completed plans, `done` on completed tickets, and move terminal docs to `<zone>/archive/` (or run `pylon lint --fix`).
6. Verify: run `./bin/pylon lint --ci` before committing.

## Command reference

Ledger commands accept `--pylons <path>` to point at an explicit ledger; `gate`, `aroma`, and `explain` do not. Findings commands accept `--format human|json|sarif`; session commands accept `human` and `json`, and `--json` is the json alias. `pylon --version` prints the CLI version.

| Command | Purpose |
|---|---|
| `pylon brief` | Non-blocking session summary (always exits 0). |
| `pylon check <paths...>` | Intersect target paths with `contract.editing`, `contract.unstable`, and `protected:` globs. |
| `pylon check --plan <plan> <paths...>` | Same check with one accepted or in-progress plan held as yours: its `editing` and `unstable` sets do not stop you. A name that is not an active plan exits 2. |
| `pylon context <paths...>` | Emit architectural slice for target files (`--preamble` adds workspace header). Quirks whose `repo://` sources cover a target surface as quirk guardrails in the human output and count as injections; `repo://` and absolute targets only. |
| `pylon context --slice <plan>` | Print only one plan's contracts with resolution state, linked quirks, and linked domain terms (`--json` supported). |
| `pylon context --graph <package> [--depth N]` | Dependency view: actual import edges across registered projects against declared `depends_on`, naming undeclared and unused divergences; `--depth` walks transitively. Import extraction runs for enabled languages with a package identity: go (go.mod), typescript (package.json, also covers javascript), python, rust (Cargo.toml), php (composer psr-4), java (pom.xml), kotlin (gradle), and dart (pubspec.yaml). C and C++ have no package identity, so they produce no edges. |
| `pylon context --diff-since <ref> [--manifest]` | Report declared surfaces the diff touched whose docs did not move in the same range; uncommitted edits count on both sides, warnings name unreadable projects, `--manifest` emits the JSON document. Report-only. Glob contract ids are out of scope. |
| `pylon lint` | Validate frontmatter, folder lifecycle, links, STE-100, and contracts (`--changed`, `--parity` (declared-vs-code parity: domain `states:` against the enum at `states_symbol`; `machines:` transition triples against the marked code map via the go, php, rust, and typescript profiles or external plugins; route exposes against project source, advisory) plus `--doc-only`, `--fix`, `--ci`, and `--allow-external-extensions` (permits external plugins for this run; the default refuses them, and `PYLON_ALLOW_EXTERNAL_EXTENSIONS=1` is the environment equivalent). Findings render a remediation line from the rule catalog. macOS AppleDouble `._*` docs are skipped when `.gitignore` covers them; otherwise they fail with a fix hint. With `--ci`, any finding at all escalates to exit 1. |
| `pylon contract <plan>` | Validate 1 plan's `contract.stable` entries against tracked projects (`--allow-external-extensions` permits external plugins). |
| `pylon map` | Filterable corpus overview ranked by BM25 plus fusion signals (`--type`, `--package`, `--tag`, `--status`, `--folder`, `--query`, `--limit`, `--all`). |
| `pylon search <query>` | Full-text BM25 search across non-archived docs, fused with status, recency, rule id, and project signals; file and package query tokens expand to owning packages and graph neighbors (`--limit`, `--all`). |
| `pylon aroma search\|recommend` | Structural code similarity: methods resembling changed lines, a snippet, or a diff, plus the code they share (`--lines`, `--snippet`, `--diff`, `--min`, `--json`). Corpus is registered projects by default, else `--corpus`. Advisory; never gates. |
| `pylon staleness` | Flag `packages/*.md` older than 90 days (`--package`, `--fix`). |
| `pylon review plan <path>` | Check proposal/plan structure and STE-100 compliance. |
| `pylon review postmortem [path]` | Check quirk sections and `sources:` existence (`--check-sources`). |
| `pylon review staleness [--package <pkg>] [--fix]` | The staleness pass as a review lens: same engine, review grouping for release work. |
| `pylon review ratchet` | Compare advisory counts per package/zone against `manifests/ratchet.json`; rising counts block (`--update` re-baselines after review). |
| `pylon review rework` | Count `fix:`/`refactor:` commits touching paths a `feat:` commit changed, per registered project, report-only (`--window`, `--package`). |
| `pylon lint --update-baseline` | Rewrite `manifests/baseline.json` from the current FORCED findings. When the baseline exists, matched findings stop blocking while new ones still fail; entries that no longer match are reported so the file only shrinks. |
| `pylon self-update --from <dir> [--dry-run] [--force]` | Apply a staged release to an installed pylon. Staged files verify against the release manifest first; unmodified product files are replaced, your edits land as `.pylons-new` copies (never overwritten), user files are untouched, replacements are backed up under `_pylons-backup/`, and the binary swaps last. |
| `pylon review annotations --from <file|->` | Convert lint findings (SARIF or JSON) into GitHub Check annotations plus a summary comment body; blocking errors only, capped at 50 with the total kept. Findings without a usable path stay in the totals but never annotate, because GitHub rejects a whole batch over one bad entry. `--path-prefix <p>` makes paths repo-relative when the KB lives in a subdirectory. The engine emits the payload; the CI workflow posts it. |
| `pylon harvest pr --diff <patch|-> [--metadata <json>] [--record <file>]` | Parse a merged PR diff plus optional PR metadata into a reviewable candidate patch: exported-symbol `exposes:` proposals for the claiming package doc, scaffolded quirk post-mortems for `Footgun:` markers in commit or review bodies, and warnings for unattributed or vendored paths. Report-only; the engine never writes to main. A diff over the 4 MiB input cap exits 2 with a split-the-merge hint. |
| `pylon review retro --findings <file|-> [--window <period>] [--min <n>] [--shape <s>]` | Count review-finding signatures (lens plus rule, contract, and path; free text accumulates on a normalized-text slug) inside the window and open exactly one promotion candidate per repeated signature: a convention stub, a `wrong_if` entry, or a lint-rule proposal. Report-only; the engine never writes to main. Prune records outside every window you use; the findings read cap is 64 MiB. |
| `pylon review utility [--prune] [--promote]` | Score ledger documents from citation markers (`<!-- pylon:referenced-doc: <path>.md -->`, never inside fenced code blocks and never from README/AGENTS/CONTEXT docs) in the corpus and injection telemetry recorded at human `pylon context` delivery: citations over injections decayed on a 30-day half-life, day-granular. `--prune` moves uncited quirks delivered on 2 or more distinct days past the weighted floor (mass 3.0) to `quirks/archive/` (an existing archive entry is never overwritten); `--promote` writes one convention proposal stub per document cited 3 or more times from 2 or more files, named from the full ledger path. Deleted documents never score. Report-only without the flags. The telemetry state lives in the git dir, so a fresh clone starts with no history and nothing prunes off absence of evidence; a malformed state exits 2. |
| `pylon review merge-danger` | Gate one `Merge danger:` body line against the closed door vocabulary and the diff's derived minimum door (`--body <file|->`, `--base <ref>`; blocking on missing, invalid, duplicate, or downgraded lines). |
| `pylon review release-surface` | Block when a pinned deployment consumes a public surface removed or changed against its pinned release tag (`--pinned-releases <dir>`, `--base <ref>` overrides pinning; blocking on removed, changed, unknown pins, or a missing tag). |
| `pylon gate pre-commit\|pre-push` | Run one gate stage in-process; the shipped hook shims call exactly this. Exists so external hook templates can chain the engine with one stable line. |
| `pylon review gate-surface` | Block when a diff touches a surface the checker itself relies on: suppression and closed-set manifests, the shipped hooks, the declared-schema templates, or a widened gate knob in `pylons.yaml` (`--base <ref>`, default `origin/main`; under `--ci` an unreadable base fails the run). |
| `pylon sync --detached` | Start a detached background fetch when the last one is over an hour old, then fast-forward onto `origin/main` on a clean tree; never blocks. |
| `pylon review-brief <plan>` | Generate `.pylon-review-brief.md` (`--stdout` prints it instead) or validate review packets (`--base <ref>`, `--validate`, `--validate-resolutions`). |
| `pylon protocol [list|<name>]` | List or render CLI protocols (`--template <path>` renders a custom template; `pylon reflect` is the hygiene pass; `pylon protocol agent` is the session loop). |
| `pylon reflect [--template <path>]` | Render the session retro protocol for the closing record. |
| `pylon extensions [list|schema]` | List built-in language extensions or print the external plugin schema (`--json` supported). |
| `pylon manifest [rules|packages|product]` | Generate or `--check` `manifests/rules.json` and `manifests/packages.json` (a `generate:` list in pylons.yaml disables either; a disabled manifest never regenerates and is never stale). `product` renders `manifests/product.json`, the release-time hash list of the product layer plus one pinned hash per platform binary (pass `--release-version <v>` to version it and one `--binary <os-arch>=<file>` per cross build). |
| `pylon explain <rule_id>` | Print rule description and fix hint for any `PYLON*` rule. |
| `pylon install` | Write `.githooks/` and enable `core.hooksPath`; idempotent. Refuses a pylon below the repository root, refuses to replace another tool's `core.hooksPath` unless `--force`, and reports hooks that drifted from the shipped assets. |
| `pylon init [dir]` | Scaffold a new pylon; wires gate hooks when run inside a git repository. |
| `pylon new <type> <name> [--title <t>] [--number]` | Create a document of a known type from the ledger's own template: zone placement, default status, and today's dates are filled, the name must be kebab-case (investigations gain the PYLON030 date prefix), and an existing file is never overwritten. |
