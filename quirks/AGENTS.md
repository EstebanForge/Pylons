---
title: "Quirks (Append-only Traps)"
type: reference
status: active
---

# Quirks

This folder is an append-only ledger of non-obvious traps, integration edge cases, and post-mortems.

## Rules

- Scaffold from `../templates/quirk.md` (`status: active`).
- Include `## Symptom`, `## Cause`, and `## Resolution` (`PYLON022`).
- Declare affected `contracts:` (`kind` and `id`, `PYLON036`) and `sources:` (`PYLON025`) so `pylon check` and `context` surface warnings.
- Set `guard:` (`lint-rule`, `convention`, `test`, `accepted-risk`) within 30 days (`PYLON028`).
