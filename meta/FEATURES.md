---
title: "Pylons: Feature Guide"
type: meta
status: active
created: 2026-11-20
updated: 2026-11-20
tags: [features, workflow, agents, developer-guide]
---

# Pylons: Feature Guide

This file is the feature guide for the Pylons ledger and its `pylon` CLI. It doubles as the pitch, because both jobs need the same answers. For every feature you get 4 answers: what it is, what hurts today, how the pain goes away, and which command enforces the fix.

Two reading paths. If you manage people or products, read sections 1 and 2, then skim the feature cards in section 3. If you write code here, or an agent writes it for you, the whole file applies, and section 5 is the command reference you keep open.

Sources of truth: [`../README.md`](../README.md) (overview), [`../AGENTS.md`](../AGENTS.md) (rules and CLI manual), [`MANIFESTO.md`](MANIFESTO.md) (design philosophy), [`USAGE.md`](USAGE.md) (daily workflow), [`ROADMAP.md`](ROADMAP.md) (future work). Fix drift in the source files first, then mirror here.

## 1. The problem we built Pylons for

A platform of many repositories. Each repository moves on its own clock and depends on several of the others. That setup is normal for a platform of any size. The pain comes from the parallelism. One team changes a package, another team builds on it, and nobody finds out until something breaks at the joins.

Four failures keep coming back:

- **Architectural drift**: someone improvises a pattern, the code stops matching the agreed design, and the divergence ships as permanent debt.
- **Branch collisions**: parallel work quietly edits the same surface. You find out in staging, at the worst possible hour.
- **Opaque boundaries**: a developer guesses which interfaces are safe to build on. The guess is wrong, a refactor breaks it, and the breakage crosses repositories.
- **Agent amnesia**: every agent session starts from zero and reverse-engineers the system from scratch, with total confidence.

Agents poured gas on all four. An agent writes in minutes what a human writes in days, and it wakes up knowing nothing about the platform. Left to guess, it invents a pattern that already exists three files over, or leans on an interface somebody is halfway through rewriting. Confidence is free for them. The review bill is not.

We checked our own house before believing any of this. An audit of our own merge history found that most shipped work had lost the link to the plan that justified it. One large feature shipped complete while its plan sat unarchived with an empty `artifact:` field. Multiply that by every branch in flight, and documentation drifts into fiction within months.

## 2. The idea: a platform that knows itself

Pylons is a map and a checker. The map: every package, contract, decision, and footgun, written in files a machine can parse. The checker: a Go binary that reads the map and proves the code still matches it, on every commit, with zero model calls.

Code stays the source of truth. Pylons is the map, and the map stays honest because nothing writes to it unchecked.

### Where state lives

| Domain | System of record | Responsibility |
|---|---|---|
| Project execution | The team's tracker | Milestones, task sizing, delivery |
| Architecture and contracts | Pylons | Boundaries, public surfaces, guardrails, decisions, quirks |
| Code implementation | Git | Source of truth for all production code |
| Verification and tests | Each mapped repo's QA suite | Proving the behavior the map declares |
| Versioning and releases | Each repo's release flow | Semantic versioning, changelogs, tags |

### What each reader gets

| If you are | What changes for you |
|---|---|
| An engineer | You know which surfaces are safe before you build. The CLI names the collision before the first line of code, not after the merge. |
| A lead or manager | Features start from a written, sized plan with typed contracts. Risk surfaces at planning time, where it is cheap, instead of in staging, where it is not. |
| The team | Architecture knowledge survives departures. Agents multiply the team instead of multiplying the failure modes. |

## 3. The features

Every card answers in the same order: **What** names the feature, **The pain** names the hurt, **How it works** shows the fix, and **Guard** names the command that enforces it. The groups follow the outcome, so you can pick the pain you care about and read only that group.

### 3.1 Know what is safe to build on

#### Package maps (`packages/`)

