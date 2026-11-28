# Pylons Roadmap

**Rule for agents and maintainers:** this file lists unshipped work only. When an item here ships, delete it from this file in the same change and record it at the top of `docs/CHANGELOG.md` in the source repository. Do not mark items done in place; shipped work lives in the changelog, not here.

Features carry no order. Pull any feature forward when demand asks; list position implies no sequence.

Pylons operates as an automated governance and review engine; `pylon` is its CLI.

## Determinism Design Constraint

All features, rules, and commands in Pylons and the `pylon` CLI must be 100% deterministic. Enforcement relies strictly on AST parsing, static YAML schemas, git commit timestamps, and exact set intersections. No non-deterministic model calls or probabilistic heuristics run inside the engine. This guarantees identical, reproducible execution across local pre-commit hooks, developer CLIs, and CI automation.

## Future Directions

### enforcement-hardening

Agreed 2026-11-23 and deferred. Nothing here is scheduled. Every gate runs on one clone, and git lets the client skip each one: `--no-verify`, `PYLON_HOOKS_OPTIONAL=1` with no binary, an unset `core.hooksPath`, and plumbing commits. The items below close what a local gate can close and add the server check for the rest. Each item carries its design, its tests, and its known limits, so work can start without new research. Code paths are left out on purpose; read the engine first. Dependencies sit under **Needs** and are the only ordering.

#### server-boundary
- **Problem**: Hooks do not travel with a push. Only a check on the server covers every local bypass.
- **Design**: Ship a CI guide with three parts. An example workflow that downloads one pinned release binary with its checksum, clones in full, and runs `lint --ci`, `manifest --check`, `review gate-surface`, `review merge-danger`, and `review ratchet`. A ruleset JSON that the maintainer applies by hand: pull request required, status checks required in strict mode with one pinned source app, force pushes blocked, deletions restricted. A CODEOWNERS example that covers the gate surfaces.
- **Guard**: Add `.github/` to the surfaces that the gate-surface rule protects. Today a change can edit the workflow that judges it.
- **Rules**: CI never builds `pylon` from the tree under review; that lets a change rewrite the checker. The agent identity stays out of the ruleset bypass list. `--ci` fails on every finding, `ADVISORY` included; choose once between `--ci` and the ratchet for the pull request gate.
- **Tests**: The gate-surface rule fails first on a workflow file change.
- **False blocks**: A full clone costs time on large repositories. Accept it.
- **Not in scope**: A `pylon ruleset` command. It adds network calls and hosted state to an offline engine.
- **Known limits**: GitHub.com has no receive hook. Rulesets have no commit-message rule.
- **Needs**: Nothing. Build it first.

#### object-in-transit
- **Problem**: The commit gate takes its file list from the index but reads the bodies from the working tree. A partially staged document is checked in one form and committed in another. The package manifest is rebuilt from the documents on disk and then staged, so an uncommitted or an untracked document leaks into the commit. The push gate ignores the ref lines that git sends and lints the working tree, not the commits that ship.
- **Design, commit**: The gate reads three inputs from disk: staged Markdown bodies, `manifests/pylons.yaml`, and the `packages/` tree when the gate generates that manifest. Add one precondition before the manifest step: refuse while any of them differs from the index or is not in the index. Emit one `FORCED` finding that lists the files and the fix: stage them or stash them. Keep one shared check. Add no check at each read site.
- **Choice**: Reading blobs from the index was rejected. It needs four read paths that must stay in sync and an index reader for the whole `packages/` tree. It also hides the two slips that agents make: a newer edit left unstaged behind an older staged blob, and a new document that was never staged. The precondition catches both, and agents already stage all they touch.
- **Design, push**: Read the ref lines from standard input. Treat empty input as no refs. Skip delete pushes. Refuse when the pushed tree is not the tree of `HEAD`, or when the tracked tree has changes.
- **Tests**: Fail first on a partially staged document and on an untracked package document; pass after staging. Fail first on a push whose tree is not the linted tree.
- **False blocks**: A person who stages part of one document must stage all of it or stash the rest. A push of a branch that is not checked out is refused; push it from its own worktree. A push from a tree with work in progress is refused; commit it or stash it first.
- **Not in scope**: Exporting the pushed tree with `git archive`. Relative project paths break in the export.
- **Needs**: Nothing.

