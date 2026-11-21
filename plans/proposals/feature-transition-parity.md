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

`lint --parity` today checks a closed state set against a backed enum. A lifecycle is more than its cases: states change only along declared transitions, and the shipped set check cannot see a triple. A code change can add a transition, drop one, or leave a state unreachable, and the ledger still reads as parity. The roadmap carries the design at Priority 1; this plan turns it into an ordered build. It also fixes the product gap. Today the extractors are a hard-coded table inside parity.go, so a stack without a built-in extractor cannot use parity at all. The rule is: the CLI provides the plumbing, users configure the rest. The CLI reads a declarative spec, validates it, and acts on it. This ledger never registers a live project in its own manifest. Verification runs on fixture machines in the test suite. Adoption happens in the user's repo, by configuration.

## External contract (code-checked)

The comparison extends cli/internal/rules/parity.go. The catalog tops out at PYLON052, so the new rules register from PYLON053. The declaration machinery reuses what the set checks shipped: `states:` and `states_symbol:` on a domain entity. Resolution goes through SafeJoin, and extraction reuses the extractor selection in extractEnum, today an extension-based if chain that M0 refactors behind the profile interface. One shipped asymmetry this plan inherits and pins: the set checks read the working tree, while contract resolution honors a configured git ref. Parity extraction reads the working tree on both sides.

## Design

A lifecycle becomes a formal machine: one closed `State` set, one backed `Event` enum beside it, and one pure `(State, Event) -> State` transition map. The doc side declares each machine as one entry of a `machines:` front-matter list on the domain entity. Every entry carries its id, `states:` set, `transitions:` triples, and mandatory `initial:` and `terminal:` sets. The list form is pinned from day one, because real entities carry several machines. The legacy migration in one adoption tree holds an arm/disarm lifecycle and a migration phase. Order-shaped entities in the other stack carry order status, payment status, and subscription status at once. The code side is a package-level nested map keyed by state case value, then event case value, with literal case values only. Purity, throw-on-invalid, and compare-and-swap persistence are convention prose plus a table test in the mapped repo's QA suite; lint cannot prove them.

Declaration is explicit in both directions. A profile-defined declaration marker sits above the map in code, and a `machines:` front-matter entry on the domain entity names the same target. No enum auto-discovery: discovery by proximity is a heuristic and violates the determinism constraint.

### Product surface: language profiles

Extraction is a configuration surface, not a built-in table. The engine is plumbing: it reads a declarative parity spec, validates it against a schema, and drives everything from it. Profile selection, the two extraction requests (State/Event case lists; transition triples), and the comparison all follow the spec. Users configure parity per project through a `parity:` section in `manifests/pylons.yaml` and name the target with a `repo://` URI in the frontmatter. Nothing new to learn.

Three profile tiers serve any stack:

- Built-in profiles ship for every language the ledger already serves, plus csharp and ruby. The set: go, rust, typescript, javascript, python, php, csharp, and ruby. They run in-process for speed and need no external binary.
- External profiles: an executable `pylon-ext-<name>` answering the new protocol request kinds. It rides the existing JSON stdin/stdout v1 protocol, the 5-second timeout, and `--allow-external-extensions`. A stack with no built-in profile works by dropping in a plugin and enabling `<name>`.
- Nothing configured, nothing judged: a declared machine with no matching profile yields the PARITY_NOT_EVALUATED advisory, never a silent pass. An `on_unevaluated:` spec knob (advisory or error, default advisory) lets a CI adopter make it block.

The binding form is explicit and override-capable: `profiles.<language>: {use: builtin:<language>}` or `{use: external:<plugin>, options: {...}}`. Rebinding a language to a plugin overrides the builtin with no implicit precedence. With no `parity:` section at all, an enabled builtin extension acts as an implicit builtin binding, so existing ledgers keep working. Two profiles claiming the same file extension is a config error. The parity subtree is a closed schema, unlike the lenient rest of the manifest. Unknown keys there are a hard error, so a typo can never silently disable a check. Free-form growth goes only into the per-profile `options:` bag.

The canonical code shape is a per-profile contract, documented where the profile is documented, not one global form. The shipped set checks move behind the same interface, so one profile serves both the state-set and transition checks.

### Extracted shape: per profile

Each profile pins one canonical form for its language, plus the marker that declares it. The Go reference profile ships first. Its canonical form is a package-level `var transitions = map[State]map[Event]State{...}` literal with typed const identifiers as keys and values, marked by a `// pylon:machine <id>` doc-comment directive. Extraction walks the fixed `go/parser` node shape: GenDecl, ValueSpec, CompositeLit, KeyValueExpr. The AST resolves the two enum case sets and the triples `(from, event, to)`. Everything else is out of extractor scope for every profile: computed keys, spreads, non-literal `to` values, and runtime-built statechart libraries. A profile that cannot fully resolve a declared machine errors instead of skipping. Go rejects duplicate constant map keys at compile time, so PYLON058 stays language-neutral with the compiler as Go defense in depth. The php profile targets a class-level const map keyed by case values. The rust profile targets a match table or const map. Every non-Go profile pins a line-parseable canonical form, because line-based extraction cannot trust nested structures. The csharp, javascript, and ruby profiles join the built-in set out of the box and pin their canonical forms at profile time. One token rule spans every profile. A triple compares literal case values only when every case of that enum carries one, else identifiers. The rule is pinned per enum, never mixed.

