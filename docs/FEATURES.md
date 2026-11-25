---
title: "Pylons: Feature Guide"
type: meta
status: active
created: 2026-11-20
updated: 2026-11-25
tags: [features, workflow, agents, developer-guide]
---

# Pylons: Feature Guide

This file is the feature guide for the Pylons ledger and its `pylon` CLI. It doubles as the pitch, because both jobs need the same answers. For every feature you get 4 answers: what it is, what hurts today, how the pain goes away, and which command enforces the fix.

Two reading paths. If you manage people or products, read sections 1 and 2, then skim the feature cards in section 3. If you write code here, or an agent writes it for you, the whole file applies, and section 5 is the command reference you keep open.

Sources of truth: [`../README.md`](../README.md) (overview), [`../pylons/AGENTS.md`](../pylons/AGENTS.md) (ledger rules and CLI manual), [`MANIFESTO.md`](MANIFESTO.md) (design philosophy), [`USAGE.md`](USAGE.md) (daily workflow), [`ROADMAP.md`](ROADMAP.md) (future work). Fix drift in the source files first, then mirror here.

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
- **For agents**: `pylon check <paths>` before editing. If a path intersects `contract.editing` or `contract.unstable`, the CLI emits a blocking stop signal, and the agent stops instead of writing code on contested ground. When the agent works that plan, `check --plan <plan>` holds the scope of the plan as its own, and every other plan still stops it.
- **Guard**: `pylon contract <plan>` resolves every stable contract against tracked repositories. A missing symbol, route, or file fails validation.

#### The plan lifecycle (`plans/`)

- **What**: A plan moves through a closed status set: `proposed`, `accepted`, `in-progress`, `paused`, `shipped`, `abandoned`. Each status carries its own obligations.
- **The pain**: Started work stops mid-flight and nothing records where it stopped. The next session re-derives the state from diffs, or restarts the work.
- **How it works**: Proposals under review get the full structural review (`PYLON021`) and the type-prefix filenames (`PYLON038`). In-progress plans name their trace: a feature branch or an `artifact:` link (`PYLON027`). A paused plan carries a `## State` section naming the resume point: what is done, the next step, any blocker (`PYLON065`). Paused work is not held to live contracts; the checks keep their accepted and in-progress scope. Shipping closes the loop: terminal status, `artifact:` link, move to `plans/archive/` (`PYLON009`).
- **Guard**: `pylon lint` enforces the statuses, folder placement, and the section rules above. `pylon map --type plan --status in-progress` lists active work.

#### Ubiquitous language (`domain/`)

- **What**: The canonical vocabulary of the platform: entities, nouns, and verbs, defined once in the domain zone.
- **The pain**: "Membership", "plan", "tier", and "account" all sound interchangeable. Teams and agents mix them, and you get class names, database fields, and route parameters that name the same idea 3 different ways.
- **How it works**: One term per concept. Documentation, plans, and reviews use the canonical term, so a `Membership` in the docs is the same `Membership` in the schema.
- **How it works**: Each entity declares `avoid:` anti-terms: the near-synonyms that must not name the concept. Documentation, reviews, and generated code stay on the canonical term.
- **Guard**: The closed value sets live in `manifests/vocabulary.json`. An entity with an empty `avoid:` list fires `PYLON039`.

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
- **How it works**: The investigation document lives beside the work, before and during the fix (`YYYY-MM-DD-<slug>.md`, `PYLON030`). Audit records (`type: audit`) share the zone with their own rules: findings-table row statuses (`PYLON018`) and reaudit dates (`PYLON026`). Investigations stay contract-free. Terminal records of both shapes move to `investigations/archive/`. Findings with recurring traps promote into `quirks/` with a verified guard.
- **Guard**: `pylon lint` enforces the lifecycle statuses and folder placement.

#### Capture and promotion (`pylon harvest pr`, `pylon review retro`)

- **What**: A merge-time capture pass and a retro pass that turn repeated review findings into ledger candidates.
- **The pain**: The lesson lives in the merged PR and dies in the review thread, so the same comment returns on the next PR.
- **How it works**: `harvest pr` reads a merged diff: exported symbols become `exposes:` proposals for the claiming package, `Footgun:` markers become quirk stubs, and `--record` appends each marker as a JSONL finding. `review retro` counts finding signatures in a window and opens exactly one promotion candidate per repeated signature: a convention stub, a `wrong_if` falsifier entry, or a lint-rule proposal.
- **For agents**: The loop needs no session memory: the record accumulates in files, and a human accepts or drops each candidate.
- **Guard**: Report-only commands. Malformed records fail closed.

