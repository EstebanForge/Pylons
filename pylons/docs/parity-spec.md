---
title: "The parity spec"
type: reference
status: active
created: 2026-11-20
updated: 2026-11-24
owners:
  - Pylons
sources: []
description: "Schema and examples for the declarative parity spec: language profiles, machine declarations, and transition triples."
tags: [reference, schema, parity]
---

# The parity spec

## Overview

Status: active. The parity spec is the declarative configuration for `lint --parity` profile work. The `parity:` section schema and the machine machinery ship (rules `PYLON053` through `PYLON059`); everything below describes shipped behavior unless marked planned.

## Shape of the configuration

The CLI provides the plumbing and reads the spec; users configure the rest. Two declarative surfaces work together:

1. A parity spec section in the manifest family (like `extensions:` today), binding each language to a parity profile.
2. Machine declarations in domain-entity frontmatter, naming the code targets and the declared triples.

## Example: profile binding

```yaml
version: 1
extensions:
  - go
parity:
  version: 1
  profiles:
    # Built-in binding: keyed by language id.
    go:
      use: builtin:go
    # Rebind a built-in language to your own plugin. Rebinding kotlin
    # needs --allow-external-extensions at check time.
    kotlin:
      use: external:kotlin
      options:
        marker: "pylon:machine"
    # Override: rebind a built-in language to your own plugin.
    php:
      use: external:wp-php
```

`use: builtin:<language>` selects an in-process profile shipped with the CLI (go, rust, typescript, javascript, python, php, csharp, ruby, c, cpp, java, kotlin, dart, and sql are the built-in set). `use: external:<name>` selects the enabled extension `<name>`, run as the `pylon-ext-<name>` binary; it needs `--allow-external-extensions` at check time. Rebinding a language overrides its built-in with no implicit precedence. Two profiles claiming the same file extension is a config error; one profile re-listing an extension its language already owns is not. A declared machine with no matching binding yields the PARITY_NOT_EVALUATED advisory; an `on_unevaluated:` knob (default advisory) can make it block. With no `parity:` section at all, an enabled builtin extension acts as an implicit builtin binding.

## Example: machine declaration

On `domain/shipment.md`, one entity carrying two machines (a lifecycle and a returns process), because real entities often hold several:

```yaml
machines:
  - machine: shipment
    states:
      - pending
      - picked
      - shipped
      - delivered
      - cancelled
    states_symbol: repo://platform-api/Domain/State.cs::ShipmentStatus
    transitions_symbol: repo://platform-api/Transitions.cs::shipment
    on_unevaluated: advisory
    initial:
      - pending
    terminal:
      - delivered
      - cancelled
    transitions:
      - {from: pending, event: pick, to: picked}
      - {from: picked, event: ship, to: shipped}
      - {from: shipped, event: deliver, to: delivered}
  - machine: returns
    states:
      - requested
      - received
      - refunded
    transitions_symbol: repo://platform-api/Transitions.cs::returns
    initial:
      - requested
    terminal:
      - refunded
    transitions:
      - {from: requested, event: receive, to: received}
      - {from: received, event: refund, to: refunded}
```

The profile extracts the enum case set and the transition triples from the named code targets, and the checker compares the declared sets against the extracted ones in both directions. Each `machine:` value is an identity, not a pointer: the code-side marker must carry the same id (`// pylon:machine shipment` above the map). Machine ids are unique across the whole repo.


A runnable external plugin ships at `extensions/_examples/pylon-ext-parity`, and `_examples/ledger/` is a miniature ledger with one declared machine, an accepted plan, and a quirk.

## Working-tree semantics

Parity extraction reads the working tree on both sides: the declared frontmatter is KB content in the tree, and the code extraction reads the checked-out source. This is deliberate symmetry. Contract resolution honors a project's `ref:`, so a `ref:`-pinned ledger compares contracts against the pin and machines against the checkout. Run parity gates where the working tree is the truth, and treat `ref:` pinning as the contract-layer complement.

## Machine discipline

The convention a user's ledger declares alongside machine declarations: one closed state set per lifecycle, in a backed enum in the target language; one backed event enum beside it; one pure transition map. Side effects live outside transitions; lint cannot prove purity, so that rule is convention prose plus a table test in the mapped repo's QA suite (`verified_by:` names it). An invalid transition throws; the table test proves it, not lint. No parallel boolean flags (`isLoading` beside `isSuccess`); a review-lens rule, not an engine rule. Persisted transitions use a compare-and-swap on the stored state, because background workers may run concurrently.

## Key constraints

- Every declaration is explicit. There is no enum auto-discovery; discovery by proximity is a heuristic and violates the determinism constraint.
- The declared format is structured frontmatter. The engine never parses a generated markdown table as source of truth.
- A profile that cannot fully resolve a declared machine errors and fails closed. A skipped machine must never read as parity.
- `initial:` and `terminal:` are mandatory fields on every machine entry. Machine ids are unique across the repo.
- Configuration lives in the user's ledger. This ledger registers no live project and carries no machine declarations.
- The `states:`/`states_symbol:` set checks and the `machines:` transition checks both run under `lint --parity`. Transition extraction ships for go, php, rust, and typescript built-in profiles; other languages need an external plugin binding. The typescript canonical shape is the nested switch reducer: the outer `switch` reads the state, inner switches read the event, and every transition is a string-literal return under a `// pylon:machine <id>` directive. State-level or non-literal returns are absorbs, not triples; default arms and throw guards are ignored; single-line case bodies, casts (`as State`, `satisfies`), member access (`State.A`), `as const`, a `case` opening a block, depth beyond two switch levels, escaped quotes kept verbatim, and any other reducer logic fail closed. See [EXTENSIONS.md](../EXTENSIONS.md) for what is enabled today.
- States set-check extraction ships in-process for go, rust, php, typescript, c, cpp, java, kotlin, dart, and sql. The canonical shape is one member per line inside a named enum. C enumerators and C++ `enum class` take an optional backing type. Java constants take optional constructor arguments and end at the constant-list semicolon. Dart takes plain enums and enhanced enums whose members carry no constructor arguments. Postgres takes `CREATE TYPE ... AS ENUM` with comma-separated quoted values; `ALTER TYPE ... ADD VALUE` folds in. Fail closed: java and dart members with bodies, kotlin entries with bodies or nested call arguments or the member-list semicolon form, C anonymous typedef enums, an empty member parse, a bare sql name answered by two schema-qualified declarations, an sql value list with a missing comma or no values, and MySQL column `ENUM(...)` (a column option, not a named set). A braceless reference or a function signature or parameter list mentioning the type is skipped. Kotlin primary-constructor enums parse. Other `ALTER TYPE` verbs are not extracted. javascript, python, csharp, and ruby carry built-in bindings but no in-process states extractor yet. On the implicit extensions path, cpp claims the shared `.h` header over c when a project enables both; a project enabling only c keeps `.h`. An explicit parity spec claiming `.h` from both is a config error, like `.js` for typescript and javascript.
