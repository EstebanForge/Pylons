---
title: "Plans"
type: reference
status: active
---

# Plans

This folder holds active feature, refactor, chore, docs, and perf plans (`status: accepted` or `in-progress`).

## Rules

- Scaffold from `../templates/plan.md` and name files `<type>-<slug>.md` (`PYLON038`).
- `accepted` plans require `ticket:` (`PYLON016`) and `wrong_if:` on every `contract.stable` entry (`PYLON014`).
- `in-progress` plans keep a live branch or `artifact:` trace (`PYLON027`).
- When shipped or abandoned, set `status: shipped` (with `artifact:`) or `abandoned` and move to `archive/`.
- Never place bug fixes here; route complex bugs to `../investigations/`.
