---
title: "The extension protocol and schema"
type: reference
status: active
created: 2026-11-20
updated: 2026-11-20
owners:
  - esteban
sources: []
description: "Formal schema, manifest, and wire examples for drop-in pylon-ext-<name> plugins."
tags: [reference, schema, extensions]
---

# The extension protocol and schema

## Overview

The formal JSON Schema (version 1) lives at `manifests/extensions.json`. It defines two documents: the optional plugin manifest, and the stdin/stdout wire protocol between `pylon` and a `pylon-ext-<name>` executable. Inspect it with `pylon extensions schema`; list installed extensions with `pylon extensions list`.

## Manifest example

A plugin may ship a manifest beside its executable. Fields other than the required three are optional.

```json
{
  "schema_version": 1,
  "name": "csharp",
  "description": "C# symbol resolution for the platform-api project.",
  "kinds": ["csharp-symbol"],
  "entrypoint": "pylon-ext-csharp"
}
```

Required: `schema_version` (always `1`), `name`, and `kinds`. `entrypoint` names the executable when it differs from `pylon-ext-<name>`.

## Wire protocol example

`pylon` writes one JSON line to stdin and reads one JSON line from stdout, with a 5-second timeout.

Request:

```json
{"schema_version": 1, "kind": "csharp-symbol", "id": "Services/OrderService.cs::OrderService", "root": "/path/to/platform-api"}
```

The optional `ref` field carries the project's configured git ref. Resolution against a ref reads the ref, not the working tree.

Successful response:

```json
{"resolved": true, "detail": "declared at Services/OrderService.cs:17"}
```

Unresolved response:

```json
{"resolved": false, "detail": "no symbol named OrderService"}
```

Failure response:

```json
{"resolved": false, "error": "file not found"}
```

## Key constraints

- A plugin serves only the contract kinds its name prefixes: the `csharp` plugin answers `csharp-*` kinds. Dash-less kinds never reach external plugins.
- Enable the plugin name in `manifests/pylons.yaml` under the global `extensions:` or `projects.<alias>.extensions:`. Unenabled kinds fail resolution, so projects stay explicit about what they depend on.
- Run `lint` or `contract` with `--allow-external-extensions` (or `PYLON_ALLOW_EXTERNAL_EXTENSIONS=1`) against trusted repositories only. Without the opt-in, external plugins never execute.
- `error` means the plugin could not run or judge the request; `detail` means a normal unresolved verdict.
- The schema is versioned. A plugin that speaks an unknown `schema_version` is rejected, not misread.