#### bypass-corpus
- **Problem**: No test drives a real commit or a push through the shipped hooks. Each hole is invisible until someone hits it.
- **Design**: One end-to-end test. Build the binary into a temp dir, init a repository and a bare remote, run `pylon install`, then drive real `git commit` and `git push` commands through the shims. A table maps each step to its result: normal commit, `--no-verify`, plumbing commit (`commit-tree` then `update-ref`), partially staged document, untracked package document, pushed tree that is not the linted tree, a commit that sets `no_verify: allow`, `commit -a`, `commit <path>`, rebase, cherry-pick, revert, merge, `git am`, amend.
- **Rule**: Assert the open paths as open. When a later item closes one, its row flips; that flip is the proof.
- **Not in scope**: A per-rule skip variable such as `SKIP=<rule>`. It is a silent bypass that the committer controls. The committed baseline stays the only opt-out.
- **Needs**: Nothing. Build it with the first item that must flip a row.

#### no-verify-guard
- **Problem**: `git commit --no-verify` skips the commit gate and leaves no trace. `prepare-commit-msg` is the one commit hook that `--no-verify` does not skip, and its non-zero exit aborts the commit.
- **Design, key**: `no_verify: deny|allow` in `manifests/pylons.yaml`. Absent means deny. Read the key from `HEAD` and from the index; the stricter value wins, so one commit cannot loosen the key and use it in the same step. No environment variable turns the guard off. `PYLON_HOOKS_OPTIONAL` is ignored in deny mode.
- **Design, stamp**: The last step of the commit gate writes the index tree (`git write-tree`) and the engine version into the git dir.
- **Design, stage**: Add `gate prepare-commit-msg <file> <source> [<sha>]`. The `gate` command and the shims must pass the hook arguments through; today the stage name is the only argument. In deny mode, abort when the stamp is missing or its tree differs from the current `git write-tree`. The message names the fix: commit again without `--no-verify`.
- **Exemptions**: Merge and squash sources. The state files `MERGE_HEAD`, `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `rebase-merge/`, and `rebase-apply/`. Rebase and cherry-pick replays are expected to run no commit gate and to call this hook; prove both with the corpus first.
- **Install**: Add the shim to the embedded hook assets and to the hook name list. `self-update` does not touch `.githooks`, so every clone runs `pylon install` once more.
- **Tests**: Through the corpus. Deny plus `--no-verify` aborts. Allow plus `--no-verify` commits with the skipped marker. A normal commit passes. `commit -a` and `commit <path>` pass. Rebase, cherry-pick, revert, and merge are not blocked.
- **Known limits**: `git push --no-verify`, `-c core.hooksPath=`, plumbing commits, and a deleted shim stay open; the server boundary covers them. `git am` and automatic merge commits run no Pylons hook today. Git calls `applypatch-msg`, `pre-applypatch`, and `pre-merge-commit` for them, and the ledger ships none. This stage sees merges and exempts them. It never sees `git am`. A `reference-transaction` hook was rejected: it fires on every ref update, fetch included.
- **Needs**: The bypass corpus.

#### bypass-marker
- **Problem**: In allow mode a skipped gate leaves no trace in the history.
- **Design**: The `prepare-commit-msg` stage writes `Pylon-Gate: skipped` when allow mode is on and no stamp matches. Write it with `git interpret-trailers --in-place --if-exists replace`, so an amend does not duplicate it.
- **Not in scope**: A `pass` receipt with a rule-set hash and an engine version. CI re-runs every rule and needs no claim from the author. The hash also changes with each baseline payment.
- **Known limits**: A cherry-pick copies the marker onto a new commit. A squash merge can drop trailers.
- **Needs**: The `prepare-commit-msg` stage from no-verify-guard, and the server boundary.

#### history-aware-rules
- **Problem**: Two ledger rules exist as prose only: an accepted decision is immutable, and a quirk is append-only. State-based lint cannot see an in-place edit or an illegal status change.
- **Design**: A history pass over staged documents that compares the `HEAD` blob with the index blob, on the pattern of the gate-surface rule. Three `FORCED` rules with the next free IDs: a decision edited in place (only `status`, `updated`, and the archive location may change), a quirk body that is not append-only, and a status change outside a declared edge.
- **Vocabulary**: Add a `transitions` key to `manifests/vocabulary.json` beside each status set. A test proves that every edge names statuses from that set.
- **Baseline**: The new rules never enter the baseline. An in-place edit is an event, not standing debt.
- **Where it runs**: Commit gate on the index. CI on `base...HEAD`.
- **Edge cases**: A staged delete of an accepted decision is a violation; deletes sit outside the staged filter today. A rename with a heavy edit shows as a delete plus an add and must fail closed. An `updated:` bump is allowed with a status change. `lint --fix` renames without staging, so the pass reads the index only.
- **Accepted cost**: A typo in an accepted decision needs a superseding decision. That is the documented rule.
- **Needs**: The bypass corpus.

#### code-repo-gate
- **Problem**: A mapped code repository has no gate of its own. `pylon check` is advice that a session can skip, and the pylon lives in another repository.
- **Design, CI**: Run `pylon check` over the changed paths of a pull request in the code repository, with the ledger beside it. This path cannot be skipped.
- **Design, local**: `pylon install --project <alias>` writes a shim into that repository, sets its local `core.hooksPath`, and reuses the hooks-path conflict error and the chain line. A `gate code-pre-commit --project <alias>` stage turns staged paths into `repo://` targets and runs `check`.
- **Pointer**: `git config --local pylons.root <path>` in the code repository. It is not committed. No pointer means warn and pass. A pointer without a binary fails closed.
- **Not in scope**: A global `core.hooksPath` or `init.templateDir` dispatcher. It takes over hooks that belong to other tools.
- **Needs**: The server boundary gives the CI path its workflow.

