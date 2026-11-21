# Pylons Roadmap

**Rule for agents and maintainers:** this file lists unshipped work only. When an item here ships, delete it from this file in the same change and record it at the top of `pylon://meta/CHANGELOG.md`. Do not mark items done in place; shipped work lives in the changelog, not here.

Features carry no order. Pull any feature forward when demand asks; list position implies no sequence.

Pylons operates as an automated governance and review engine; `pylon` is its CLI.

## Determinism Design Constraint

All features, rules, and commands in Pylons and the `pylon` CLI must be 100% deterministic. Enforcement relies strictly on AST parsing, static YAML schemas, git commit timestamps, and exact set intersections. No non-deterministic model calls or probabilistic heuristics run inside the engine. This guarantees identical, reproducible execution across local pre-commit hooks, developer CLIs, and CI automation.

## Future Directions

### transition-parity (Priority 1)
- **Change**: Extend `lint --parity` from state sets to transition graphs. A lifecycle is a formal finite state machine (S x E -> S) represented as a domain construct with a parity-checked doc table; its code surface is a `symbol` contract. Gate: lands on a pilot package first, to eliminate latent generative drift.
- **Convention Stub**: At ship time, add a state-transition-contracts.md convention entry. Rules: one closed state set per lifecycle, in a backed enum in the target language; one backed `Event` enum beside it; one pure `(State, Event): State` transition map. Side effects live outside transitions; lint cannot prove purity, so this rule is convention prose plus a table test in the mapped repo's QA suite. An invalid transition throws; enforced by the table test, not lint. No parallel boolean flags (`isLoading` plus `isSuccess`); a review-lens rule, not an engine rule. Persisted transitions use a compare-and-swap on the stored state, because background workers may run concurrently. Scope: batch processors, status pipelines, and renewal flows in the mapped repos. String-status systems stay out of scope until wrapped in a backed enum.
- **Extractable Shape**: One canonical form. A class-level `const TRANSITIONS` nested map, keyed by `State` case value then `Event` case value, literal enum case values only, declared on the same class as the machine. Declaration is explicit: a `#[StateMachine]` attribute (or language equivalent) naming the enums and the persisted state key, or a `machine:` front-matter key in the domain doc pointing at the qualified name. No enum auto-discovery; discovery by proximity is a heuristic and violates the Determinism Constraint. Everything else is out of extractor scope: computed keys, spreads, `static::` and trait-provided constants, `match` default arms, non-literal `to` values, runtime-built maps.
- **Declared Format**: The engine generates the doc table from code between `<!-- pylon:transitions:begin -->` markers, as a candidate patch per the manifest-regen model. `initial:` and `terminal:` state sets are mandatory declared fields. A Mermaid `stateDiagram-v2` render beside the table is free output of the same triples.
- **Deterministic Mechanism**: AST extraction resolves the enum case sets and the transition triples `(from, event, to)` from the const map. Block verification checks the generated doc section against the same triples. Exact set intersection in both directions; this moves the parity machinery from set pairs to triple sets.
- **Enforcement** (lint severities, registered in `repo://pylons/cli/internal/rules/catalog.go`; never `FORCED`/`ADVISORY` check grades, which stay plan-signal-only):
  - `TRANSITION_UNPARSEABLE` (error): a declared machine whose map cannot be fully resolved. Fail closed; a skipped machine must never read as parity.
  - `TRANSITION_MISMATCH` (error): a doc triple with no code triple, or the reverse.
  - `UNKNOWN_STATE_IN_TRIPLE` (error): a triple names a state absent from the enum.
  - `UNREACHABLE_STATE` (error): a case with no inbound transition and not declared initial.
  - `DEAD_STATE` (error): a case with no outbound transition and not declared terminal. Batch machines must reach a terminal.
  - `DUPLICATE_TRANSITION` (error): two triples share `(from, event)` with different `to`. Map literals silently overwrite duplicates; the AST still sees both.
  - `BYPASSED_STATE_WRITE` (advisory, ratchet-fed): the persisted state key declared in the marker is written by a direct persistence call outside the machine class.
- **Plan Binding**: Plans touching a machine list the transition map and the state enum as `symbol` contracts. `contract.editing` and `contract.unstable` then apply with no new engine code. A removed or changed triple, or a removed enum case, reads as `CHANGED-UNDECLARED` unless a plan covers it, and escalates the merge door to `one-way`: persisted data can sit in a now-illegal state. An added triple stays two-way.
- **Consumer**: `pylon lint` blocks on error findings; the advisory feeds `advisory-ratchet`. `pylon context` prints the graph beside contract entries, so agents see the allowed transitions before writing flow code. `brief` reports the machine count and the count missing `verified_by:`. The machine declaration carries `verified_by:` naming one table test in the mapped repo's QA suite; lint resolves it by set membership. The test derives its legal triples from the same const as the full cross product of state cases and events, and asserts the complement throws. No hand-copied table, no third source to drift.
- **Objective**: When the first machines ship, mapped lifecycles become checkable graphs instead of prose and parallel booleans. Lint proves shape parity between doc and code; the convention and the table test carry purity, throw-on-invalid, and concurrency.

