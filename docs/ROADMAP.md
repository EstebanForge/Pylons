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

## Explicitly Not Adopted

- **Hosted context runtime.** The agent harness owns sessions. No external portal dependency.
- **Model harness layer.** Model choice stays at the agent level, not inside Pylons.
- **Proprietary session store.** All captured context lands in git under the KB root. Reviewable, blameable, durable.
- **Auto-commit.** Every capture produces a candidate patch. A human or an approved agent skill commits it. The engine never writes to `main` directly.
- **Multi-agent dispatcher daemon.** No heavy background daemon or multi-process hook router. Local operation requires only lightweight detached fetch operations.
- **Rule-engine execution.** `pylon` does not run `ast-grep`, `conftest`, or other rule engines, and does not execute rule files from a repository. The ledger proves that a rule file exists. The CI of the code repository runs it.

Directions, not commitments. Scope and order shift with the platform.