#### Knowledge utility (`pylon review utility`)

- **What**: A deterministic utility score per document, from citations the corpus declares and injections the engine records.
- **The pain**: The quirks ledger only grows. Obsolete traps inject into every session forever, and proven patterns never rise from quirks into formal conventions.
- **How it works**: An agent session that closes with a citation comment (`<!-- pylon:referenced-doc: quirks/<file>.md -->`) names the document that shaped its work. The engine records one injection every time `pylon context` delivers a document: finding source docs, plain-context quirk guardrails, or slice-linked quirks. The score is citations over injections, every event decayed on a 30-day half-life.
- **Guard**: `--prune` moves quirks delivered on at least two distinct days whose uncited weighted mass clears 3.0 to `quirks/archive/`; `--promote` opens one convention proposal per document cited 3 or more times from more than one place. Both need evidence: absence of telemetry never prunes, one heavy day is not repeated use, deleted documents never score, and the engine never rewrites an existing archive entry or stub.

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

#### Detached freshness (`pylon sync`)

- **What**: A background fetch that never blocks the session, with the fast-forward on a later clean run.
- **The pain**: A long-lived checkout drifts behind origin, and the checks run against a stale map.
- **How it works**: `sync --detached` records a timestamp, fetches `origin main` in the background, and fast-forwards `main` only when the tree is clean on a later run. A failed fetch surfaces in `pylon brief` as `FETCH-FAILED <time>`.
- **Guard**: Diverged or dirty trees are never merged.

#### Point-of-work context (`pylon context`)

- **What**: The minimal relevant architectural slice for the files about to change: matching contracts, active collisions, quirk guardrails, conventions, and anti-patterns.
- **The pain**: Stuffing whole repository directories into a context window burns tokens, slows generation, and degrades attention. The model drowns instead of reading.
- **How it works**: Given target paths, it emits only what applies to those paths, cutting prompt tokens sharply while putting exact constraints at the point of work. Quirks whose `repo://` sources cover a target (exact path, or the source path as a directory ancestor) surface as quirk guardrails in the human output; targets need project attribution (`repo://` URIs or absolute paths inside tracked projects), the same rule the collision walk applies. A source pinned to one branch (`repo://alias@branch/path`) fires only while the ledger's checked-out branch matches it; a branch name cannot contain a slash, and a detached HEAD or a missing repository keeps a pinned source silent. Guardrails, `--slice`, and the ripple advisory all honor the pin. `--preamble` prepends a cache-friendly architecture preamble for session boots.
- **For agents**: This is the difference between an agent that guesses and an agent that reads the rules for exactly the code it is about to touch.

#### Stop blocks and protected paths (`pylon check`)

- **What**: Hard stops for agents and humans. Package maps declare `protected:` globs; touching one is a `FORCED` finding. `--stop-block` renders the halt as a structured block an agent cannot misread.
- **The pain**: An autonomous agent editing machine-generated config, vendored manifests, or a security boundary without authorization is how quiet incidents start.
- **How it works**: `check <paths>` intersects your paths with every active plan's `contract.editing` and `contract.unstable` sets, evaluates `wrong_if` falsifiers, and matches `protected:` globs. `--plan <plan>` leaves out the `editing` and `unstable` sets of that one active plan.
- **For humans**: The same command answers the daily question "is anyone else in here right now?" before you write a line.

#### Discovery (`pylon map`, `pylon search`)

- **What**: One line per document in the ledger, filterable by type, package, tag, folder, status, and query, ranked by BM25 relevance fused with deterministic signals (status, recency, rule ids, queried project). Full-text search across the corpus beside it.
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

#### Session protocols (`pylon protocol`, `pylon reflect`)

- **What**: Protocol pages shipped with the binary and rendered on demand: the agent session loop and the closing retro.
- **The pain**: Every team re-writes its agent rituals in chat, and each session follows a different loop.
- **How it works**: `pylon protocol [list|<name>]` renders a shipped protocol (`agent` is the session loop); `pylon reflect` renders the hygiene record for session close. `--template <path>` renders a custom template.
- **For agents**: One stable loop, shipped with the ledger, rendered at session start and close.

### 3.4 Proof, not promises

Documentation you cannot verify is a rumor with formatting. This group makes every claim checkable, deterministically.

#### Zero model calls

