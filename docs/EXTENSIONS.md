---
title: "Drop-In Extensions and Plugins"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-19
---

# Drop-in extensions and plugins

Pylons ships 7 language-agnostic core contract kinds (`file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`) that require 0 language parsers. When projects need stricter declaration checks, enable built-in language extensions or drop in custom executables.

The formal JSON Schema (version 1) for extension manifests and the stdin/stdout protocol lives at [manifests/extensions.json](../manifests/extensions.json). Inspect it anytime with `pylon extensions schema` and list available extensions with `pylon extensions list`.

## Enabling extensions in `manifests/pylons.yaml`

Enable extensions globally at the top level or per project under `projects.<alias>.extensions`:

```yaml
version: 1
extensions:
  - go
projects:
  web-app:
    path: ../web-app
    ref: main
    extensions:
      - typescript
```

If an extension is not enabled for a project, its contract kinds fail resolution so projects stay explicit about what they depend on.

## Built-in language extensions

| Extension | Contract kind(s) | Matches |
|---|---|---|
| `go` | `go-symbol` | `func`, `type`, `const`, `var` declarations in `<file>::<Symbol>` |
| `rust` | `rust-item` | `fn`, `struct`, `enum`, `trait`, `type`, `const`, `static`, `mod` items |
| `typescript` | `ts-export`, `js-export` | `export` declarations (`function`, `const`, `let`, `var`, `class`, `interface`, `type`, `enum`) |
| `python` | `py-symbol` | Top-level `def` / `async def` / `class` definitions |
| `php` | `php-symbol` | `function`, `class`, `interface`, `trait`, `enum` declarations |

## Writing external drop-in plugins (`pylon-ext-<name>`)

To support any custom language, schema format, or framework without recompiling `pylon`:

1. Create an executable named `pylon-ext-<name>` and place it in `<pylons-root>/extensions/` or anywhere on `PATH`. Reference plugins live in `extensions/_examples/`; copy one out of there. The CLI ignores every directory whose name starts with `_`, so `_examples/` content is never dispatched, linted, or indexed anywhere in the ledger.
2. Enable `<name>` in `manifests/pylons.yaml` under `extensions:`.
3. Use contract kinds prefixed with `<name>-` (for example `<name>-symbol` or `<name>-route`).
4. Pass `--allow-external-extensions` (or set `PYLON_ALLOW_EXTERNAL_EXTENSIONS=1`) when running `lint` or `contract` against trusted repositories.

### JSON stdin/stdout v1 protocol

`pylon` invokes `pylon-ext-<name>` with a 5-second timeout, writing 1 JSON line to `stdin`:

```json
{"schema_version": 1, "kind": "proto-message", "id": "auth.proto::AuthRequest", "root": "/path/to/project"}
```

The executable writes 1 JSON line to `stdout`:

```json
{"resolved": true, "detail": "proto/auth.proto"}
```

If resolution fails due to a runtime error, return `{"resolved": false, "detail": "reason", "error": "diagnostic"}`.

## Parity plugins

Hook-name contracts ship natively: with the `php` extension enabled, the `wp-hook` kind resolves the id's hook tag against a project's PHP files through the standard hook functions, no plugin needed. The family covers registration, firing, removal, queries, the ref-array and deprecated variants, and the remove_all pair. Variable and concatenated tags are out of scope: a contract names a literal tag. Matching ignores comments, so a tag named only in prose counts as wired, and PHP case folding applies to hook names. One pattern serves both engines, and it is line-shaped, so a hook call wrapped across lines is out of scope on every path: the verdict is the same unresolved answer whether a ref is configured or not.

The parity system uses the same `pylon-ext-<name>` plugins for two extra request types beyond contract resolution. With a `use: external:<name>` profile binding in the `parity:` section of `manifests/pylons.yaml`, pylon sends the plugin a request with `"op"` instead of `"id"`:

- `"op": "states"` asks for the case list of one enum: the plugin answers `{"cases": [{"name": "...", "value": "..."}]}` or, when it cannot resolve the type, `{"error": "diagnostic"}`.
- `"op": "transitions"` asks for the transition triples of one machine: the plugin answers `{"triples": [{"from": "...", "event": "...", "to": "..."}]}` or fails with an error line.

The engine reads the marked source file itself and sends its bounded text with the request, so the plugin stays a pure function. Transport failures grade as the not-evaluated advisory; a plugin-reported resolution failure grades as a hard finding. See [../pylons/pylons-docs/parity-spec.md](../pylons/pylons-docs/parity-spec.md) for the profile bindings. A runnable example plugin ships at `pylons/extensions/_examples/pylon-ext-parity`; the miniature ledger under `examples/ledger/` shows the declared formats end to end.