#### enforced-by-pointer
- **Problem**: A convention can carry `status: enforced`, but no rule asks for proof that something enforces it.
- **Design**: A frontmatter key `enforced_by:` on conventions. It holds a list of `repo://` URIs. One `FORCED` rule fires when `status: enforced` has no pointer, or when a pointer does not resolve. Reuse the `file` contract resolver. Register the key in the frontmatter keys of the vocabulary and in the convention template, or the unknown-key rule fires.
- **Tests**: Fail first on a missing key and on a dead pointer. Pass on a pointer that resolves.
- **Not in scope**: Running the rule files. They run in the CI of the code repository. The ledger proves that the file exists.
- **Needs**: Nothing.

### guardrails-proof-and-recovery

Agreed 2026-11-26 and deferred. Nothing here is scheduled. These items extend static guardrails, contract proof strictness, audit observability, and rollback recovery inside Pylons' deterministic offline boundary (complements `enforcement-hardening` above, which already covers `server-boundary`, `no-verify-guard`, `bypass-marker`, `history-aware-rules`, `enforced-by-pointer`, and `code-repo-gate`). Code paths are left out on purpose; read the engine first.

#### quirk-guard-pointer
- **Problem**: `PYLON028` checks that `guard:` on an active quirk over 30 days old is one of `lint-rule`, `convention`, `test`, or `accepted-risk`. A quirk can claim `guard: test` or `guard: lint-rule` without pointing to a real test or rule, so the guard rots into an unverified claim.
- **Design**: Extend `PYLON028` (or add a companion `FORCED` rule) so `guard: test` and `guard: lint-rule` require a non-empty `guardrail:` (or `verified_by:`) pointer that resolves against tracked project sources via the `file` or `symbol` contract resolver. Require `guard: convention` to resolve a `pylon://conventions/` target. Keep `guard: accepted-risk` pointer-free, with an optional `links:` entry to an accepted decision. Update `manifests/rules.json` and `manifests/vocabulary.json` if new keys or rules are added.
- **Tests**: Fail first on an active quirk over 30 days old with `guard: test` and an empty or dead pointer, and on `guard: convention` without a resolving convention link. Pass when the pointer resolves or `guard: accepted-risk` is set.
- **Not in scope**: Running the referenced test or lint rule from `pylon`. The code repository CI runs it.
- **Needs**: Nothing.

