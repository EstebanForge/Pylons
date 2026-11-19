---
title: "Tickets"
type: reference
status: active
---

# Tickets

This folder tracks local work items: bugs, tasks, improvements, and chores.

## Rules

- Scaffold from `../templates/ticket.md`.
- Allowed `status:` values: `open`, `in-progress`, `blocked`, `done`, `wontfix`.
- Allowed `kind:` values: `bug`, `task`, `improvement`, `chore`.
- Keep active tickets (`open`, `in-progress`, `blocked`) in this root folder.
- Move completed or closed tickets (`done`, `wontfix`) to `archive/`.
- Reference ticket paths or IDs from `plans/` `ticket:` frontmatter when a plan implements a ticket.
