---
title: "Adopting an Existing Ledger"
type: reference
status: active
created: 2026-11-21
updated: 2026-11-24
owners: [Pylons]
sources: []
description: "Bringing a ledger that predates Pylons under the engine, without losing your content."
tags: [adoption, migration, reference]
---

# Adopting an existing ledger

A ledger can predate Pylons: its own frontmatter conventions, its own URI schemes, its own hook tooling, and years of accepted debt. Pylons absorbs all of it through configuration. Nothing is renamed, nothing is rewritten, and the engine never edits your files.

## The order that works

1. Place the binary. `scripts/install.sh --bin-only .` drops `bin/pylon` and its license files into the existing directory and touches nothing else. The binary is engine-owned; a later run upgrades it in place.
2. Write the smallest `manifests/pylons.yaml` that names your reality. Register your code repositories under `projects:`. Declare your own link schemes under `schemes:`, so links the engine cannot resolve stay recognized instead of dark.
3. Decide who maintains the manifests. `generate: []` hands a manifest to foreign tooling; an absent `generate:` means the gates maintain both. A manifest the gates do not maintain never reports drift.
4. Point `exclude:` at anything the engine should not judge, and configure `decision_prefix:` if your accepted-decision files are not named `DNNNN-slug.md`.
5. Absorb the accepted debt once: `pylon lint --update-baseline`. The baseline is a readable ledger of what you accepted, with counts; anything not in it still fails. Re-run it after paid debt shrinks. Path-numbered entries stay distinct, so renumbering a file is new debt, not a free pass.
6. Wire the gates last. `pylon install --pylons .` writes the hook shims. Or chain the engine into your own hook template with one line: `bin/pylon gate pre-commit || exit 1`. The source repository's manual covers the wiring (docs/INSTALL.md, section 6).
7. Update with `pylon self-update --from <staged-release>` or `scripts/install.sh` again. Product files three-way against the release; your edits come back as `.pylons-new` copies; user files are never touched. Two truths about that first update. `pylon new` reads `templates/` from the ledger itself. A ledger created before templates shipped must copy `templates/` from the release first. And a ledger with no `manifests/product.json` yet reports every product file as drifted on the first `self-update`. That is the expected first-pass shape; resolve it with `--force` or by hand.

## What the knobs absorb

| Reality | Knob |
| --- | --- |
| Links in foreign URI schemes | `schemes:` |
| Directories outside the engine's judgment | `exclude:` |
| Manifests owned by other tooling | `generate:` |
| Accepted decisions named differently | `decision_prefix:` |
| Years of known debt | `manifests/baseline.json` |

## What configuration cannot absorb

- Legacy keys survive one advisory pass (PYLON064). A doc type the engine does not know falls back to the union of known keys. Only keys unknown everywhere are flagged. On a known type, an out-of-template key is flagged individually. Either extend the template or drop the key. The registry in `manifests/vocabulary.json` stays the closed list.
- Hook ownership is binary: either your hook template chains the engine, or the shipped shims own `.githooks/`. An external tool that regenerates hook files will overwrite the shims. The chain line in your own template is the durable path.
- Contract resolution needs registered projects. `repo://` surfaces that name unregistered aliases fail closed (PYLON003); register the projects or drop the pointers.

## Migrate a foreign package manifest

A predecessor tool can own `manifests/packages.json` with its own schema. Atlas is the worked example: a dict keyed by package name, with eleven fields per package. Pylon owns the same paths, so the two tools cannot share the directory. Retire the foreign tool and move the data. Every field has a destination:

| Foreign field | Pylon destination |
| --- | --- |
| package name | the `projects:` alias key in `manifests/pylons.yaml` |
| `path` | `projects[].path` |
| `tier` | `projects[].tier` (free-form) |
| `repo` | `projects[].repo` |
| `ref` | `projects[].ref`; carry every branch pin, a dropped pin changes resolution silently |
| `status` | package doc `status:`; the vocabulary is narrower, so map `production` to `active` |
| `depends_on` | package doc `depends_on:` |
| `type` | package docs stay `type: package`; put `wordpress-plugin` and kin in `tags:` |
| `composer_name`, `testable` | `tags:` entries |
| `clonable` | no destination; it governed the foreign tool's clone mechanics |

Four facts keep the migration honest:

- The package catalog is the docs, not the registry: a `depends_on:` name with no `packages/*.md` doc of `type: package` is an unknown package (PYLON013). Write one stub doc per third-party name.
- Contract kinds are closed: `file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`, plus enabled extensions. Remap foreign kinds before lint (`rest-route` to `route`, `template-glob` to `glob`).
- Quirk `sources:` name code and resolve against the ledger root or a project. Rewrite stack-relative paths to `repo://<alias>/<path>`.
- Absorb the prose debt once with `pylon lint --update-baseline`, then wire the gates. Advisory counts still escalate under `--ci`.

## The ratchet, not the switch

Adoption is a ratchet: absorb the debt, wire the cheapest gate, then tighten. The gates judge only what you declared, and the baseline shrinks as you pay. Nothing in this document requires a Pylons release cycle; every knob above ships in the current binary.
