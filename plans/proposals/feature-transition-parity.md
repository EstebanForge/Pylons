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
description: "Extend lint --parity from state sets to transition triples: language profiles extract the backed machine from any mapped stack, and the declared frontmatter triples are checked against it."
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

`lint --parity` today checks a closed state set against a backed enum. A lifecycle is more than its cases: states change only along declared transitions, and the shipped set check cannot see a triple. A code change can add a transition, drop one, or leave a state unreachable, and the ledger still reads as parity. The roadmap carries the design at Priority 1; this plan turns it into an ordered build. It also fixes the product gap. Today the extractors are a hard-coded table inside parity.go, so a stack without a built-in extractor cannot use parity at all. End users must be able to configure parity for their own languages, the way they already configure contract kinds.

## External contract (code-checked)

The comparison extends cli/internal/rules/parity.go. The catalog tops out at PYLON052, so the new rules register from PYLON053. The declaration machinery reuses what the set checks shipped: `states:` and `states_symbol:` on a domain entity. Resolution goes through SafeJoin, and extraction reuses the per-language extractor table in extractEnum.

## Design

A lifecycle becomes a formal machine: one closed `State` set, one backed `Event` enum beside it, and one pure `(State, Event) -> State` transition map. The doc side declares the triples as structured frontmatter on the domain entity, plus mandatory `initial:` and `terminal:` sets. The code side is a package-level nested map keyed by state case value, then event case value, with literal case values only. Purity, throw-on-invalid, and compare-and-swap persistence are convention prose plus a table test in the mapped repo's QA suite; lint cannot prove them.

Declaration is explicit in both directions. A profile-defined declaration marker sits above the map in code, and a `machine:` front-matter key on the domain entity names the same target. No enum auto-discovery: discovery by proximity is a heuristic and violates the determinism constraint.

### Product surface: language profiles

Extraction is a configuration surface, not a built-in table. The engine is language-neutral. Each language carries a parity profile that answers two protocol requests. Request one returns the case lists for the `State` and `Event` enums. Request two returns the transition triples from the declared map. Users configure parity per project exactly as they configure contract kinds today. They enable the language in `manifests/pylons.yaml` and name the target with a `repo://` URI in the frontmatter. Nothing new to learn.

Three profile tiers serve any stack:

- Built-in profiles ship for every language the ledger already serves, plus csharp and ruby. The set: go, rust, typescript, javascript, python, php, csharp, and ruby. They run in-process for speed and need no external binary.
- External profiles: an executable `pylon-ext-<name>` answering the new protocol request kinds. It rides the existing JSON stdin/stdout v1 protocol, the 5-second timeout, and `--allow-external-extensions`. A stack with no built-in profile works by dropping in a plugin and enabling `<name>`.
- Nothing configured, nothing judged: a project with no matching profile yields the fail-closed PARITY_NOT_EVALUATED advisory, never a silent pass.

The canonical code shape is a per-profile contract, documented where the profile is documented, not one global form. The shipped set checks move behind the same interface, so one profile serves both the state-set and transition checks.

### Extracted shape: per profile

Each profile pins one canonical form for its language, plus the marker that declares it. The Go reference profile ships first. Its canonical form is a package-level `var transitions = map[State]map[Event]State{...}` literal with typed const identifiers as keys and values, marked by a `// pylon:machine <id>` doc-comment directive. Extraction walks the fixed `go/parser` node shape: GenDecl, ValueSpec, CompositeLit, KeyValueExpr. The AST resolves the two enum case sets and the triples `(from, event, to)`. Everything else is out of extractor scope for every profile: computed keys, spreads, non-literal `to` values, and runtime-built statechart libraries. A profile that cannot fully resolve a declared machine errors instead of skipping. Go rejects duplicate constant map keys at compile time, so PYLON058 stays language-neutral with the compiler as Go defense in depth. The php profile targets a class-level const map keyed by case values. The rust profile targets a match table or const map. The csharp, javascript, and ruby profiles join the built-in set out of the box and pin their canonical forms at profile time.

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

- [ ] M0: Pilot gate. Register one real project, any stack, in manifests/pylons.yaml. Add the `state-transition-contracts.md` convention entry (proposed). No implementation before this milestone.
- [ ] M1: Profile protocol. A language-neutral parity profile interface in the engine, with the shipped set-check extractors behind it. The extensions protocol and schema gain the two request kinds, including the external plugin path. Red-first.
- [ ] M2: Go reference profile. The package-level map literal and directive marker, parsed with `go/parser`. Red-first tests over a fixture machine; unparseable shapes fail closed as PYLON053.
- [ ] M3: Declared side. The `machine:` key, `transitions:` triples, `initial:`, and `terminal:` as domain-entity frontmatter, validated by the parser. No generator in v1.
- [ ] M4: Comparison and graph rules. Triple-set intersection in both directions, plus PYLON054 through PYLON058, each with a red-first test.
- [ ] M5: Advisory and proofs. PYLON059 via the import-boundary write scan; `verified_by:` resolution; `context` and `brief` surfacing.
- [ ] M6: Merge-door escalation wiring, ship-rule docs (USAGE, FEATURES, CHANGELOG, roadmap deletion), and the ship commit.
- [ ] M7: Generation. The frontmatter re-marshal with `--check` drift and the Mermaid render, after the first real machine exists. The remaining built-in profiles land behind the same interface, priority set by the stacks that adopt the pilot.

## Open questions

- Pilot: which mapped project and lifecycle go first? Any project qualifies; the choice shapes the first real extractor fixture.
- Resolved: comparison-only in v1. Generation follows the first real machine (M7).
- Resolved for the Go reference profile: a package-level `var` literal with const identifier keys, marked by a `// pylon:machine <id>` directive. Other profiles pin their own canonical forms when they land.
