---
title: "The machine contract"
type: reference
status: active
created: 2026-11-21
updated: 2026-11-21
owners:
  - Pylons
sources: []
description: "What the engine reads, what it judges, and how the schemas may move."
tags: [reference, contract, schema]
---

# The machine contract

## What machines read

The engine decides only on structured keys: frontmatter, manifests, and its own generated JSON. The engine never judges prose. Prose exists for humans and for agents that read documents as documents.

## The closed exceptions

The engine judges prose only through a closed list. No new exceptions without a rule id and a schema note.

1. ASD-STE100 sentence limits (PYLON006).
2. Required section headings (PYLON021, PYLON022, PYLON031).
3. Term links and the ubiquitous language (PYLON005, PYLON039).
4. Raw paths in quirks (PYLON033).
5. The pull request body lines `Merge danger:` and `Data migration:` (PYLON047).
6. The line marker `pylons-allow-secret` exempts exactly its own line from the secret rules (PYLON060, PYLON061). Use it on quoted provider example keys, never on real credentials.

## Strict frontmatter

Frontmatter decodes strictly. Anchors, aliases, merge keys, and custom tags fail closed as PYLON001, with the file line named. Line numbers in those findings are file-relative: the opening fence is file line 1. Unknown keys inside the closed payloads (machines, transitions, contract entries, scenarios) fail closed the same way.

An unknown top-level key stays advisory (PYLON064). The key set per doc type lives in manifests/vocabulary.json under `frontmatter_keys` and mirrors the engine through a test. Adding a key to a template adds it there.

## How schemas move

Adding a key or a value is free and needs no version bump. Removing a key, renaming a key, or changing a value's meaning bumps `version` in the touched manifest and lands in the changelog. No migration layer ships.

Generated JSON is different: rules.json, packages.json, and product.json are rewritten whole from the engine on every generation, so they never preserve unknown keys. Their `version` field is the contract.

## Where each format lives

1. Human-authored typed data: YAML frontmatter, validated strictly.
2. Human prose: Markdown, judged only through the closed exceptions.
3. Machine-written data: JSON or JSONL, written by the engine and schema-checked before it is trusted.
