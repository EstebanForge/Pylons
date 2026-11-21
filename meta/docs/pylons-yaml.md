---
title: "The pylons.yaml manifest"
type: reference
status: active
created: 2026-11-20
updated: 2026-11-20
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
| `projects.<alias>.tier` | no | Package tier hint: `development` or `production`. |
| `projects.<alias>.extensions` | no | Extension ids enabled for this project only. |

## Key constraints

- A missing file is not an error: an empty ledger still runs, with zero projects.
- An unparsable file produces a fail-closed finding. Commands that only inform print a warning and continue.
- One alias can register one project; alias names are stable identifiers, so renaming one breaks every `repo://` URI that names it.
- The extension system is configured here too. See [EXTENSIONS.md](../EXTENSIONS.md) for the language list and plugin installation.
- The `parity:` section configures the parity spec in this same manifest family. See [parity-spec.md](parity-spec.md) for the closed schema, and `manifests/parity.json` for its JSON schema.