- **What**: A one-file map per platform package: its role, its dependencies (`depends_on:`), its public surfaces (`exposes:`), machine-owned paths (`protected:`), and off-limits internals (`internal:`).
- **The pain**: In a multi-repo platform, nothing marks the line between public API and internal detail. Code reaches into another package's private class, a refactor breaks it, and the fix crosses 3 repositories.
- **How it works**: The map is YAML front matter in `packages/<slug>.md`. The lists are explicit, so both humans and agents know exactly what is in bounds.
- **For agents**: Agents read the map before planning and consume only declared surfaces. When an agent targets a protected path, `pylon check` names the violation.
- **Guard**: `pylon lint` resolves package references against registered projects (`PYLON003`) and flags unresolved `exposes:` entries. `pylon contract <plan>` proves a plan's stable contracts against live code.

#### Typed contracts and falsifiers (`plans/`)

- **What**: A contract is a written promise about a piece of code: its name, what it does, and whether other code may depend on it. Every plan declares its contracts with a status: `contract.stable` (safe to build on), `contract.unstable` (moving soon, ask first), `contract.editing` (somebody is in there right now).
- **The pain**: Two branches touch the same in-flight interface. One renames a symbol, the other merges against the old name, and staging fails days later. Nobody planned the collision; it just happened.
- **How it works**: Contracts are typed across 7 kinds: `file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`. Every stable contract carries a `wrong_if`: a falsifiable statement of what change would break the promise. A contract you cannot falsify is a wish, so accepted plans do not take those (`PYLON014`).
- **For agents**: `pylon check <paths>` before editing. If a path intersects `contract.editing` or `contract.unstable`, the CLI emits a blocking stop signal, and the agent stops instead of writing code on contested ground.
- **Guard**: `pylon contract <plan>` resolves every stable contract against tracked repositories. A missing symbol, route, or file fails validation.

#### Ubiquitous language (`domain/`)

- **What**: The canonical vocabulary of the platform: entities, nouns, and verbs, defined once in the domain zone.
- **The pain**: "Membership", "plan", "tier", and "account" all sound interchangeable. Teams and agents mix them, and you get class names, database fields, and route parameters that name the same idea 3 different ways.
- **How it works**: One term per concept. Documentation, plans, and reviews use the canonical term, so a `Membership` in the docs is the same `Membership` in the schema.
- **Guard**: The closed value sets live in `conventions/vocabulary.md`. The roadmap item `avoid-lists` adds an anti-name guard beside each term.

#### Transaction flows (`flows/`)

- **What**: Sequence diagrams for the transactions that cross packages and services, end to end.
- **The pain**: From one repository's code you cannot see a whole transaction. A developer or agent misses a lifecycle step and ships a partial state update that only fails under real data.
- **How it works**: Each flow documents actors, entry points, writes, dispatched events, and external calls for one lifecycle.
- **For agents**: Agents review the complete lifecycle before modifying any single package in it.
- **Guard**: `pylon lint` validates structure, frontmatter, and links.

#### Conventions (`conventions/`)

- **What**: Platform-wide patterns for the recurring jobs: helpers, boundaries, models, release automation. How things are built here, written down once.
- **The pain**: Without a conventions layer, every developer rediscovers a registration pattern from a random older repository, and the platform accumulates 4 dialects of the same idea.
- **How it works**: Convention docs live in `conventions/` (proposals in `conventions/proposals/`), and `pylon context` surfaces them alongside contracts when your paths match.
- **Guard**: Lint validates structure and links. Agents receive them as advisory context, not trivia.

#### Cross-linked, machine-resolvable references

- **What**: Two URI schemes across the whole ledger. `repo://<project>/<path>` points into a registered repository and resolves against its declared branch. `pylon://<path>` points at any ledger document.
- **The pain**: Documentation lives in a separate repository from code. A developer or agent editing a function does not know which conventions, rules, decisions, or quirks govern that code without manual search.
- **How it works**: Ledger documents link to the code they govern through `repo://`, and code-side knowledge links back through `pylon://`. Resolvers never guess: `repo://` with a named `ref:` resolves that ref, and a ref that does not resolve fails closed.
- **For agents**: When a document names a `repo://` path, an agent resolves it before editing. When code mentions a `pylon://` doc, the agent reads it first.
- **Guard**: `pylon lint` rejects raw machine paths in prose (`PYLON033`) and validates that contract source references resolve.