- **The pain**: Verification that depends on a model gives a different verdict every run, costs per call, fails offline, and can be argued with.
- **How it works**: The engine is a Go binary. Every check is AST parsing, YAML schema validation, git inspection, or exact set intersection. Same input, same verdict, on your laptop and in CI. A fact about other documents is computed at check time, never stored: no frontmatter key records a `blocked` or `ready` state.
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
- **Guard**: `pylon lint` enforces sentence limits, voice, and vocabulary across the corpus.

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
- **How it works**: `symbol`, `route`, `cli`, `event`, `schema`, and their language-prefixed variants (`go-symbol`, `php-symbol`, `py-symbol`, `js-export`, `ts-export`) demand `verified_by:` or `falsified_by:` on accepted and in-progress plans (`PYLON040`). Resolution of the pointer stays `PYLON035`.
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
- **How it works**: `manifest` compiles validated JSON artifacts (`packages.json`, `rules.json`, `vocabulary.json`) from the source documents and the compiled tables. The vocabulary mirror states the closed frontmatter vocabulary — status sets, contract kinds with id forms, the per-type key registry, protected owners, and the closed value sets — and the pre-push gate fails when it drifts from the binary, so the text can never quietly lie about what the engine enforces. `manifest --check` proves the committed JSON matches the compiled catalogs, so CI catches drift.
- **Guard**: The pre-push gate runs the check on every push.

#### Machine parity (`lint --parity`)

- **The pain**: A machine documented in the ledger drifts from the enum and match arms in code, and no test names the drift.
- **How it works**: The parity pass compares declarations against marked code: domain `states:` against the enum at `states_symbol`, `machines:` transition triples against the marked code map, and route `exposes:` against source mentions. An unbound language, a transport failure, or a missing half stays advisory and never reads as parity.
- **Guard**: Declaration errors block without the flag (doc-only graph checks). Code mismatches block under it. External `pylon-ext-<name>` plugins serve languages the binary does not ship.

#### Review packets (`pylon review-brief`)

- **The pain**: A fresh reviewer (human or agent) must reassemble context from PRs, tickets, and plans, and reviewers with partial context approve partial work.
- **How it works**: For one plan, it assembles a self-contained packet: contracts with resolution state, per-repo diff stats, package map excerpts, and a coverage grid. Returned reviews pass 2 validation stages: `--validate` proves every row was evaluated, `--validate-resolutions` proves every finding was resolved before merge.
- **For agents**: Peer review becomes a reproducible gate a fresh-context agent can run with deterministic completeness checks.

#### Installable ledger

- **What**: A one-command installer that downloads a released binary and scaffolds a fresh ledger with its own git repository, wired hooks, and verified checksums.
- **The pain**: A map that requires a build chain to install gets skipped. Adoption dies in the setup instructions.
- **How it works**: `install.sh` detects the platform, downloads the release artifacts, verifies them against `checksums.txt`, and extracts into a ledger folder it names with you (default `pylons`). It runs `git init`, wires the gate hooks, and scaffolds an initial commit; it refuses a target that already holds a repository. A re-run updates an existing pylon in place without touching user files; `--bin-only .` drops just the binary. Ledger discovery walks 3 steps: an explicit `--pylons`, a walk up from the working directory, then the binary's own home. The binary inside its ledger answers from the parent project root. Gate hooks stay an installed-ledger feature; a nested dev ledger runs `pylon lint --pylons pylons` instead. Windows users download the binary directly.
- **For agents**: Nothing to build. The agent registers repositories in `pylons/manifests/pylons.yaml` and starts checking against real code.

#### Secret shapes (`PYLON060`, `PYLON061`)

- **The pain**: Credentials leak into docs through examples and pasted logs. Deleting the line does not remove it from git history.
- **How it works**: Every lint run scans the corpus for six closed provider shapes, private-key blocks included, as `FORCED` errors. Assignment shapes (JSON, ENV, dash-key, bearer) advise on the rest. Findings name the file and line, never the value. One `pylons-allow-secret` marker exempts exactly its own line, and these rules never enter the baseline.
- **Guard**: `pylon explain PYLON060`. Blocking exit 1.

#### The gate guard (`pylon review gate-surface`)

- **The pain**: A PR that widens what the gates ignore (exclude lists, hook files, closed vocabularies) silences every check downstream.
- **How it works**: One command diffs the merge against a hardcoded surface list: gate manifests, `.githooks/`, `templates/`, and added gate knobs in `pylons.yaml`. Each touched surface is a `FORCED` finding the baseline cannot absorb.
- **Guard**: `pylon review gate-surface --base <ref>`; `PYLON062` blocks, `PYLON063` advises on an unreadable base.

