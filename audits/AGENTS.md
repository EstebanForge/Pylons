---
title: "Audits"
type: reference
status: active
---

# Audits

This folder holds point-in-time health, security, and compliance audits.

## Rules

- Scaffold from `../templates/audit.md`.
- Set `remediation:` (`open`, `in-progress`, `complete`, `rejected`) and `reaudit: YYYY-MM-DD` (`PYLON026`).
- Every findings table row must end with a valid status (`open`, `fixed`, `rejected`, `moved`; `PYLON018`).
- Move `superseded`, `withdrawn`, or `retired` audits to `archive/`.