### 3.2 Never learn the same lesson twice

Every incident has 2 costs: the outage and the relearning. The features in this group delete the second cost.

#### The quirks ledger (`quirks/`)

- **What**: A ledger of verified incidents and integration traps, each with its root cause and its fix.
- **The pain**: Platform bugs get fixed, forgotten, and reintroduced months later by a different person or a fresh agent with no memory of the first round. The team pays for the same bug repeatedly.
- **How it works**: Each quirk carries mandatory sections: Symptom, Cause, Resolution. A `sources:` list anchors it to the exact files involved, and a `guard:` field declares its verification within 30 days (`PYLON028`).
- **For agents**: When an agent touches a file listed in a quirk's `sources:`, `pylon context` injects the guardrail automatically. The trap costs one write-up, then it defends itself.
- **Guard**: `pylon review postmortem --check-sources` fails when an anchored file no longer exists, so dead quirks surface immediately.

#### Investigations (`investigations/`)

- **What**: Structured diagnostic records for complex, multi-package bugs: symptom, reproduction, fail-first commands, root cause, blast radius, and resolution evidence.
- **The pain**: Hard bugs get solved in chat threads and closed tickets, and the knowledge dies there. The next similar bug restarts from zero.
- **How it works**: The investigation document lives beside the work, before and during the fix (`YYYY-MM-DD-<slug>.md`, `PYLON030`). Investigations stay contract-free. Terminal records move to `investigations/archive/`. Findings with recurring traps promote into `quirks/` with a verified guard.
- **Guard**: `pylon lint` enforces the lifecycle statuses and folder placement.

#### Decision records (`decisions/`)

- **What**: Architecture decision records. Once accepted, a decision is immutable; a change of direction means a new record that supersedes the old one.
- **The pain**: Architectural churn. Teams and agents re-propose patterns that were already evaluated and rejected, and senior people re-litigate settled questions.
- **How it works**: The decision history is an append-only ledger. Proposals start in `decisions/proposals/`; accepted records are numbered `DNNNN-<slug>.md` (`PYLON010`). Superseded records move to `decisions/archive/`, dated and readable, so the "why" behind the platform stays answerable.
- **For agents**: The ledger stops agents from re-arguing settled choices with fresh confidence on every run.
- **Guard**: Lint enforces the numbering, statuses, and archive placement.

#### Tickets (`tickets/`)

- **What**: Bugs, tasks, and improvements tracked inside the ledger, one file each, linked from the work they gate.
- **The pain**: Execution state lives in an external tracker the ledger cannot see. Plans ship while their tickets say open, and nobody notices until the tracker and the map disagree.
- **How it works**: Plans and other documents carry a `ticket:` list pointing at their tracker items. Completed tickets close with status `done` and move to `tickets/archive/`.
- **Guard**: `pylon lint` requires ticket references on accepted plans (`PYLON016`) and enforces the ticket lifecycle.

### 3.3 Agents that cannot wander

An agent with the wrong context ships the wrong thing fast. These features feed it the right context and stop it when it strays.

#### Session brief (`pylon brief`)

- **What**: A compact summary at session start: the pylon root, the registered projects, the enabled extensions, and the corpus size.
- **The pain**: Engineers and agents start work blind. Nobody knows whether the clone they are about to trust is registered, configured, or stale.
- **How it works**: One command at boot. It never blocks: it reports what it sees and exits 0, and a missing or broken configuration surfaces on stderr instead of failing your session.
- **For agents**: The counts tell an agent the ledger is wired before it plans against it. Deeper state lives one command away in `map` and `check`.

#### Point-of-work context (`pylon context`)

- **What**: The minimal relevant architectural slice for the files about to change: matching contracts, active collisions, quirk guardrails, conventions, and anti-patterns.
- **The pain**: Stuffing whole repository directories into a context window burns tokens, slows generation, and degrades attention. The model drowns instead of reading.
- **How it works**: Given target paths, it emits only what applies to those paths, cutting prompt tokens sharply while putting exact constraints at the point of work. `--preamble` prepends a cache-friendly architecture preamble for session boots.
- **For agents**: This is the difference between an agent that guesses and an agent that reads the rules for exactly the code it is about to touch.

