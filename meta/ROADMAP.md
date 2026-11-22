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

### retrieval-recall-suite
- **Deterministic Mechanism**: A golden query-to-doc recall test. A fixed sample ledger plus a query set with known target docs; the test asserts recall@K thresholds. Extend the `_examples/ledger` fixture or ship a dedicated test ledger. Runs inside the standard Go suite; no separate binary or cache.
- **Consumer**: The gate for every ranking change: `search-index-hygiene`, `rank-fusion`, `index-term-expansion`, `query-expansion`, and any future `knowledge-utility` scoring. A ranking change that drops recall below the threshold fails the suite.
- **Objective**: Ranking changes prove themselves against fixed expectations, never vibes.

### search-index-hygiene
- **Problem**: The search index feeds on the raw document file. YAML keys (`status:`, `created:`) pollute term frequency, and structured metadata (type, status, package, components, owners) stays invisible to queries unless prose repeats it.
- **Change**: Strip frontmatter from the indexed text (`resolve.WithoutFrontMatter`) and prepend a deterministic context header built from `DocMeta`: title, type, status, package, components, owners, tags. The header is situated context generated from enforced frontmatter, never from a model. This is the retrieval counterpart of the strict frontmatter work: the machine data the engine enforces becomes the machine data the index reads.
- **Where**: `scanDocs` and `filterDocs` in `cli/cmd/pylon/main.go`; `resolve/bm25.go` stays untouched.
- **Tests**: Frontmatter keys must never influence ranking. A query naming a component or owner must surface the owning doc without that term in prose.
- **Deferred**: Heading-level chunking. STE sentence caps keep ledger docs short, so whole-document units stay until real ledgers outgrow them.
- **Objective**: Search answers "which doc owns this quirk" without the answer appearing in body prose.

### rank-fusion
- **Scope**: No new command. `pylon map` and `pylon search` gain a deterministic second-stage rerank.
- **Deterministic Mechanism**: Stage one: BM25 keeps a candidate pool, mirroring the η1/η2 two-phase pattern already proven in the aroma ranker. Stage two: a closed rerank-signal set adjusts scores: exact identifier hits (rule ids, `repo://` URIs, machine ids), status priority (active and accepted above terminal states), recency from `updated:`, citation counts once `knowledge-utility` ships, and contract or file proximity when the ledger is queried from inside a registered project. Signals carry fixed weights in code; weights stay non-configurable until a real ledger needs tuning.
- **Where**: A rerank pass beside `filterDocs` in `cli/cmd/pylon/main.go`.
- **Tests**: The `retrieval-recall-suite` gates every weight or signal change.
- **Objective**: Hybrid retrieval beats single-signal. The ledger's own structured graph is the second signal, so no embeddings are needed.

### index-term-expansion
- **Deterministic Mechanism**: Append linked domain-term titles and tags into the context header from `search-index-hygiene`. One hop only: terms named in a document's `links:`, plus `domain/` term matches in prose (the graph `PYLON005` already forces to exist). No recursion.
- **Where**: Header construction beside `filterDocs`; reuse the doc graph `scanDocs` already loads.
- **Tests**: A document citing a term without naming it in prose must still surface for queries on that term's synonyms.
- **Objective**: Glossary-defined vocabulary makes a document retrievable before its prose uses the vocabulary.

### query-expansion
- **Deterministic Mechanism**: When the invocation names a file path or a registered package, expand the query before ranking: resolve the path through `normalizeCheckTarget`, add the owning package's name and components, add dependency-graph neighbor names at depth one. Human output labels the expansion terms; JSON carries them as an explicit field.
- **Where**: The `pylon search` dispatch in `cli/cmd/pylon/main.go`.
- **Tests**: A query naming a project source file must rank the owning domain entity first, with no manual query terms.
- **Objective**: The caller's working context improves recall, deterministically.

## Explicitly Not Adopted

- **Hosted context runtime.** The agent harness owns sessions. No external portal dependency.
- **Model harness layer.** Model choice stays at the agent level, not inside Pylons.
- **Proprietary session store.** All captured context lands in git under the KB root. Reviewable, blameable, durable.
- **Auto-commit.** Every capture produces a candidate patch. A human or an approved agent skill commits it. The engine never writes to `main` directly.
- **Multi-agent dispatcher daemon.** No heavy background daemon or multi-process hook router. Local operation requires only lightweight detached fetch operations.

Directions, not commitments. Scope and order shift with the platform.
