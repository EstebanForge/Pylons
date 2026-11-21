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
- **How it works**: Each entity declares `avoid:` anti-terms: the near-synonyms that must not name the concept. Documentation, reviews, and generated code stay on the canonical term.
- **Guard**: The closed value sets live in `conventions/vocabulary.md`. An entity with an empty `avoid:` list fires `PYLON039`.

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
- **How it works**: Each quirk carries mandatory sections: Symptom, Cause, Resolution. A `sources:` list anchors it to the exact files involved, and a `guard:` field declares its verification within 30 days (`PYLON028`). Two closed-token signals round out the record: `ai_authored_contributed:` (yes|no|unknown) and `volume_pressure:` (review|test|rollout|observability|none). A human declares them; the engine never infers them (`PYLON041`).
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
- **How it works**: One command at boot. It never blocks: it reports what it sees and exits 0, and a missing or broken configuration surfaces on stderr instead of failing your session. When the last `pylon sync --detached` background fetch failed, brief prints `FETCH-FAILED <time>` so the failure is never silent.
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

#### Vertical slices (`slice: true`)

- **What**: An opt-in plan flavor where one capability is the whole task, with declared scenarios as its acceptance criteria.
- **The pain**: A task that spans layers drifts: the route ships, the test never does, and the plan says done. Scope creep wins by default.
- **How it works**: A slice plan declares a behavior contract and a test surface (`PYLON045`), plus `given`/`when`/`expect` scenarios whose `covers:` names the contract ids they prove (`PYLON046`). The rules gate only accepted and in-progress plans; a scenario without all three parts proves nothing. Scenarios run in the mapped repo's QA suite; the engine only checks the declared coverage.
- **For agents**: `pylon context --slice <plan>` loads the slice and nothing else: its contracts with resolution state, linked quirks, and linked domain terms. Import edges surface as `PYLON043`: one advisory per undeclared target per package, where `depends_on:` names any package that owns the imported alias and sibling components stay internal. `pylon check` names every artifact that must move with the touched surface (`PYLON044`), matching glob contracts too and anchoring the finding to a real document.
- **Guard**: All slice rules are advisory. The ratchet budget excludes `PYLON043` because its count moves with sibling repos; everything else feeds `pylon review ratchet`.
- **Fail loud**: A plan document whose plan frontmatter cannot parse fires a FORCED `PYLON001` instead of silently losing its collisions and ripple.

#### The merge danger line (`pylon review merge-danger`)

- **What**: One required line in every pull request body, classifying the blast radius before review starts: `Merge danger: <one-way|two-way> | blast radius: <scope in one phrase>`.
- **The pain**: The merge decision is where production breakage gets decided, and nothing forced the classification. Reviewers skimmed past irreversible changes.
- **How it works**: The closed door vocabulary has two doors: `one-way` (migrations, backfills, irreversible writes, production contract changes) and `two-way` (easy revert). The gate validates the line and derives the minimum door from each project's own diff, so the pylon can live outside the code it judges: a `schema` contract change forces `one-way`, and a derived `one-way` may never be downgraded. A `one-way` door pairs with a breaking commit subject or a `#major` token in the diff, or a `Data migration:` note at a line start in the body. A diff the gate cannot read (shallow clone, missing base) surfaces an advisory instead of passing silently. CI runs `pylon review merge-danger --body <file|-> [--base <ref>]` before merge.
- **Guard**: Convention `conv.merge-danger-line` (`PYLON047`); blocking findings exit 1.

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

#### Remediation on every finding

- **What**: Every finding renders a `fix:` line sourced from the rule catalog.
- **The pain**: A rule ID names what broke, not what to do next. Humans and agents re-derive the same fix every time.
- **How it works**: The rendered `fix:` line and `pylon explain` read the same catalog hint. The json and sarif feeds carry a remediation field too.
- **Guard**: Explicit remediations always win; unknown rules render no hint.

#### Proof pointers (`verified_by:` / `falsified_by:`)

- **What**: Stable contracts of provable kinds demand a pointer to the test that proves or falsifies them.
- **The pain**: A stable contract with no test is a promise nobody plans to keep, and nothing says so until the code drifts.
- **How it works**: `symbol`, `route`, `cli`, `event`, `schema`, and their language-prefixed variants (`go-symbol`, `ts-export`) demand `verified_by:` or `falsified_by:` on accepted and in-progress plans (`PYLON040`). Resolution of the pointer stays `PYLON035`.
- **Guard**: `pylon review ratchet` holds the advisory count at the baseline.

#### Advisory ratchet (`pylon review ratchet`)