#### strict-contract-proof
- **Problem**: `PYLON040` (`contract_unproven`), `PYLON045` (`slice_incomplete`), and `PYLON046` (`contract_no_scenario`) are advisory-only. Teams cannot block merges when `machines:` transitions or `contract.stable` surfaces ship without `verified_by:` or `falsified_by:` pointers, or when a `slice: true` plan lacks scenario coverage.
- **Design**: Add `strict_proofs: true|false` (default `false`) in `manifests/pylons.yaml` and a `--strict-proofs` flag on `pylon lint` and `pylon contract`. When enabled, promote `PYLON040` on `machines:` and `contract.stable` entries, and `PYLON045`/`PYLON046` on accepted or in-progress `slice: true` plans, from `ADVISORY` to `FORCED` (exit 1). Keep default `false` so existing ledgers do not break on upgrade.
- **Tests**: Fail first with exit 1 when `strict_proofs` is enabled and a `contract.stable` or `machines:` entry lacks `verified_by:`/`falsified_by:`, or when a `slice: true` plan misses a behavior/test contract or covering scenario. Pass with exit 0 when disabled or when all pointers resolve via `PYLON035`.
- **Not in scope**: Executing mapped repository test suites or parsing test assertions.
- **Needs**: Nothing.

#### ledger-trend-export
- **Problem**: `pylon review ratchet` checks current advisory counts against a snapshot, and `pylon review utility` reads local `.git/` injection telemetry, but teams have no deterministic way to inspect rule hit rates, ratchet burn-down, or rework trends across commits or CI runs.
- **Design**: Add `pylon review trends [--window <period>] [--package <pkg>] [--format human|json]`. Walk git commits in the requested window by commit timestamp, compute per-rule finding counts, ratchet deltas, and `rework` ratios, and include `utility` citation/injection counts when present. Emit deterministic `human` and `json` output with no daemon or external store.
- **Tests**: Table tests over a fixture git repo with multiple commits showing advisory burn-down, rising rule counts, and byte-stable JSON output across repeated runs.
- **Not in scope**: Runtime application logs, traces, metrics, live alerting, or hosted dashboards.
- **Needs**: Nothing.

#### self-update-rollback
- **Problem**: `pylon self-update` backs up replaced and removed product files under `_pylons-backup`, but has no restore command if a release breaks a workflow or introduces a regression.
- **Design**: Write a backup manifest inside `_pylons-backup` (previous version, binary path, and backed-up relative file paths and SHA-256 hashes) during `self-update`. Add `pylon self-update --rollback [--dry-run]` to verify the backup manifest, restore backed-up product files, delete files newly added by the failed update, and atomically restore the previous `pylon` binary. Refuse fail-closed (exit 2) when `_pylons-backup` or its manifest is missing or corrupt.
- **Tests**: Fail first when no backup exists. Pass on round-trip `self-update` followed by `self-update --rollback`, proving restored files, dropped-file recovery, and binary hash match pre-update state, and `--dry-run` leaves disk untouched.
- **Not in scope**: Production deployment, database migration, or feature-flag rollback.
- **Needs**: Nothing.

