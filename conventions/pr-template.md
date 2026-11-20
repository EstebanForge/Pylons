---
title: "Pull request template: merge danger line"
type: convention
slug: conv.merge-danger-line
level: MUST
status: accepted
created: 2026-11-20
updated: 2026-11-20
owners:
  - estebanforge
ticket: []
applies_to: ["all"]
supersedes: []
artifact: ""
description: "Every pull request body carries one merge danger line; the merge door is classified before review starts."
tags: [conventions]
---

# Pull request template: merge danger line

## The rule

Every pull request body carries one required line, right after the How section. The line:

`Merge danger: <one-way|two-way> | blast radius: <scope in one phrase>`

The door vocabulary is closed. No other door tokens parse.

## Door vocabulary

- `one-way`: migrations, data backfills, irreversible external writes, bulk emails, contract changes shipped to production. Review hard.
- `two-way`: easy revert, localized blast radius. Skim.

## Consistency rules

- `one-way` must pair with a release marker in the diff or a `Data migration:` note in the body. A release marker is a breaking commit subject (`feat!:`, `feat(scope)!:`, `fix!:`) or a `#major` token in a commit subject. The `Data migration:` note must start a body line.
- A `schema` contract change in the diff forces `one-way` minimum.
- A derived `one-way` may never be downgraded to `two-way`. The author line may only escalate, never de-escalate.

## Enforcement

`pylon review merge-danger --body <file> [--base <ref>]` parses the body, validates the closed vocabulary, and derives the minimum door from the diff. A missing line, an unknown door token, or a derived `one-way` under an author `two-way` line is a blocking finding. CI runs it before merge.

## Why

The merge decision is where production breakage gets decided. One cheap line makes blast radius explicit, permanent, and greppable; squash-merge carries it into the durable record.