- **What**: A committed advisory budget per package and per zone, stored in `manifests/ratchet.json`.
- **The pain**: Advisories never block, so their count grows forever. Review debt is invisible until it is normal.
- **How it works**: `pylon review ratchet` compares today's advisory counts against the baseline. A rise is `FORCED` (`PYLON042`) and names the files behind it. `--update` re-baselines after review; a human commits the new budget.
- **Guard**: Clock-driven advisories (stale packages, audit dates, plan branches) sit outside the budget, so the gate never fails with zero edits.

#### Rework rate (`pylon review rework`)

- **What**: A per-project count of `fix:` and `refactor:` commits that touch paths a `feat:` commit changed.
- **The pain**: A hotspot repaired 5 times reads as 5 fixes in the log. Each one looks like progress; the map never moves.
- **How it works**: `pylon review rework` builds hot paths from feat commits in the window, orders commits newest first, and counts later fix and refactor commits that touch them. Hub files (lockfiles, changelogs) never count as hot. Report-only; exit 0.
- **Guard**: None. The number informs retro and planning; nothing gates on it.

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

### 3.6 Structural code search

`pylon aroma` answers a question the map cannot: which functions already do something like this, and what code do they carry with them? It indexes the corpus into featurized parse trees, finds the methods that structurally resemble a query, then clusters them and returns the code they share, with each line labeled. The query is the changed lines of a diff, a snippet, or a file range. Zero model calls, deterministic, advisory. The algorithm is the Aroma paper (OOPSLA 2019), ported from its reference implementation and pinned by differential goldens; the design record lives in the archive.

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
| `context --slice <plan>` | Prints only one plan's contracts with resolution state, linked quirks, and linked domain terms. | Starting a slice task | Missing plan exits 2 |
| `context --preamble` | Emits `context` plus the cache-friendly architecture preamble. | Agent prompt boot | Clean output |
| `map` | Projects corpus front matter as filterable one-line discovery records ranked by BM25. | Discovering docs before reading them | Usage errors exit 2 |
| `search <query>` | Full-text BM25 search across non-archived documents. | Finding docs by content | Usage errors exit 2 |
| `aroma search\|recommend` | Structural code similarity over featurized parse trees: methods resembling the query, then the code its results share. Query from `--lines path:a-b`, `--snippet <file|->`, or `--diff <file|->` (one query per method the lines touch); corpus is registered projects by default, else `--corpus`. Zero model calls. | Exploring unfamiliar code, orienting on a diff | Never; usage, unreadable corpus, or an empty corpus exit 2 |
| `review release-surface` | Pinned-release diff: block when a pinned deployment consumes a symbol, route, or cli surface removed or changed against its pinned tag; lockfiles under `--pinned-releases <dir>`, `--base <ref>` overrides pinning. | Release review, version-bump CI | Removed or changed consumed surfaces, unknown pin aliases, missing tags |
| `context --graph <package>` | Dependency view: actual import edges across registered projects, declared `depends_on`, undeclared and unused divergences; `--depth N` walks transitively. | Ownership checks, agent orientation | Never; a package no doc declares exits 2 |
| `context --diff-since <ref>` | Session capture: surfaces the diff touched whose docs did not move in the same range (committed or not); `--manifest` emits the JSON document, warnings name unreadable projects. | Session close, doc-drift checks | Never; usage errors exit 2 |
| `lint` | Validates frontmatter, lifecycle statuses, links, the dependency graph, and STE-100 prose. | Before commit / PR | Frontmatter, links, lifecycle errors |
| `lint --fix` | Auto-relocates misplaced docs and heals single-depth broken links. | Fixing lint errors | Unfixable errors |
| `lint --changed` | Pre-commit fast path: validates staged git files only. | Git pre-commit gate | Errors in staged files |
| `lint --parity` | Declared-vs-code set checks on top of the standard pass: domain `states:` against the enum at `states_symbol` (backing values when every case carries one, else case names) and route exposes against project source mentions (advisory). Opt-in flag. | Typed-contract drift in states and routes | Count and state mismatch error; an unresolvable enum, an unreadable case line, or a missing half stays advisory and never reads as parity |
| `lint --ci` | Strict mode: exits 1 on any finding, including advisories. | CI pipelines | Any finding |
| `contract <plan>` | Resolves a plan's `contract.stable` definitions against tracked repositories. | Pre-release verification | Unresolved contracts |
| `staleness [--package <pkg>]` | Flags package documents older than the age limit. | Release preparation | Stale documentation |
| `staleness --fix` | Syncs verified dates in extracted frontmatter only. | Release preparation | Stale documentation |
| `review plan <path>` | Validates plan or proposal structure and STE-100 compliance. | Proposal drafting | Invalid schema |
| `review postmortem [path]` | Validates quirk structure (Symptom, Cause, Resolution) and guard types. | Post-mortem authoring | Missing required sections |
| `review postmortem --check-sources` | Confirms every file in a quirk's `sources:` array exists in the mapped repos. | Post-mortem validation | Missing source files |
| `review ratchet [--update]` | Counts advisories per package and zone against the committed `manifests/ratchet.json` baseline. Rising counts block. | CI or pre-push | Advisory count above baseline |
| `review rework [--window] [--package]` | Counts `fix:` and `refactor:` commits touching paths a `feat:` commit changed, per registered project. Report-only. | Retro and planning | Never (skips unreadable projects) |
| `review annotations --from <file|->` | Converts lint findings (SARIF or JSON) into GitHub Check annotations plus a summary comment body; blocking errors only, capped at 50 with the total kept, hostile text sanitized. | CI PR feedback (the workflow posts; the engine emits) | Never; unparsable input exits 2 |
| `review-brief <plan>` | Assembles a self-contained review packet for fresh-context reviewers. | Peer review | Plan errors |
| `review-brief --validate <file>` | Stage-1 validation: verifies review completeness and empty evaluations. | Review submission | Incomplete reviews |
| `review-brief --validate-resolutions <f>` | Stage-2 validation: verifies every finding was resolved before merge. | Pre-merge verification | Unresolved findings |
| `manifest [packages\|rules]` | Compiles machine-readable JSON manifests. | Tooling build | Schema failure |
| `manifest --check` | Verifies the committed manifests match the compiled catalogs without writing. | CI verification | Catalog drift |
| `protocol [list\|<name>]` | Lists or renders shipped CLI protocols (`pylon reflect`, `pylon protocol agent`). | Session rituals | Unknown protocol |
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
| `--package <pkg>` | Scopes execution to a single package. | `staleness`, `review staleness`, `review rework`, `map` |
| `--update` | Rewrites the advisory baseline from a fresh lint run. | `review ratchet` |
| `--window <period>` | History window for rework counting: `90d`, `12w`, `1y`, or bare days. | `review rework` |
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
| `--corpus <dir>` | Adds a corpus root for indexing; repeatable. Default is the ledger's registered projects, else the working directory. | `aroma` |
| `--lines <path>:a-b` | Builds the query from file lines; comma-separated ranges and repeats allowed. | `aroma` |
| `--snippet <file\|->` | Builds one query from a snippet file or stdin. | `aroma` |
| `--diff <file\|->` | One query per unified-diff hunk, from the added lines of the checked-out files. | `aroma` |
| `--min <float>` | Results must be more similar than this. Search defaults 0.71, recommend 0.65. | `aroma` |
| `--min-support <n>` | Fewest methods behind a recommendation. 2 by default; 1 reproduces the paper's behavior. | `aroma` |
| `--min-query-size <n>` | Drops queries with fewer features. Default 0. | `aroma` |
| `--include-tests` | Keeps `_test.go` files in the corpus. | `aroma` |
| `--lang <id>` | Picks the language front end for stdin snippets. Only `go` ships today. | `aroma` |
| `--graph <package>` | Names the package the dependency view starts from. | `context --graph` |
| `--depth N` | Walk depth for the graph; 1 covers the package's own components. | `context --graph` |
| `--from <file|->` | The lint findings document (SARIF or JSON); `-` reads stdin. | `review annotations` |
| `--path-prefix <p>` | Prefix for annotation paths when the KB lives in a repo subdirectory. | `review annotations` |
| `--pinned-releases <dir>` | Deployment directory holding `<alias> <version>` lockfiles; the pinned version resolves to a release tag. | `review release-surface` |
| `--diff-since <ref>` | Compares history since the ref against declared surfaces. | `context` |
| `--manifest` | Emits the session-capture manifest as JSON. | `context --diff-since` |

### 5.3 Exit codes

| Code | Meaning | Action required |
|---|---|---|
| `0` | Success or advisory-only findings. | Proceed with the workflow. |
| `1` | Blocking finding (`FORCED` collision, broken contract, lint error, ratchet regression). | Fix the violation or coordinate with the conflicting plan owner. |
| `2` | Usage or runtime setup error (unknown command, invalid flag or window, unreadable baseline, unresolvable package filter). | Check syntax via `pylon --help`. |
| `3` | Environment failure (no `manifests/pylons.yaml` found). | Run from the ledger root or pass `--pylons <path>`. |