#### merge-danger-rollback-note
- **Problem**: `PYLON047` (`pylon review merge-danger`) requires a `one-way` door to carry a breaking release marker or a `Data migration:` note, but does not require a recovery or rollback plan when a change is hard to reverse.
- **Design**: Extend `pylon review merge-danger` so a `one-way` door also requires a non-empty `Rollback:` (or `Recovery:`) line in the PR body naming the revert procedure, forward-fix strategy, or `pylon://` runbook link. Gate behind a `manifests/pylons.yaml` flag or `--require-rollback-note` if backward compatibility with existing PR templates is needed, and update `USAGE.md` and `conv.merge-danger-line`.
- **Tests**: Fail first on a `one-way` PR body with `Data migration:` but no `Rollback:` line when enabled; pass when `Rollback:` is non-empty; leave `two-way` PRs unaffected.
- **Not in scope**: Automated execution of rollback scripts or runbooks.
- **Needs**: Nothing.

### runtime-enforcement-and-recall

Agreed 2026-11-27 and deferred. Nothing here is scheduled. These items extend the ledger past the git gate into the moment a session acts or asks: command-time enforcement for quirk rules, staleness judged by references instead of dates, a receipt of what the ledger served, and a sweep that flags the live records left holding a dead premise. They complement `enforcement-hardening` and `guardrails-proof-and-recovery` above. Code paths are left out on purpose; read the engine first.

#### enforced-quirks
- **Problem**: A quirk can claim a guard, and `pylon context` injects quirks as advice, but nothing in the ledger stops a command at the moment it runs. "Never force-push" informs. It does not enforce. The git gates cover commits and pushes only.
- **Design, rules**: Quirk frontmatter gains an `enforce:` list. Each entry is a map with `action: ask|deny`, `surface: command|read|write`, and a glob `pattern`. Examples: deny the command `git push --force*`, ask before `write` on `migrations/**`, deny `read` on `.env*`. The key must be registered for the quirk type in the frontmatter registry and mirrored in the payload template, or the unknown-key rule fires. One new rule (next free ID) validates the grammar: known action, known surface, non-empty pattern, globs only (`*`, `**`, no shell metacharacters, no `;` inside the pattern). Parse failures fail closed as frontmatter findings.
- **Design, matcher**: A pure deterministic matcher: pattern list plus one command string or one path, decision out (`allow|ask|deny`) plus the quirk and rule that decided. Command parsing: split chains on `&&`, `||`, `;`, and `|` outside quotes; recurse one level into `bash -c`, `sh -c`, and `eval` single-string arguments; strip variable-assignment prefixes and the wrappers `sudo`, `env`, `nohup`, `timeout`, `nice`, `xargs`; match the binary by basename so `/usr/bin/git` is `git`. A pattern matches the full command string or the binary token. Quoted arguments are argument text, never re-split as commands. Path matching reuses the existing glob helper against the target path.
- **Design, stage**: `gate enforce` reads the active quirks of the ledger and decides one call: `--command <raw>` or `--path <p> --mode read|write`. Exit 0 allow, 4 ask, 1 deny. It prints one JSON line with the decision, the quirk, the rule, and the matched pattern, so a host adapter can render the reason. `ask` waits for the human through the host harness; the engine never prompts.
- **Design, adapters**: Ship thin per-harness hook scripts that call the stage. No daemon, no server, no resident process. A harness without hooks gets no enforcement; the git gates still run.
- **Protection**: A quirk carrying `enforce:` is append-only and its `enforce:` list cannot be loosened in place; the same protection class as the accepted-decision immutability work (history-aware-rules). The gate-surface rule treats such quirks as protected ledger surface. `guard:` accepts a new value `enforce` exactly when the quirk carries a non-empty `enforce:` list; `quirk-guard-pointer` then asks no external pointer for it.
- **Baseline**: The grammar rule never enters the baseline. Enforcement decisions are events, not standing debt. Add the rule ID to the baseline-exempt set beside the secret and gate-surface rules.
- **Tests**: Fail first on an `enforce:` entry with an unknown action and on a pattern containing `;`. Matcher table: the bare forbidden command; the same command under `sudo`, `env`, `timeout`, and `bash -c`; the command as a chained tail on `&&`, `||`, and `|`; a path-prefixed binary; a quoted commit message that contains the pattern text (must not fire); an ordinary command the pattern does not name (must pass). One end-to-end test drives the disguised-call table through an adapter script.
- **False blocks**: Globs are the quirk author's duty; `git push*` denies every push. Quirks are reviewable documents, and every deny names the quirk that decided, so a wrong pattern is one edit and one review away.
- **Known limits**: The matcher sees the command string only. Variables expanded at run time, a script that runs the forbidden thing inside, shell functions, and aliases are invisible. It does not claim them. The `read` and `write` surfaces depend on the host harness passing the target path; the command surface is the dependable one.
- **Not in scope**: A resident daemon, an MCP enforcement server, intercepting harness internals, executing anything.
- **Needs**: Nothing. The matcher and the stage stand alone; adapters are thin and can land one harness at a time.

