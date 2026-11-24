---
title: "The pylons.yaml manifest"
type: reference
status: active
created: 2026-11-20
updated: 2026-11-23
owners:
  - Pylons
sources: []
description: "Schema and examples for manifests/pylons.yaml, the per-ledger configuration file."
tags: [reference, config, schema]
---

# The pylons.yaml manifest

## Overview

`manifests/pylons.yaml` is the one configuration file a ledger needs. It sits at the ledger root and is validated on every run: an unparsable file is reported as a finding, not silently ignored. The file maps the ledger's projects to their code, selects language extensions, and binds protocol names.

## Full example

```yaml
version: 1

# Global extension list: enabled for every project.
extensions:
  - go

# Protocol overrides: map a protocol name to its source document.
protocols:
  agent: agent.md

projects:
  # The key is the alias. It appears in every repo:// URI.
  platform-api:
    # Path to the checkout, relative to the ledger root.
    path: ../platform-api
    # Git ref used to read the code when the working tree is not authoritative.
    ref: main
    # Extra extensions enabled for this project only.
    extensions:
      - typescript

  rust-daemon:
    path: ../daemon
    ref: v0.4.2
    tier: production
```

## Field reference

| Field | Required | Meaning |
|---|---|---|
| `version` | yes | Config version. Only `1` exists today. |
| `extensions` | no | Language extension ids enabled for every project: `go`, `rust`, `typescript`, `python`, `php`, or any enabled external plugin name. |
| `protocols` | no | Map of protocol name to source document under `protocols/`. |
| `projects` | yes | Map of alias to project. The alias keys every `repo://<alias>/...` URI. |
| `projects.<alias>.path` | yes | Checkout path relative to the ledger root. |
| `projects.<alias>.repo` | no | Repository URL for documentation value. |
| `generate` | no | Manifests the gate hooks maintain: `rules`, `packages`. Absent means both; an exact list disables the rest. |
| `schemes` | no | Extra URI schemes the ledger recognizes; declared-scheme links are checked for well-formedness. |
| `exclude` | no | KB-relative globs never linted and never indexed. |
| `projects.<alias>.ref` | no | Git ref the checker reads when resolving the project's code. Unset means the working tree. |
| `projects.<alias>.tier` | no | Free-form tier hint, for example `core`, `app`, or `service`. |
| `decision_prefix` | no | One to four uppercase letters naming accepted-decision files `<prefix>NNNN-slug.md`. Absent means `D`. |
| `projects.<alias>.extensions` | no | Extension ids enabled for this project only. |

## Key constraints

- The engine locates the ledger by this file: a missing one makes every command exit 3. An existing file with zero projects is the empty ledger, and it runs.
- An unparsable file produces a fail-closed finding. Commands that only inform print a warning and continue.
- One alias can register one project. Alias names are stable identifiers. Renaming one breaks every `repo://` URI that names it.
- The extension system is configured here too. The source repository's manual (docs/EXTENSIONS.md) lists the language set and plugin installation.
- The `parity:` section configures the parity spec in this same manifest family. See [parity-spec.md](parity-spec.md) for the closed schema, and `manifests/parity.json` for its JSON schema.
## How this schema moves

Adding a key or a value is free and needs no version bump. Removing a key, renaming a key, or changing a value's meaning bumps `version` and lands in the changelog. No migration layer ships.

The per-type frontmatter key registry lives in `manifests/vocabulary.json` under `frontmatter_keys` and mirrors the engine. The full contract (what machines read, the closed prose exceptions, and strict frontmatter decoding) lives in `cli/docs/machine-contract.md` of the Pylons source repository.