#### Stop blocks and protected paths (`pylon check`)

- **What**: Hard stops for agents and humans. Package maps declare `protected:` globs; touching one is a `FORCED` finding. `--stop-block` renders the halt as a structured block an agent cannot misread.
- **The pain**: An autonomous agent editing machine-generated config, vendored manifests, or a security boundary without authorization is how quiet incidents start.
- **How it works**: `check <paths>` intersects your paths with every active plan's `contract.editing` and `contract.unstable` sets, evaluates `wrong_if` falsifiers, and matches `protected:` globs.
- **For humans**: The same command answers the daily question "is anyone else in here right now?" before you write a line.

#### Discovery (`pylon map`, `pylon search`)

- **What**: One line per document in the ledger, filterable by type, package, tag, folder, status, and query, ranked by BM25 relevance. Full-text search across the corpus beside it.
- **The pain**: A ledger of hundreds of documents is useless without discovery. People re-ask answered questions and re-write conventions that already exist.
- **How it works**: `map` walks document front matter at runtime and caps human output at a token budget with a footer that says how much it hid. `--json` is never truncated. `--all` includes the archive subtrees.
- **For agents**: A whole-corpus overview in one cheap read, so agents retrieve existing solutions instead of scanning file trees or guessing.

### 3.4 Proof, not promises

Documentation you cannot verify is a rumor with formatting. This group makes every claim checkable, deterministically.

#### Zero model calls

- **The pain**: Verification that depends on a model gives a different verdict every run, costs per call, fails offline, and can be argued with.
- **How it works**: The engine is a Go binary. Every check is AST parsing, YAML schema validation, git inspection, or exact set intersection. Same input, same verdict, on your laptop and in CI.
- **For agents**: A deterministic error signal cannot be negotiated with. An agent cannot charm an AST resolver.
- **Guard**: External `pylon-ext-<name>` binaries are opt-in, namespace-gated, and validated in configuration; the core checks never leave the deterministic path.

#### Gate hooks (`pylon install`)

- **What**: Shipped pre-commit and pre-push hooks wired with one command. The commit gate lints staged files; the push gate checks manifest freshness, full-corpus lint, and quirk source integrity.
- **The pain**: A checker that runs only when someone remembers is a checker that runs once a quarter. Drift lands in history before anyone looks.
- **How it works**: `pylon install` writes `.githooks/` and sets `core.hooksPath`, idempotently. The hooks fail closed: no `pylon` binary means a blocked gate, with an explicit `PYLON_HOOKS_OPTIONAL=1` opt-out. `pylon init` wires the hooks itself inside a git repository.
- **Guard**: The installer refuses a pylon below a repository root and refuses to clobber another tool's hook configuration unless `--force`.

#### Controlled language (ASD-STE100)

- **The pain**: Wordy, ambiguous documentation burns context tokens and reads differently to every session, so two agents "obey" the same doc two different ways.
- **How it works**: Pylons mandates ASD-STE100, the aerospace controlled-English standard: short sentences, active voice, closed vocabulary, hard sentence caps per document. It reads identically to a human and to a model.
- **Guard**: `pylon lint` enforces sentence limits, voice, and vocabulary across the corpus. `--stats` reports compliance per document.

#### Two grades only: `FORCED` and `ADVISORY`

- **The pain**: Linters that cry wolf get ignored by humans and halt agents on trivia. Warning fatigue kills signal.
- **How it works**: Findings carry exactly 2 grades. `FORCED` exits 1: structural failure, broken contract, active collision, protected path. `ADVISORY` exits 0: context, style notes, optional guidance.
- **For agents**: The stopping rule is mechanical: halt on `FORCED`, read and proceed on `ADVISORY`. No false-positive deadlocks. CI fails only on the real ones.

#### Corpus health (`pylon lint`)

- **The pain**: Documentation rots quietly: links decay, files land in the wrong folders, front matter drifts from the schema.
- **How it works**: Lint validates frontmatter, lifecycle statuses, cross-document links, the dependency graph, and prose against the rule catalog. `--fix` auto-relocates misplaced documents and heals broken links. `--changed` keeps the pre-commit gate fast.
- **Guard**: `pylon explain <rule_id>` prints any rule's meaning and fix hint in plain language, so humans and agents self-correct instead of guessing.