#### reference-staleness
- **Problem**: Staleness today is a calendar: the staleness check flags an old `updated:` date for human review. A document can carry yesterday's date and still be dead: it names a just or make target that was renamed, a package script that was removed, an environment variable the code stopped reading, a version the manifests no longer allow, or a source file whose symbol moved. Recency is not correctness, and search and map serve dead documents with no verdict.
- **Design**: A deterministic reference-check pass. For each parsed document, collect the things it names, resolve each against the registered project checkouts, and classify: `verified` (exists), `dead` (history shows it existed and it is gone), `unknown` (no proof either way; likely a typo). The first batch resolves `repo://` paths with the existing resolution and `path::Symbol` targets through the existing language resolvers. Just and make targets, package scripts, environment names, and version constraints go through the extension protocol as `pylon-ext-<name>` executables, so the core stays dependency-light.
- **Design, precision**: The `dead` grade requires proof from git history that the named thing existed. Without that proof the grade is `unknown`. A rename lands as `dead` only when the old name is gone from the current tree.
- **Design, surfaces**: Findings are `ADVISORY`: one per document with the dead count and the names, beside the existing staleness finding. Corpus entries gain verdict counts, so `map --json` and `search` can show `verified|dead|unknown` per result. The pass runs on changed documents during lint; `pylon review staleness [--all]` runs the full corpus with symbol and extension resolution.
- **Tests**: Fail first on a document naming a just target that a fixture commit deleted (`dead`), on a symbol that moved (`dead`), and on a name with no history (`unknown`). Pass when the target exists. Verdict fields in `map --all --json` are byte-stable across two runs.
- **False blocks**: Shallow clones lose the history proof; the grade falls back to `unknown`, never `dead`. Generated and vendored trees are skipped like everywhere else.
- **Known limits**: The pass sees registered checkouts only. A document that is wrong while all its references live stays invisible to this pass; the human staleness review keeps that job.
- **Not in scope**: Executing targets or scripts, semantic truth of prose, a background resolver daemon.
- **Needs**: Nothing for paths and symbols. The extension protocol for targets, scripts, environment names, and versions.

