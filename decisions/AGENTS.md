---
title: "Decisions (ADRs)"
type: reference
status: active
---

# Decisions

This folder holds Architecture Decision Records.

## Rules

- Scaffold proposals in `proposals/` from `../templates/decision.md` (`status: proposed`).
- On acceptance, assign the next sequential number (`DNNNN-<slug>.md`, `PYLON010`) and move to this folder root with `status: accepted`.
- Accepted decisions are immutable. To reverse or change direction, write a new decision with `supersedes:` and move the old one to `archive/` (`superseded` or `deprecated`).
