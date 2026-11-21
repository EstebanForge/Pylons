---
title: "The parity spec"
type: reference
status: proposed
created: 2026-11-20
updated: 2026-11-20
owners:
  - esteban
sources: []
description: "Schema and examples for the planned declarative parity spec: language profiles, machine declarations, and transition triples."
tags: [reference, schema, parity, proposed]
---

# The parity spec

## Overview

Status: proposed. The parity spec is the declarative configuration for `lint --parity` extension work: the transition-graph rules do not exist yet, and this document pins the planned schema before any code lands. The plan of record lives at `repo://pylons/plans/proposals/feature-transition-parity.md`. Everything below describes intent, not shipped behavior.

## Shape of the configuration

The CLI provides the plumbing and reads the spec; users configure the rest. Two declarative surfaces work together:

1. A parity spec section in the manifest family (like `extensions:` today), binding each language to a parity profile.
2. Machine declarations in domain-entity frontmatter, naming the code targets and the declared triples.

## Example: profile binding (planned)

```yaml
version: 1
extensions:
  - go
parity:
  profiles:
    # Built-in profiles: keyed by language id.
    go:
      source: builtin
    # External profiles: keyed by the plugin name, enabled like extensions.
    csharp:
      source: pylon-ext-csharp
```

`source: builtin` selects an in-process profile shipped with the CLI (go, rust, typescript, javascript, python, php, csharp, ruby are the planned built-in set). Any other `source:` names a drop-in plugin executable. A language with no binding yields the fail-closed advisory; it never silently passes.

## Example: machine declaration (planned)

On `domain/shipment.md`:

```yaml
machine: shipment
states:
  - pending
  - picked
  - shipped
  - delivered
  - cancelled
states_symbol: repo://platform-api/Domain/State.cs::ShipmentStatus
transitions_symbol: repo://platform-api/Transitions.cs::shipment
initial:
  - pending
terminal:
  - delivered
  - cancelled
transitions:
  - {from: pending, event: pick, to: picked}
  - {from: picked, event: ship, to: shipped}
  - {from: shipped, event: deliver, to: delivered}
```

The profile extracts the enum case set and the transition triples from the named code targets, and the checker compares the declared sets against the extracted ones in both directions.

## Key constraints

- Every declaration is explicit. There is no enum auto-discovery; discovery by proximity is a heuristic and violates the determinism constraint.
- The declared format is structured frontmatter. The engine never parses a generated markdown table as source of truth.
- A profile that cannot fully resolve a declared machine errors and fails closed. A skipped machine must never read as parity.
- `initial:` and `terminal:` are mandatory fields on every machine declaration.
- Configuration lives in the user's ledger. This ledger registers no live project and carries no machine declarations.
- Until the spec ships, the shipped `states:` and `states_symbol:` set checks under `lint --parity` are the only parity surface. See [EXTENSIONS.md](../EXTENSIONS.md) for what is enabled today.