#### session-receipt
- **Problem**: The ledger serves context all day (brief, map, search, context injections, gate decisions) and keeps no record of what it served. After the fact, "the ledger had it and the session ignored it" is unprovable, and a serve that arrived cut or never arrived is invisible. The product scope covers audit trail; serving is the one audit surface with no trail.
- **Design**: An opt-in, append-only receipt at `_pylon/receipt.jsonl` (underscore directories are never linted or indexed). `pylon install` adds the directory to the ledger `.gitignore`. Every serve surface appends one JSON line: timestamp, command, argv, and its verdict fields: brief (sections served, lines, budget cuts), search (query, paths served), context (targets, quirks injected), `gate enforce` (allow, ask, deny with the quirk that decided), doctor (run summary). Content is deterministic; timestamps are the only wall-clock values and they never enter a rule.
- **Rules**: Writes fail open: a read-only filesystem or a corrupt receipt never blocks a serve. Rotation is deterministic: past a line cap (default 1000) the oldest lines drop. The receipt is write-only evidence: no rule, gate, or search reads it, so it cannot touch engine determinism. Secret scrubbing runs over each line with the same scanner the findings use, replacing hits before the write.
- **Design, query**: `pylon receipt [--last N] [--command <name>]` prints filtered lines as JSONL for unix tools and `jq`.
- **Tests**: Fail first on a serve that drops a budget cut from the receipt, and on a line whose argv carries a secret that was not scrubbed. Pass on read-only failure (the serve exits 0 and writes no line). Rotation drops the oldest lines at the cap. Line shape is byte-stable.
- **False blocks**: None. Opt-in, fail-open, local.
- **Known limits**: The receipt records what the ledger served, not what the agent did with it. Tool calls that never touch `pylon` leave no line.
- **Not in scope**: Transcript capture, telemetry export, a viewer or server, agent-behavior scoring.
- **Needs**: Nothing.

#### invalidation-sweep
- **Problem**: `PYLON066` flags the doc that names its successor, and the archive holds the replaced record, but the docs that leaned on the dead premise stay silent. A live package still declaring `depends_on:` on a superseded package, or a live doc whose `links:` still resolve to a superseded decision, keeps serving with no review signal. The supersedes keys record the replacement chain; nothing walks the dependency direction.
- **Design, dead set**: A target is dead when any of these hold: a non-empty `superseded_by:`, `status: superseded`, or a path under `/archive/` (the same detection the contract-claim pass uses). A subject is live when none of them hold for the subject itself.
- **Design, edges**: Explicit edges only, one hop, no transitive closure: a `links:` entry whose URI resolves to a dead target, and a `depends_on:` entry naming a dead package by the identity `pylon context --graph` already uses. Prose mentions are not edges. Contract consumption is not an edge until a machine key for consumers exists.
- **Design, rules**: Two `ADVISORY` rules with the next free IDs. The sweep rule emits one finding per live subject naming each dead target it still holds, beside the target's own `PYLON066` finding. The consistency rule checks the supersession pair: B names A in `superseded_by:` while A's `supersedes:` list does not name B back, or the reverse, and one finding names the missing side. No new status vocabulary: the finding carries the review duty, and the closed status sets stay closed.
- **Choice**: Transitive closure was rejected. One dead premise would flood every transitively linked doc, and the flood hides the doc the author must actually fix. One hop keeps the signal; the author follows the chain by hand, one review at a time.
- **Where it runs**: A corpus pass beside the contract-claim pass in the lint order, skipped under `ChangedOnly` like the graph pass, honoring `exclude:` globs and the root skip list. `pylon review` reports both rules like every advisory.
- **Baseline**: Ordinary advisories: baselinable, so a team can accept a known dependency on a dead record, and the ratchet tracks them like the rest.
- **Tests**: Fail first on a live package with `depends_on:` on a superseded package, a live doc with a `links:` entry resolving to an archived decision, and a pair missing its back link. Stay silent on: both docs archived together, a consistent pair, a two-hop path (A links B, B links dead C, A stays silent), and docs with no links or `depends_on`.
- **False blocks**: Archiving a dead record and its dependents in separate commits fires advisories between the commits. The fix is order, not code: archive the dependents in the same change, or baseline the findings while the move spans commits.
- **Known limits**: A doc that names a dead decision in prose with no `links:` entry is invisible. If a consumption key for contracts lands later, the sweep extends to it.
- **Not in scope**: Transitive closure, auto-archiving dependents, a `needs_review` status, link rewriting.
- **Needs**: Nothing. The supersedes keys parse today and the corpus pass shape exists.

