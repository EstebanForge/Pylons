---
title: "The closed frontmatter vocabulary"
type: reference
status: active
created: 2026-11-21
updated: 2026-11-21
owners:
  - esteban
sources: []
description: "The closed value sets Pylons frontmatter fields accept, and the machine-readable registry at manifests/vocabulary.json."
tags: [reference, schema, vocabulary]
---

# The closed frontmatter vocabulary

Several Pylons frontmatter fields accept values from a closed set. The machine-readable registry lives at `manifests/vocabulary.json`; this page explains it. Status values are enforced by the lint engine (`PYLON008`); the other sets are conventions, with `PYLON041` validating the post-mortem tokens.

## Status values

The `status:` field records where a document sits in its lifecycle. Allowed values differ by root folder; the JSON `status` map carries the sets, keyed by folder.

| Root | Allowed `status:` values |
|---|---|
| `proposals/` | `proposed` |
| `plans/` | `proposed`, `accepted`, `in-progress`, `shipped`, `abandoned` |
| `conventions/` | `proposed`, `accepted`, `enforced`, `superseded` |
| `decisions/` | `proposed`, `accepted`, `superseded`, `deprecated` |
| `audits/` | `active`, `superseded`, `withdrawn`, `retired` |
| `investigations/` | `active`, `resolved`, `abandoned` |
| `packages/` | `active`, `deprecated`, `retired` |
| `references/` | `active`, `superseded`, `retired` |
| `flows/` | `active` |
| `quirks/` | `active` |
| `domain/entities/` | `proposed`, `accepted`, `changed` |
| `tickets/` | `open`, `in-progress`, `blocked`, `done`, `wontfix` |

## Contract entry kinds

Core built-in contract kinds work across any language; the JSON `contract_kinds` map carries each kind's `id:` grammar.

| `kind:` | `id:` grammar |
|---|---|
| `file` | `repo://<alias>/path/to/file` or `path/to/file` |
| `glob` | `repo://<alias>/path/**/*.ext` or `path/**/*.ext` |
| `symbol` | `path/to/file::SymbolName` |
| `route` | `METHOD /path/pattern` |
| `cli` | `command subcommand` |
| `event` | `event.name` |
| `schema` | `ModelOrTableName` |

Drop-in extensions enabled in `pylons.yaml` may register additional kinds (for example `go-symbol`, `ts-export`, `rust-item`, `py-symbol`, `php-symbol`).

## Protected path owners

A package `protected:` entry carries `glob`, `owner`, and `reason`. The JSON `protected_owners` map carries the closed owner set and each owner's grade: `release-bot`, `vendored`, `generated`, and `human-only` are `FORCED`; `ask-first` is `ADVISORY`.

## Other closed value sets

These fields accept values only from the sets in the JSON `closed_sets` map. No lint rule validates them beyond `PYLON041`; the sets are conventions.

| Field | Documents | Allowed values |
|---|---|---|
| `level:` | conventions | `MUST`, `SHOULD` |
| `ai_authored_contributed:` | quirks | `yes`, `no`, `unknown` (validated by `PYLON041`) |
| `volume_pressure:` | quirks | `review`, `test`, `rollout`, `observability`, `none` (validated by `PYLON041`) |
| `remediation:` | audits | `open`, `in-progress`, `complete`, `rejected` |
| `severity:` | investigations, audits | `critical`, `high`, `medium`, `low` |
| `reproducible:` | investigations | `always`, `intermittent`, `environmental` |
| `rollout:` | packages | `development`, `production` |
| `guard:` | quirks | `lint-rule`, `convention`, `test`, `accepted-risk` |

Free-form fields (`description:`, `artifact:`, `scope:`, `criteria:`, `ticket:`, `superseded_by:`, `source_of_truth:`, `relationships:`, `guardrail:`) carry prose, links, or lists and accept any content.
