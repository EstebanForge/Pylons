# Pylons Roadmap

**Rule for agents and maintainers:** this file lists unshipped work only. When an item here ships, delete it from this file in the same change and record it at the top of `pylon://meta/CHANGELOG.md`. Do not mark items done in place; shipped work lives in the changelog, not here.

Features carry no order. Pull any feature forward when demand asks; list position implies no sequence.

Pylons operates as an automated governance and review engine; `pylon` is its CLI.

## Determinism Design Constraint

All features, rules, and commands in Pylons and the `pylon` CLI must be 100% deterministic. Enforcement relies strictly on AST parsing, static YAML schemas, git commit timestamps, and exact set intersections. No non-deterministic model calls or probabilistic heuristics run inside the engine. This guarantees identical, reproducible execution across local pre-commit hooks, developer CLIs, and CI automation.

## Future Directions

### knowledge-utility
- **Command**: `pylon review utility [--prune] [--promote]`
- **Deterministic Mechanism**: Track knowledge usage across agent sessions using structured citation comments (`<!-- pylon:referenced-doc: <slug> -->`). Compute a deterministic utility score for each document: citation count divided by injection count, combined with recency decay.
- **Automated Lifecycle**:
  - *Quirk Pruning*: Demote or archive quirks that appear in context repeatedly without citations, preventing context bloat from obsolete bugs.
  - *Pattern Promotion*: Detect quirks or conventions cited frequently across multiple packages. Propose promotion to formal platform conventions in `conventions/` or automated rules in `pylon lint`.
- **Consumer**: Filter input for `pylon context` ranking and periodic hygiene reviews.
- **Objective**: Keep context lean and high-signal. Weed out obsolete quirks and elevate proven operational patterns to formal engineering standards.

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