### adoption-and-interop

Agreed 2026-11-27 and deferred. Nothing here is scheduled. These items let a ledger adopt records written for the file-based knowledge formats that formed around the same artifact class, starting with the Open Knowledge Format (OKF, v0.2). The ledger adopts the records and judges them under its own rules; it never adopts the format's permissiveness (unknown keys tolerated, broken links blessed, trust advisory). One line of positioning: the bundle format makes a corpus self-describing; Pylons makes it self-enforcing. Code paths are left out on purpose; read the engine first.

#### okf-import
- **Problem**: `pylon import` scaffolds bare markdown and skips files that already carry valid frontmatter, so OKF-shaped material lands either unwrapped or untouched, and nothing maps its provenance and lifecycle keys onto the ledger's. The two formats share an artifact class and oppose a philosophy: OKF standardizes exchange and judges nothing; Pylons judges truth. Adoption must take the records and refuse the permissiveness.
- **Design**: An import profile selected with `--profile okf`. One static map, versioned against OKF 0.2: `type:` maps through a fixed table to a ledger type, unknown values to `reference` with the original value recorded in the body header; `description:` and `tags:` map through unchanged; `sources:` entries map to ledger `sources:` entries keeping `resource` verbatim (URLs stay URLs; `repo://` stays reserved for real registrations); `status:` maps `draft` to `proposed`, and `stable` and `deprecated` fall through to the target type's default status with the OKF status recorded in the body. Keys with no ledger counterpart (`generated`, `verified`, `stale_after`, the attested-computation fields) are never invented into frontmatter: the importer records them as one fenced block at the top of the body, so nothing drops silently. `--type` and `--name` keep their current meaning and override the map.
- **Rules**: The importer never validates a bundle for OKF conformance; that is OKF tooling's job. Imported docs become ordinary ledger docs: every rule judges them from the first lint, and proof keys stay empty until a human or the strict-proof work fills them. Dry-run prints the per-file mapping before any `--write`.
- **Tests**: Fail first on a file whose OKF `sources:` entries lose their `resource` at import, and on an unknown `type:` mapping to anything but `reference`. Pass on: a `draft` plan landing in `plans/proposals/` as `proposed`; a deprecated doc landing at the type's default status with the OKF status in the body block; dry-run exiting 0 with the mapping table and writing nothing; a body byte-identical apart from the recorded block.
- **False blocks**: None at import. Bundle-relative links (`/tables/x.md`) survive into the body, and the normal link checks flag them after import; fixing targets is the designed post-import step, not a defect.
- **Known limits**: OKF per-claim footnote attribution and Attested Computation contracts have no ledger counterpart; they stay as prose. `verified:` trust events are recorded, never trusted: a human-reviewed OKF doc is unverified until the ledger's own proof keys say so.
- **Not in scope**: OKF export, conformance checking, trust-tier computation, a permissive lint mode for OKF keys.
- **Needs**: Nothing. `pylon import` exists with dry-run default, `--write`, `--type`, and `--name`.

## Explicitly Not Adopted

- **Hosted context runtime.** The agent harness owns sessions. No external portal dependency.
- **Model harness layer.** Model choice stays at the agent level, not inside Pylons.
- **Proprietary session store.** All captured context lands in git under the KB root. Reviewable, blameable, durable.
- **Auto-commit.** Every capture produces a candidate patch. A human or an approved agent skill commits it. The engine never writes to `main` directly.
- **Multi-agent dispatcher daemon.** No heavy background daemon or multi-process hook router. Local operation requires only lightweight detached fetch operations.
- **Rule-engine execution.** `pylon` does not run `ast-grep`, `conftest`, or other rule engines, and does not execute rule files from a repository. The ledger proves that a rule file exists. The CI of the code repository runs it.

Directions, not commitments. Scope and order shift with the platform.