#### Pinned-release surface (`pylon review release-surface`)

- **The pain**: A deployment pinned to an old tag breaks when a consumed symbol or route changes, and the break lands at upgrade time.
- **How it works**: Lockfiles under `--pinned-releases <dir>` name each deployment's version. The command diffs HEAD against each pinned tag and blocks when a pinned deployment consumes a removed or changed surface.
- **Guard**: Unknown pin aliases and missing tags exit 1. `--base <ref>` overrides pinning for local checks.

#### CI annotations (`pylon review annotations`)

- **The pain**: Lint output inside a CI log is unreadable, so findings must land on the pull request itself.
- **How it works**: The command converts lint findings (SARIF or JSON) into GitHub Check annotations plus a summary body: blocking errors only, capped at 50, hostile text sanitized. The workflow posts them; the engine only emits.
- **Guard**: Never exits on findings; unparsable input exits 2. `--path-prefix` fixes annotation paths when the ledger lives in a repo subdirectory.

#### Strict frontmatter

- **The pain**: A typo in frontmatter used to fail silently and disable checks the author believed were active.
- **How it works**: Frontmatter decodes node-first. Anchors, aliases, merge keys, and custom tags fail closed with line numbers. Unknown keys are per-type advisories (`PYLON064`); the closed payloads (machines, transitions, contract entries, scenarios) reject unknown sub-keys hard.
- **Guard**: `PYLON001` names the reason, `PYLON064` names the key.

#### The readable baseline (`manifests/baseline.json`)

- **The pain**: Adoption means years of accepted debt. An opaque accept-list hides what was swallowed, and renumbered debt could swap silently.
- **How it works**: `pylon lint --update-baseline` records accepted `FORCED` findings as readable entries with counts. Matched findings stop blocking while anything new still fails. Stale entries are reported so the file only shrinks. Path numbers stay literal, so a renumbered target is new debt.
- **Guard**: Secret and gate-surface rules never enter it. A foreign baseline blocks with `PYLON001`.

### 3.5 The rest of the ledger

The cards above cover the systems with commands and grades. The rest of the ledger is reference material: zones without a card of their own above, still part of what developers and agents read. One line each, so this guide stays honest about the whole map.

| Zone | Holds | Who reads it |
|---|---|---|
| `templates/` | One schema per document type. Every new document starts here. | Anyone writing a doc |
| `proposals/` | Plans and conventions under review, before they graduate to their home zones. | Reviewers, plan authors |
| `references/` | Third-party API docs and external hooks the platform relies on. | Developers and agents extending integrations |
| `prompts/` | Shared raw agent prompts, reusable across the platform. | Agents and their operators |
| `skills/` | Reusable team skills for agent environments. | Agents |
| `protocols/` | Shipped agent protocols the CLI renders (`pylon protocol`, `pylon reflect`). | Agents and their operators |
| `extensions/` | External `pylon-ext-<name>` plugins; `_examples/` holds a runnable parity plugin. | Developers enabling code extraction |
| `pylons-docs/` | Reference pages that travel with the ledger: the adoption guide, the parity language spec, and the `pylons.yaml` schema. | Everyone |
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

## 5. Reference

Run commands through the `pylon` binary in the ledger root (`pylons/bin/pylon` from the project root, `./bin/pylon` from inside the ledger). Exit code 3 means no `manifests/pylons.yaml` was found; every command's and flag's contract is in the [`USAGE.md`](USAGE.md) dictionary.

### 5.1 Command and flag dictionary

The complete command and flag dictionary lives in [`USAGE.md`](USAGE.md): one dense row per command and flag, purpose and options inline. This guide keeps the feature cards above and the exit codes below; the dictionary is maintained in one place so a new command or flag edits a single table.
### 5.2 Exit codes

| Code | Meaning | Action required |
|---|---|---|
| `0` | Success or advisory-only findings. | Proceed with the workflow. |
| `1` | Blocking finding (`FORCED` collision, broken contract, lint error, ratchet regression, foreign baseline). | Fix the violation or coordinate with the conflicting plan owner. |
| `2` | Usage or runtime setup error (unknown command, invalid flag or window, unresolvable package filter). | Check syntax via `pylon --help`. |
| `3` | Environment failure (no `manifests/pylons.yaml` found). | Run from the ledger root or pass `--pylons <path>`. |

Lint-family commands (lint, check, review) report unreadable input as findings and exit 1; command-style commands (contract, review-brief, annotations, merge-danger) treat unreadable input as a setup error and exit 2.