#### Staleness (`pylon staleness`)

- **The pain**: Code moves every release while docs sit still, and the map drifts into fiction one minor version at a time.
- **How it works**: Compares each package document's `updated:` date against its age limit and the repository's history. `--fix` syncs verified dates in frontmatter only.
- **Guard**: Stale documentation becomes a visible, named finding instead of a slow surprise.

#### Machine feeds (`pylon manifest`)

- **The pain**: External tools re-parse Markdown and duplicate extraction logic in every language that touches the platform.
- **How it works**: `manifest` compiles validated JSON artifacts (`packages.json`, `rules.json`) from the source documents. `manifest --check` proves the committed JSON matches the compiled catalogs, so CI catches drift.
- **Guard**: The pre-push gate runs the check on every push.

#### Review packets (`pylon review-brief`)

- **The pain**: A fresh reviewer (human or agent) must reassemble context from PRs, tickets, and plans, and reviewers with partial context approve partial work.
- **How it works**: For one plan, it assembles a self-contained packet: contracts with resolution state, per-repo diff stats, package map excerpts, and a coverage grid. Returned reviews pass 2 validation stages: `--validate` proves every row was evaluated, `--validate-resolutions` proves every finding was resolved before merge.
- **For agents**: Peer review becomes a reproducible gate a fresh-context agent can run with deterministic completeness checks.

#### Installable ledger

- **What**: A one-command installer that downloads a released binary and scaffolds a fresh ledger with its own git repository, wired hooks, and verified checksums.
- **The pain**: A map that requires a build chain to install gets skipped. Adoption dies in the setup instructions.
- **How it works**: `install.sh` detects the platform, downloads the release artifacts, verifies them against `checksums.txt`, extracts into `./pylons`, runs `git init`, wires the gate hooks, and scaffolds an initial commit. Windows users download the binary directly.
- **For agents**: Nothing to build. The agent registers repositories in `pylons/manifests/pylons.yaml` and starts checking against real code.

### 3.5 The rest of the ledger

The cards above cover the systems with commands and grades. The rest of the ledger is reference material: zones with no CLI of their own, still part of what developers and agents read. One line each, so this guide stays honest about the whole map.

| Zone | Holds | Who reads it |
|---|---|---|
| `templates/` | One schema per document type. Every new document starts here. | Anyone writing a doc |
| `proposals/` | Plans and conventions under review, before they graduate to their home zones. | Reviewers, plan authors |
| `references/` | Third-party API docs and external hooks the platform relies on. | Developers and agents extending integrations |
| `prompts/` | Shared raw agent prompts, reusable across the platform. | Agents and their operators |
| `skills/` | Reusable team skills for agent environments. | Agents |
| `audits/` | Per-package and test-suite audit records. | Reviewers, leads |
| `meta/` | Documentation about Pylons itself: manifesto, feature guide, roadmap, changelog, CLI manual. Start at [`meta/INDEX.md`](meta/INDEX.md). | Everyone |
| `manifests/` | Compiled JSON artifacts the CLI emits and CI checks. | Tooling |

## 4. The loop

The features only pay off if the loop closes: plan, build, check, ship, write back.

### 4.1 Developer workflow

```
1. Start Session      -> pylon brief (confirm the ledger is wired)
2. Doc Discovery      -> pylon map --type quirk --package <pkg> (surface known footguns and conventions)
3. Pre-Code Check     -> pylon check <paths> (verify no contract collisions)
4. Context Retrieval  -> pylon context <paths> (review contracts and quirks)
5. Implementation     -> Write code and tests in the mapped repo's QA suite
6. Commit Gate        -> pylon lint --changed (fast staged linting)
7. Ship and Archive   -> Merge PR, set the plan's artifact: link, move it to the archive
```

### 4.2 Agent workflow

