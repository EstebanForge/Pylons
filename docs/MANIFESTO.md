---
title: "The Pylons Manifesto"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-25
---

# The Pylons manifesto

Documentation you cannot verify is a rumor with formatting. Pylons gives multi-repo platforms and single-project teams a shared memory backed by deterministic proof.

## Core principles

1. Code is truth: source code is the source of truth for implementation. When code and docs disagree, the checker fails until they match.
2. Zero model calls in verification: `pylon` uses schema checks, pattern matching, git inspection, and set intersections. Same input, same verdict, offline and in CI.
3. Two grades only: `FORCED` (exit 1) blocks broken contracts, active collisions, and protected paths. `ADVISORY` (exit 0) informs without crying wolf.
4. Controlled language: ASD-STE100 keeps every doc short, active-voice, and unambiguous for both humans and AI agents.
5. Closed agent loop: orient (`brief`, `map`), check collisions (`check`, `context`), write back (`quirks/`, `plans/`, `decisions/`), and gate (`lint --ci`).
6. Excluded archives: retired, shipped, and superseded records move to `<zone>/archive/` during reflection passes (`pylon reflect`) and stay out of active context, search, and collision checks unless `--all` is requested.
7. Computed, never stored: a fact about other documents is computed at check time and never written to a file. Stored cross-facts need cascade writes and go stale; one walk computes them and stays true. No frontmatter key will ever record a `blocked` or `ready` state.
