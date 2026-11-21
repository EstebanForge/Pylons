---
title: "Transition-graph parity"
type: plan
status: proposed
created: 2026-11-20
updated: 2026-11-20
owners:
  - esteban
ticket: []
artifact: ""
target_release: ""
components: []
links: []
description: "Extend lint --parity from state sets to transition triples: a declared lifecycle table checked against a backed transition map in the mapped project."
slice: false
scenarios: []
tags: [roadmap, parity, fsm]
contract:
  stable: []
  unstable: []
  editing: []
---

# Transition-graph parity

## Problem

`lint --parity` today checks a closed state set against a backed enum. A lifecycle is more than its cases: states change only along declared transitions, and the shipped set check cannot see a triple. A code change can add a transition, drop one, or leave a state unreachable, and the ledger still reads as parity. The roadmap carries the design at Priority 1; this plan turns it into an ordered build.

## External contract (code-checked)

The comparison extends cli/internal/rules/parity.go. The catalog tops out at PYLON052, so the new rules register from PYLON053. The declaration machinery reuses what the set checks shipped: `states:` and `states_symbol:` on a domain entity. Resolution goes through SafeJoin, and extraction reuses the per-language extractor table in extractEnum.

## Design

A lifecycle becomes a formal machine: one closed `State` set, one backed `Event` enum beside it, and one pure `(State, Event) -> State` transition map. The doc side declares the triples as structured frontmatter on the domain entity, plus mandatory `initial:` and `terminal:` sets. The code side is a package-level nested map keyed by state case value, then event case value, with literal case values only. Purity, throw-on-invalid, and compare-and-swap persistence are convention prose plus a table test in the mapped repo's QA suite; lint cannot prove them.

Declaration is explicit in both directions. A `// pylon:machine <id>` doc-comment directive sits above the map in code, and a `machine:` front-matter key on the domain entity names the same target. No enum auto-discovery: discovery by proximity is a heuristic and violates the determinism constraint.

### Extracted shape

One canonical form per language. Go first: a package-level `var transitions = map[State]map[Event]State{...}` literal with typed const identifiers as keys and values. Extraction walks the fixed `go/parser` node shape: GenDecl, ValueSpec, CompositeLit, KeyValueExpr. The AST resolves the two enum case sets and the triples `(from, event, to)`. Everything else is out of extractor scope: computed keys, spreads, non-literal `to` values, and runtime-built maps such as statechart libraries. An extractor that cannot fully resolve a declared machine errors instead of skipping. Go rejects duplicate constant map keys at compile time, so PYLON058 stays language-neutral with the compiler as Go defense in depth.

### Rules

Severities are lint severities, registered in the catalog. `FORCED` and `ADVISORY` check grades stay plan-signal-only and never appear in lint output.

- `PYLON053 TRANSITION_UNPARSEABLE` (error): a declared machine whose map cannot be fully resolved. Fail closed.
- `PYLON054 TRANSITION_MISMATCH` (error): a doc triple with no code triple, or the reverse.
- `PYLON055 UNKNOWN_STATE_IN_TRIPLE` (error): a triple names a state absent from the enum.
- `PYLON056 UNREACHABLE_STATE` (error): a case with no inbound transition and not declared initial.
- `PYLON057 DEAD_STATE` (error): a case with no outbound transition and not declared terminal. Batch machines must reach a terminal.
- `PYLON058 DUPLICATE_TRANSITION` (error): two triples share `(from, event)` with different `to`. Map literals silently overwrite duplicates; the AST still sees both.
- `PYLON059 BYPASSED_STATE_WRITE` (advisory, ratchet-fed): the persisted state key named by the machine is written outside the machine's surface.

### Generation and binding

v1 ships comparison only: the declared frontmatter triples are parsed, checked against the code map, and the rules fire. Generation lands after the first real machine passes. It re-marshals one structured frontmatter key, the manifest-regen model. A Mermaid `stateDiagram-v2` render beside it is free output of the same triples. The roadmap's marker-delimited table is superseded; structured frontmatter is the declared format. Plans touching a machine list the transition map and the state enum as `symbol` contracts, so `contract.editing` and `contract.unstable` apply with no new engine code. A removed or changed triple escalates the merge door to one-way, because persisted data can sit in a now-illegal state; an added triple stays two-way.

### Consumer surfaces

`pylon lint` blocks on the errors; the advisory feeds `advisory-ratchet`. `pylon context` prints the graph beside contract entries. `pylon brief` reports the machine count and the count missing `verified_by:`. The machine declaration carries `verified_by:` naming one table test; lint resolves it by set membership. The test derives its legal triples from the same const as the full cross product of states and events, and asserts the complement throws.

## Milestones

- [ ] M0: Pilot gate. Register one real project, any language with a Go front end, in manifests/pylons.yaml. Add the `state-transition-contracts.md` convention entry (proposed). No implementation before this milestone.
- [ ] M1: Go extractor. Parse the declared enums and the map with `go/parser`. Add red-first tests over a fixture machine. Unparseable shapes fail closed as PYLON053.
- [ ] M2: Declared side. The `machine:` key, `transitions:` triples, `initial:`, and `terminal:` as domain-entity frontmatter, validated by the parser. No generator in v1.
- [ ] M3: Comparison and graph rules. Triple-set intersection in both directions, plus PYLON054 through PYLON058, each with a red-first test.
- [ ] M4: Advisory and proofs. PYLON059 via the import-boundary write scan; `verified_by:` resolution; `context` and `brief` surfacing.
- [ ] M5: Merge-door escalation wiring, ship-rule docs (USAGE, FEATURES, CHANGELOG, roadmap deletion), and the ship commit.
- [ ] M6: Generation. The frontmatter re-marshal with `--check` drift and the Mermaid render, after the first real machine exists.

## Open questions

- Pilot: which mapped project and lifecycle go first? Any project qualifies; the choice shapes the first real extractor fixture.
- Marker syntax for Go: a doc-comment directive beside the map, or the `machine:` key alone? Pick one default.
- Resolved: comparison-only in v1. Generation follows the first real machine (M6).
- Resolved: a package-level `var` literal with const identifier keys, marked by a `// pylon:machine <id>` directive.