```
1. Bootstrapping      -> Read AGENTS.md; run pylon brief to inspect the ledger state
2. Collision Gate     -> Run pylon check --stop-block <paths>; halt immediately on FORCED findings
3. Guardrail Loading  -> Run pylon context --preamble <paths> to load constraints and quirks
4. Execution          -> Surgical edits matching codebase style; resolve repo:// references before editing
5. Verification       -> Run the mapped repo's QA suite; run pylon lint to verify zero blocking errors
6. Gate Submission    -> Request human review with pylon review-brief as the packet
```

### 4.3 Closing the upstream/downstream gap

The audit behind the drift findings found one dominant failure mode: thorough plans at design time, orphaned plans after delivery. The fix is procedural, and it is already in the workflow:

1. **Mandatory PR reference**: every pull request links its plan.
2. **Ship step discipline**: at merge sign-off, the plan gains a terminal status, a shipped `artifact:` link, and a move to `plans/archive/` (`PYLON009`).
3. **Automated verification**: `pylon staleness` and the gate hooks confirm that shipped code reflects the final documented state.

Humans review and approve every change. Agents do the heavy lifting. Pylons leaves the coding to both; its job is keeping that code aligned with the architecture already agreed on.

## 5. Reference: command dictionary

Run commands through the `pylon` binary in the ledger root (`./bin/pylon` from a built checkout, `pylons/bin/pylon` from an installed ledger). Exit code 3 means no `manifests/pylons.yaml` was found; every other command's contract is in the table.

### 5.1 Complete command dictionary

| Command | Purpose | When to run | Exit code 1 on |
|---|---|---|---|
| `brief [--pylons <path>]` | Non-blocking session summary: root, projects, extensions, corpus size. | Session boot | Never (exit 3 without a ledger root) |
| `check <paths...>` | Intersects target paths with `contract.editing`, `contract.unstable`, and `protected:` globs. Evaluates `wrong_if` falsifiers. | Before editing code | Any `FORCED` collision or protected path |
| `check --stop-block` | Renders `FORCED` check findings as structured agent stop blocks. | Agent session gate before writing code | Any `FORCED` collision |
| `context <paths...>` | Emits token-efficient architectural context: contracts, collisions, quirk guardrails, conventions. | Before file edits | Clean output |
| `context --preamble` | Emits `context` plus the cache-friendly architecture preamble. | Agent prompt boot | Clean output |
| `map` | Projects corpus front matter as filterable one-line discovery records ranked by BM25. | Discovering docs before reading them | Usage errors exit 2 |
| `search <query>` | Full-text BM25 search across non-archived documents. | Finding docs by content | Usage errors exit 2 |
| `lint` | Validates frontmatter, lifecycle statuses, links, the dependency graph, and STE-100 prose. | Before commit / PR | Frontmatter, links, lifecycle errors |
| `lint --fix` | Auto-relocates misplaced docs and heals single-depth broken links. | Fixing lint errors | Unfixable errors |
| `lint --changed` | Pre-commit fast path: validates staged git files only. | Git pre-commit gate | Errors in staged files |
| `lint --stats` | Emits STE-100 suppression and violation report. Report-only. | Style audit | Always exits 0 |
| `lint --ci` | Strict mode: exits 1 on any finding, including advisories. | CI pipelines | Any finding |
| `contract <plan>` | Resolves a plan's `contract.stable` definitions against tracked repositories. | Pre-release verification | Unresolved contracts |
| `staleness [--package <pkg>]` | Flags package documents older than the age limit. | Release preparation | Stale documentation |
| `staleness --fix` | Syncs verified dates in extracted frontmatter only. | Release preparation | Stale documentation |
| `review plan <path>` | Validates plan or proposal structure and STE-100 compliance. | Proposal drafting | Invalid schema |
| `review postmortem [path]` | Validates quirk structure (Symptom, Cause, Resolution) and guard types. | Post-mortem authoring | Missing required sections |
| `review postmortem --check-sources` | Confirms every file in a quirk's `sources:` array exists in the mapped repos. | Post-mortem validation | Missing source files |
| `review-brief <plan>` | Assembles a self-contained review packet for fresh-context reviewers. | Peer review | Plan errors |
| `review-brief --validate <file>` | Stage-1 validation: verifies review completeness and empty evaluations. | Review submission | Incomplete reviews |
| `review-brief --validate-resolutions <f>` | Stage-2 validation: verifies every finding was resolved before merge. | Pre-merge verification | Unresolved findings |
| `manifest [packages\|rules]` | Compiles machine-readable JSON manifests. | Tooling build | Schema failure |
| `manifest --check` | Verifies the committed manifests match the compiled catalogs without writing. | CI verification | Catalog drift |
| `protocol [list\|<name>]` | Lists or renders shipped CLI protocols (for example `pylon reflect`). | Session rituals | Unknown protocol |
| `extensions [list\|schema]` | Lists built-in language extensions and the custom-extension schema. | Enabling code extraction | Unknown extension |
| `explain <rule_id>` | Prints rule name, severity, auto-fixability, description, and fix hint. | Fixing lint failures | Unknown rule ID |
| `install [--pylons <path>] [--force]` | Writes `.githooks/`, enables `core.hooksPath`, repairs mode bits. Idempotent. | One-time ledger wiring | Nested pylon, hook conflict, missing binary |
| `init [dir]` | Scaffolds a fresh ledger: zones, templates, manifest, gitignore. Wires hooks inside a git repo. | Starting a new pylon | Existing manifest |
| `--version` | Prints the CLI version. | Release checks | Never |

