---
title: "Investigations"
type: reference
status: active
---

# Investigations

This folder holds root-cause analyses and reproduction evidence for complex bugs.

## Rules

- Scaffold from `../templates/investigation.md` and name files `YYYY-MM-DD-<slug>.md` (`PYLON030`).
- Allowed `status:` values: `active`, `resolved`, `abandoned`.
- Closed investigations (`resolved` or `abandoned`) must include `## Root Cause` and `## Resolution` (`PYLON031`) and move to `archive/`.