### knowledge-utility
- **Command**: `pylon review utility [--prune] [--promote]`
- **Deterministic Mechanism**: Track knowledge usage across agent sessions using structured citation comments (`<!-- pylon:referenced-doc: <slug> -->`). Compute a deterministic utility score for each document: citation count divided by injection count, combined with recency decay.
- **Automated Lifecycle**:
  - *Quirk Pruning*: Demote or archive quirks that appear in context repeatedly without citations, preventing context bloat from obsolete bugs.
  - *Pattern Promotion*: Detect quirks or conventions cited frequently across multiple packages. Propose promotion to formal platform conventions in `conventions/` or automated rules in `pylon lint`.
- **Consumer**: Filter input for `pylon context` ranking and periodic hygiene reviews.
- **Objective**: Keep context lean and high-signal. Weed out obsolete quirks and elevate proven operational patterns to formal engineering standards.

### parity-linter
- **Command**: `pylon lint --parity`
- **Deterministic Mechanism**: AST extraction via the enabled language extensions and markdown table parsing. Extract enum definitions, state machines, and route registrations from the mapped source, against the declared tables in `domain/` and `packages/`.
- **Enforcement**:
  - Detect `COUNT_MISMATCH`: Markdown lists 4 states, but the code enum contains 5.
  - Detect `STATE_MISMATCH`: Markdown names state values that do not match the enum backing values.
  - Detect `API_ROUTE_MISMATCH`: Route parameters or HTTP methods in documentation disagree with registered route callbacks.
- **Objective**: Catch semantic drift between high-level architectural documentation and actual codebase implementations without manual line-by-line inspection.

### pr-harvest
- **Command**: `pylon harvest pr --diff <patch> [--metadata <json>]`
- **Deterministic Mechanism**: Run in CI upon PR merge across the mapped repositories. Parse PR commit messages, merge diffs, and review conversation markers for architectural changes and public symbol additions.
- **Automated Write-Back**: Auto-generate candidate updates: new entries for `packages/<pkg>.md` `exposes:` lists, or scaffolded post-mortems in `quirks/proposals/` when PR labels or commit messages indicate a footgun fix.
- **Consumer**: The workflow publishes the candidates as a single reviewable patch artifact. A maintainer applies the patch and commits to `main` by hand. The engine never opens branches or pull requests and never writes to `main` (see Explicitly Not Adopted). A harvest failure must fail the check, never skip silently.
- **Objective**: Capture architectural updates and footgun learnings at merge time, ensuring documentation moves in lockstep with shipped code.

### review-retro
- **Command**: `pylon review retro [--window <period>]`
- **Deterministic Mechanism**: Extend the PR harvest (`pr-harvest`). Normalize every human review finding to a signature: lens id, rule or contract id, path pattern. Count signature repeats across merged PRs and returned `review-brief` validations within the window. A signature seen twice or more opens exactly one promotion candidate as a reviewable patch.
- **Promotion Paths (closed set)**:
  - `wrong_if` falsifier entry on the touched plan contract, enforced by `check`.
  - New `pylon` lint rule proposal against the rule catalog.
  - Convention doc stub in `conventions/proposals/`.
- **Consumer**: A maintainer accepts the candidate by commit. The loop ends in executable enforcement or accepted prose, never in a repeated comment. No auto-commit: candidate patch only, per the Explicitly Not Adopted list.
- **Objective**: Never write the same comment twice. A recurring review finding becomes a check, a falsifier, or a convention. Extends the `knowledge-utility` pattern-promotion idea: input here is review findings, not document citations.

## Explicitly Not Adopted

- **Hosted context runtime.** The agent harness owns sessions. No external portal dependency.
- **Model harness layer.** Model choice stays at the agent level, not inside Pylons.
- **Proprietary session store.** All captured context lands in git under the KB root. Reviewable, blameable, durable.
- **Auto-commit.** Every capture produces a candidate patch. A human or an approved agent skill commits it. The engine never writes to `main` directly.
- **Multi-agent dispatcher daemon.** No heavy background daemon or multi-process hook router. Local operation requires only lightweight detached fetch operations.

Directions, not commitments. Scope and order shift with the platform.
