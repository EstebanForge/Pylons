---
title: "Pylons vocabulary: closed frontmatter value sets"
type: convention
slug: conv.pylons.vocabulary
level: MUST
status: accepted
created: 2026-11-19
updated: 2026-11-19
owners:
  - estebanforge
applies_to: ["all"]
supersedes: []
artifact: "cli/internal/rules/lint.go"
tags: [pylons, vocabulary, status, lifecycle, contract, conventions]
---

# Pylons vocabulary: closed frontmatter value sets

Several Pylons frontmatter fields accept values from a closed set. This document defines each allowed set.

## Status values

The `status:` field records where a document sits in its lifecycle. Allowed values differ by root folder.

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

Core built-in contract kinds work across any language:

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

A package `protected:` entry carries `glob`, `owner`, and `reason`. Allowed `owner:` values: `release-bot` (`FORCED`), `vendored` (`FORCED`), `generated` (`FORCED`), `human-only` (`FORCED`), `ask-first` (`ADVISORY`).

## Other closed value sets

These fields accept values only from the sets below. No lint rule validates them; the sets are conventions.

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