### 5.2 Flags dictionary

| Flag | Purpose and effect | Applicable commands |
|---|---|---|
| `--format <format>` | Sets output format: `human` (default), `json`, or `sarif` (findings commands only). | `check`, `lint`, `contract`, `review *`, `staleness` |
| `--json` | Alias for `--format json`. | `check`, `lint`, `contract`, `review *`, `brief`, `context`, `map` |
| `--ci` | Strict mode: exits with code 1 on any finding, including advisories. | `lint`, `contract` |
| `--fix` | Applies supported fixes (file moves, frontmatter date sync). | `lint`, `staleness` |
| `--changed` | Fast pre-commit mode: inspects only staged git files. | `lint` |
| `--stop-block` | Formats `FORCED` collision findings as agent stop blocks. | `check` |
| `--preamble` | Prepends the cache-friendly architecture preamble. | `context` |
| `--stats` | Emits the STE-100 suppression and violation report. Report-only; exits 0. | `lint` |
| `--check-sources` | Verifies that all paths declared in quirk `sources:` exist. | `review postmortem` |
| `--check` | Verifies the committed manifests match the compiled catalogs without writing. | `manifest` |
| `--package <pkg>` | Scopes execution to a single package. | `staleness`, `review staleness`, `map` |
| `--pylons <path>` | Explicit path to the ledger, bypassing auto-discovery. | All ledger commands |
| `--stdout` | Prints the generated review brief to stdout instead of writing a file. | `review-brief` |
| `--validate <file>` | Validates that a returned review packet contains required evaluations. | `review-brief` |
| `--validate-resolutions <f>` | Validates that all findings in a review packet are resolved. | `review-brief` |
| `--base <ref>` | Base git ref for diff comparisons. | `review-brief` |
| `--force` | Overwrites a conflicting `core.hooksPath` and drifted hook files. | `install` |
| `--type <types>` | Filters documents by comma-separated type list. | `map` |
| `--tag <tags>` | Filters documents by comma-separated tag list. | `map` |
| `--folder <dir>` | Filters documents by folder path. | `map` |
| `--query <q>` | Free-text relevance filter. | `map` |
| `--status <st>` | Filters documents by status. | `map` |
| `--limit <n>` | Caps result count. | `map`, `search` |
| `--all` | Includes archive subtrees and non-corpus directories in results. | `map`, `search` |

### 5.3 Exit codes

| Code | Meaning | Action required |
|---|---|---|
| `0` | Success or advisory-only findings. | Proceed with the workflow. |
| `1` | Blocking finding (`FORCED` collision, broken contract, lint error). | Fix the violation or coordinate with the conflicting plan owner. |
| `2` | Usage error (unknown command, invalid flag, missing argument). | Check syntax via `pylon --help`. |
| `3` | Environment failure (no `manifests/pylons.yaml` found). | Run from the ledger root or pass `--pylons <path>`. |
