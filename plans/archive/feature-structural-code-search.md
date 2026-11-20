---
title: "Structural code similarity"
type: plan
status: shipped
created: 2026-11-20
updated: 2026-11-20
owners:
  - esteban
ticket: []
artifact: "the pylon aroma subcommand"
target_release: ""
components: []
links: [pylon://../../meta/ROADMAP.md]
description: "Structural code similarity inside the pylon CLI. It finds methods whose code resembles a query and returns the code they share. No model, no network."
slice: false
scenarios: []
tags: [similarity, retrieval, zero-model]
contract:
  stable: []
  unstable: []
  editing: []
---

# Structural code similarity

## Problem

Pylons checks the map against the code. It says nothing about the code that already exists. An engineer changing a function has no fast way to ask a question. Which functions in this codebase already do something like this? What code do they carry with them?

The Aroma paper (Luan, Yang, Barnaby, Sen, Chandra; OOPSLA 2019, arXiv:1812.01158) answers this with a deterministic pipeline. It searches code structurally over featurized parse trees, then clusters alike results into shared-code recommendations. A TypeScript reference implementation proves the algorithm and ships a test suite. The tool reads no files, calls no model, and writes nothing.

The use is wider than review. Three readers want the same answer: the code that already does this, and the code it carries with it. An engineer explores an unfamiliar repo. An agent orients before a change. A reviewer judges a diff.

## Decision: inside the CLI

The capability ships as the `pylon aroma` subcommand. It shares the CLI's config: with no `--corpus`, the corpus is the ledger's registered projects, else the current directory. The output is advisory and never gates. Its JSON output carries a schema version, which is the only coupling point for a later bridge into context or reviews.

## Provenance

The core is a port of the reference implementation, which is MIT licensed: L9Labs/aroma-typescript at commit 7a4224a4cc73f4bff954e3e9ebc078aae5a7d509. The port is a derivative work. The MIT copyright and permission notice must travel in this repo's NOTICE file. The paper gets cited in the README. New code is Apache-2.0.

## Fidelity policy

Precedence when sources disagree: the TypeScript reference wins where the golden tests exercise the point, then the paper, then this plan. The reference README documents its departures from the paper; the port inherits each one and lists it here:

- Cluster cohesion divides by the first pruned snippet. This is a bug fix. The paper's formula divides by the unpruned first method. Under the paper's expansion rule l(τ) > 1.5, that ratio can never exceed 0.67. So the paper's cohesion test s(τ) > 0.9 is unsatisfiable as written.
- `queriesFromLines`: changed lines featurize inside their method, so ancestors and locals are known. The paper's queries are standalone snippets.
- Whole-line output with `query`/`added`/`structure` roles replaces the paper's highlighted fragments.
- `minSupport` 2: a recommendation needs two or more members. The paper's singleton recommendation (Section 4.3.5) is a whole unpruned method. Plain search already returns that, and the category was the paper's weakest.
- Origin and exclude handling with copy tracking: a pasted copy of a method is still found.
- Literal clipping at 48 characters, and `true`/`false`/`null` treated as literals.
- Snippet local guess: an undeclared snippet name counts as a local unless it is called, capitalized, or global.
- Method-level dedupe by hash; the paper dedupes at project, file, and method level.

Where the paper is silent, the reference fills the gap and the port follows, each recorded as a decision:

- τ1 normalization: a share of the query's features (needed for 0.71 to mean anything).
- Jaccard overlap sets: member lists.
- Prune ties: the earlier token wins.
- Intersect: multiset union throughout (the paper mixes union forms).
- Child index counts non-keyword children only.
- The `#.#` context falls back to a positional context when no non-local token exists.
- Fewer than three ancestors yields fewer features.

The thresholds (η1=1000, η2=100, τ1=0.65, τ2=1.5, τ3=0.9, K=5, `--min` 0.71) keep their paper values, documented as uncalibrated for Go until the post-v1 calibration runs. The 0.71 derives from the paper's case study: the first quartile of human-labeled similar pairs.

## Design

### Core (language-agnostic)

A new package holds the algorithm. It ports the reference implementation with the paper's semantics:

- Tree and token model. One `Token` holds text, a keyword flag, a local-variable flag, and a line span. One `Tree` holds ordered children and no node types. The core `Tree` constructor enforces the paper's rule: a list never holds a single subtree alone.
- Featurizing (paper 3.2). Each non-keyword token carries its own feature. It carries up to three ancestor features (label plus 1-based non-keyword position). It carries a sibling feature with its next non-keyword neighbor. For local variables it carries a usage feature joining the contexts of two consecutive uses. Every local variable is `#VAR`. Names and literals over 48 characters clip to start plus length.
- Corpus and index. Methods come from a walk over source roots. Deduplication groups methods by token content, local variables, and nesting; copies stay recorded. The index holds one inverted posting list per distinct feature. Scoring is sparse (paper 3.3.1).
- Search has two phases. Phase I keeps the top 1000 methods (η1) by distinct-feature overlap within one language. Its vectors are binary: a method repeating a feature does not contain the query more. Phase II prunes each candidate to the tokens that resemble the query (Algorithm 1, greedy by gain, bucket-rescan shortcut). It reranks by the share of the query the pruned snippet carries. SimScore here is a multiset intersection, so counts matter again.
- Recommend (Phase III) clusters the top 100 (η2) above 0.65 (τ1). Cluster growth is forward-only: candidates come from the index after the last joined member, and the best next member wins. A seed with size over 1.5 times the query share (τ2) is skipped. Cohesion is the shared share over the first pruned snippet, kept above 0.9 (τ3). It drops a cluster whose member list overlaps an earlier cluster's with a Jaccard of 0.5 or less. It returns up to 5 recommendations (K). Each line gets a role: `query`, `added`, or `structure`. `added` wins over `query` per line. Structure lines come from keyword tokens in the kept tree paths.
- Intersect adds the query's features to each target's counts. `prune` chains: each cluster member's candidate set starts from the previously kept tokens. These two rules are what keep query code visible in a recommendation.
- Every similarity and expansion comparison is strict `>`. `--min` therefore means "greater than 0.71" by default.
- Queries from changed lines featurize inside their method. There the ancestors and variables are known (the reference implementation's `queriesFromLines`). This lives in the core, not the language front end. Such a query never matches the method it came from.
- A query whose feature count is under 30 is weak signal. The tool still runs it. The JSON carries `query_size` and a `low_signal` flag below 30, and human output warns. `--min-query-size` (default 0) drops weaker queries entirely when set.
- Sorts are stable. Phase I ties break by method id. Prune ties go to the earlier token. Method ids are assigned after a sort by path, then ordinal, so a worker pool stays deterministic.
- Feature ids are interned in one serial merge and never reach output. Only sets and multisets use them, so their order is free.

### Language front end

One interface, the only language-specific step:

```go
type Language interface {
    ID() string
    Extensions() []string
    MemberAccessLabels() []string
    ParseMethods(source, path string) ([]Method, error)
    ParseSnippet(source string, options SnippetOptions) (Node, error)
}

type Method struct {
    Name      string
    StartLine int
    EndLine   int
    Body      Node
}
```

`Token` and `Node` carry line and end-line spans plus the keyword flag. `SnippetOptions` carries `Path` and `Globals` (the dialect's unresolved-identifier allowlist). `ParseMethods` returns an error so per-file diagnostics reach the stats line without failing the corpus.

Version one ships a Go front end on stdlib `go/parser` and `go/token`. Its duties:

- Method boundaries are `FuncDecl` bodies with a body, plus package-level `FuncLit`. A `FuncLit` inside a function belongs to its method. Names are `Recv.Method`. The body excludes parameters and the name.
- Keyword and punctuation tokens are rebuilt Lezer-style. Walk each node's direct children. Scan the source text in the gaps with `go/scanner`, comments skipped. This keeps source order.
- The tree builder follows the reference's builder, not just the core rule. A list that would hold one non-keyword subtree alone collapses into it; the same applies to a single-name list. Go's AST wraps statements and names (`ExprStmt`, `ParenExpr`) the way Lezer does, and the collapse keeps ancestor slots honest. The core `Tree` constructor still enforces the paper's rule for hand-built toy trees.
- `true`, `false`, `nil`, and `iota` are `*ast.Ident` in Go but literals or globals elsewhere. They get special-cased to match the reference's literal handling.
- Local variables resolve through `ast.Ident.Obj`, keyed by name per method (shadowed scopes collapse, as the reference does). A variable is local only when its declaration lies inside the method's range. Package-level vars are globals.
- Snippets parse wrapped in `package p; func _() {...}` with a line-offset fix, falling back to top-level declarations, then a lone expression. A snippet cannot see imports, so `fmt.Println` would featurize `fmt` as `#VAR`. Mitigation: an unresolved identifier that is the selector's `X` of a standard-library package counts as a global, extendable through `--globals`.
- Syntax errors keep the partial AST; a nil AST skips the file and counts in the stats. Defaults skip `vendor/`, `testdata/`, generated files (`// Code generated`), and `_test.go`, each overridable by flag.

Tree-sitter native bindings stay out: the release builds are `CGO_ENABLED=0`. For wide coverage without cgo, the noted option is a pure-Go WebAssembly runtime (wazero) running tree-sitter grammars; it is cut from the first release. `go/parser` ignores build tags and parses cgo files, so recall figures from the paper do not carry over to Go; the README says so.

### CLI

Report-only, advisory always:

```
<tool> search --corpus <dir> [--lang go] --lines <path>:120-160[,180-190]...
<tool> search --corpus <dir> --snippet <file|->
<tool> recommend --corpus <dir> --lines <path>:120-160 [--json]
<tool> recommend --corpus <dir> --diff <file|->
```

- `--corpus` is repeatable and defaults to the current directory. `--lang` selects the front end; the default infers it from extensions. The query file may also live in the corpus; origin exclusion (path plus line overlap) applies. All paths store root-relative and slash-normalized, so `./a.go`, `a.go`, and an absolute path name the same file.
- Input shapes: one or more `path:start-end` line ranges. Or a snippet file with `-` for stdin. Or a unified diff on stdin that yields queries from changed lines. The diff path is the real reviewer and agent workflow: one index pass answers every changed hunk.
- The corpus is indexed once per invocation. Output is human by default, JSON with `--json`. Exit 0 on any result set, including an empty one. Exit 2 on usage and unreadable corpus roots. Exit 1 when the corpus yields zero indexable files. Per-file parse errors never set exit 1. A query that yields no tokens exits 0 with an empty set and a stderr note. A search never blocks a gate.
- `--min-support` (default 2) controls the recommendation member floor. Setting it to 1 reproduces the paper's singleton recommendations. Those are whole methods and duplicate what `search` returns. The README documents this against the paper's Section 4.3.5.
- One stderr stats line reports files, methods, duplicates, skips, and elapsed milliseconds.
- All file reads are bounded (a byte cap, regular files only) and skips are counted. Indexing parses and featurizes in parallel per file. Then one serial merge sorts, assigns ids, interns features, and builds the posting lists as flat `[]int32` slices. Phase I scoring reuses one dense buffer per run.
- Feature multisets are sorted slices, not maps, so prune and cluster stay allocation-light. Sources stay out of memory. Render re-reads a file when it changed on disk (stat and hash check), keyed by the method's stored ordinal.

### Output stability

JSON carries a `schema_version`, the query (size, low-signal flag, lines, origin), results, and recommendations with per-line roles. Similarity is rounded to 4 decimals so platform float differences never show. Results sort by similarity, then path, then start line. Recommendations keep the reference's cluster order. Human output inserts a `...` gap marker between non-adjacent recommended lines, after the paper's own placeholder convention. JSON needs no marker; it carries line numbers.

### Non-ASCII note

The reference clips long names in UTF-16 code units; Go strings are bytes. The port clips by rune count. For non-ASCII literals over 48 characters the port differs from the reference, and the toy goldens are ASCII so they will not catch it. The README documents this.

### Cost

The reference indexes 57,770 methods in about 55 seconds and holds about 480 MB, with each search near 70 ms. Stateless reindexing is right for a repo of a few thousand methods and wrong for a monorepo. The index is therefore serializable by design, and a cache under an opt-in directory is an explicit later milestone, not an open question. The first release ships batch input so one index pass answers every query in a run. At a checkout's scale (thousands of methods), Phase I's top-1000 cut barely cuts, so Phase II prunes most of the corpus. That costs time, not correctness, and is cheap at this size.

### Verification

Three layers, from exact to statistical:

- The toy differential. A seeded random generator of toy-language programs is written twice, once in TypeScript and once as a Go test helper. Both cores run `search`, `recommend`, and `queriesFromLines` over the same generated corpus. The TypeScript outputs are committed as goldens, with the reference commit SHA in the header. A `refresh-goldens` target needs Node once; CI replays the checked-in JSON and never needs Node. This asserts exact equality of ranked lists, clusters, and lines, not just recall.
- Go-front-end characterization goldens. They are hand-written, because no reference Go front end exists. They cover the paper's Figure 4 and Listing 4 translated to Go, plus the `view`/`innerView` example. One test covers each hard case above. Go-front-end fidelity against the reference is not machine-verifiable; the README says so.
- The paper's recall benchmark as a Go acceptance test. Its protocol (Section 5) runs on a pinned Go corpus: a vendored stdlib subset, fixed by version. Queries are 5 contiguous and 5 random lines from methods of at least 12 lines. They are built in snippet mode, with fixed seeds. The core needs an internal include-origin hook for this test, since `queriesFromLines` excludes origin by default. The gate starts at Recall@1 at or above 90 percent and Recall@100 at or above 99 percent. That is looser than the reference's own snippet-mode numbers; the first run's results calibrate the gate.
- A benchmark indexes a mid-size corpus and asserts a bound on allocations, not wall time.

## Milestones

- [ ] M0: core port green. Tree, token, collapse rule, featurize, against a hand-verified test from the paper's Table 2 (Figure 4 and Listing 4).
- [ ] M1: search end to end. Corpus, index, dedupe, Phase I and II, plus a CLI skeleton over the toy language.
- [ ] M2: Go front end as its own milestone. `ParseMethods` and `ParseSnippet`, local-variable resolution, snippet globals, plus `queriesFromLines`, origin exclusion, and path canonicalization in the core.
- [ ] M3: recommend and clustering green against ported tests.
- [ ] M4: goldens, recall acceptance test, output spec, diff input, batch queries, stats line, gap markers.
- [ ] M5: provenance (NOTICE, citations), README with limitations, license files, release tooling, first release. Index cache if wanted.
- [ ] M6 (post-v1): threshold calibration for Go. Replicate the paper's Figure 1 protocol on a Go corpus. Hand-label about 50 query and result pairs. Take the first quartile of the similar scores as the Go `--min` default. Until then the README labels all thresholds "paper values, uncalibrated for Go".

Cut from the first release: stdin file lists, wazero, multi-language text, and the cache.

## Open questions

- Whether pylon later grows the bridge into context or reviews. If it does, the versioned JSON schema is the coupling point.