### Rules

Severities are lint severities, registered in the catalog. `FORCED` and `ADVISORY` check grades stay plan-signal-only and never appear in lint output.

- `PYLON053 TRANSITION_UNPARSEABLE` (error): a declared machine that cannot be fully resolved on either side. Declaration side: a missing or duplicate machine id, a missing `initial:` or `terminal:`, a bad triple shape, an unresolved target. Code side: an unresolvable map. Fail closed.
- `PYLON054 TRANSITION_MISMATCH` (error): a doc triple with no code triple, or the reverse.
- `PYLON055 UNKNOWN_STATE_IN_TRIPLE` (error): a triple names a state absent from the enum.
- `PYLON056 UNREACHABLE_STATE` (error): a case with no inbound transition and not declared initial.
- `PYLON057 DEAD_STATE` (error): a case with no outbound transition and not declared terminal. Batch machines must reach a terminal.
- `PYLON058 DUPLICATE_TRANSITION` (error): two triples share `(from, event)` with different `to`. Map literals silently overwrite duplicates; the AST still sees both.
- `PYLON059 BYPASSED_STATE_WRITE` (advisory, ratchet-fed): the persisted state key named by the machine is written outside the machine's surface.

Three more pins complete the semantics. The doc side carries its own identity. Each machine entry's id must equal its code marker id; a duplicated machine id, in one entity or across the repo, is an error. Graph checks (unreachable, dead, duplicate, unknown state) are doc-only and run without a profile. A declared machine is therefore graph-checked even before its code side is bound. Plugin infrastructure failures (missing binary, timeout, crash, invalid JSON, disabled externals) are separate from plugin verdicts. Infrastructure yields PARITY_NOT_EVALUATED with a cause; a plugin that runs but cannot resolve the shape yields PYLON053. The engine sorts the extracted triple list before comparing, so PYLON058 can see duplicates the map literal would overwrite.

### Generation and binding

v1 ships comparison only: the declared frontmatter triples are parsed, checked against the code map, and the rules fire. Generation lands after the first real machine passes. It re-marshals one structured frontmatter key, the manifest-regen model. A Mermaid `stateDiagram-v2` render beside it is free output of the same triples. The roadmap's marker-delimited table is superseded; structured frontmatter is the declared format. Plans touching a machine list the transition map and the state enum as `symbol` contracts, so `contract.editing` and `contract.unstable` apply with no new engine code. A removed or changed triple escalates the merge door to one-way, because persisted data can sit in a now-illegal state; an added triple stays two-way.

### Consumer surfaces

`pylon lint` blocks on the errors; the advisory feeds `advisory-ratchet`. `pylon context` prints the graph beside contract entries. `pylon brief` reports the machine count and the count missing `verified_by:`. The machine declaration carries `verified_by:` naming one table test; lint resolves it by set membership. The test derives its legal triples from the same const as the full cross product of states and events, and asserts the complement throws.

## Milestones

- [ ] M0: Parity spec and profile protocol. The `parity:` section in manifests/pylons.yaml carries its own required `version:`, closed-schema validation, and a per-project override. The schema ships as manifests/parity.json beside extensions.json. The `use:` binding arrives with ownership-conflict validation and builtin override. A language-neutral profile interface replaces the extractor if chain. A parity ops dispatcher keyed by profile name, separate from the dash-prefix kind routing. A reference external plugin under testdata. Red-first, verified on fixture machines only.
- [ ] M1: Convention entry. The `state-transition-contracts.md` convention file (proposed), written language-neutral.
- [ ] M2: Go reference profile. The package-level map literal and directive marker, parsed with `go/parser`. Red-first tests over a fixture machine; unparseable shapes fail closed as PYLON053.
- [ ] M3: Php and rust reference profiles. Line-parseable canonical forms: the class-level const map and the match table. The first profiles beyond Go, sized for the first two adoption stacks.
- [ ] M4: Declared side. The `machines:` front-matter list, each entry carrying its id, states, triples, and `initial:`/`terminal:` sets, validated by the parser. No generator in v1.
- [ ] M5: Comparison and graph rules. Triple-set intersection in both directions, plus PYLON054 through PYLON058, each with a red-first test.
- [ ] M6: Advisory and proofs. PYLON059 via the import-boundary write scan; `verified_by:` resolution; `context` and `brief` surfacing.
- [ ] M7: Merge-door escalation wiring, ship-rule docs (USAGE, FEATURES, CHANGELOG, roadmap deletion), and the ship commit.
- [ ] M8: Generation. The frontmatter re-marshal with `--check` drift and the Mermaid render, after the first real machine exists. The remaining built-in profiles land behind the same interface, priority set by fixture families modeled on real stacks.

## Open questions

- Does the legacy vocabulary of an existing ledger demand declared-side key aliases? The default is a one-time content migration. Aliases are deferred until a real vocabulary demands them.
- One domain entity can declare several machines, via the `machines:` list. Real cases exist in both adoption trees. The list ships in v1, and no schema bump will ever be needed.
- Resolved: comparison-only in v1. Generation follows the first real machine (M7).
- Resolved for the Go reference profile: a package-level `var` literal with const identifier keys, marked by a `// pylon:machine <id>` directive. Other profiles pin their own canonical forms when they land.
