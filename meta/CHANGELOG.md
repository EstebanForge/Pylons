---
title: "Pylons Changelog"
type: meta
status: active
created: 2026-01-26
updated: 2026-11-19
---

# Pylons Changelog

Notable changes to the Pylons knowledge-base format and `pylon` CLI.

## 2026-11-19: Initial Public Release

- Extracted and generalized the standalone Pylons living-map-ledger and `pylon` Go CLI (`cli/`).
- Added `pylons.yaml` multi-project registry with `repo://` and `pylon://` URI schemes.
- Added seven language-agnostic core contract kinds (`file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`) plus opt-in built-in extensions (`go`, `rust`, `typescript`, `python`, `php`) and external `pylon-ext-<name>` JSON plugin support.
- Added `PYLON001`–`PYLON038` deterministic lint and collision rules with `FORCED` / `ADVISORY` grades and `human`, `json`, `sarif`, and `--stop-block` output.
- Added `tickets/` zone, `archive/` exclusions across CLI context/search/manifest commands, and `protocols/` CLI-consumed templates with compiled-in defaults (`pylon protocol`, `pylon reflect`).
- Added `meta/` documentation.
- Licensed as Apache-2.0

## 2026-01-26: Origin

- Initial design of the living-map-ledger model, typed contracts, controlled language (ASD-STE100), and deterministic CLI verification loop.
