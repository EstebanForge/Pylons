---
title: "Pylons Changelog"
type: meta
status: active
created: 2026-01-26
updated: 2026-11-19
---

# Pylons Changelog

Notable changes to the Pylons format and `pylon` CLI.

## 2026-11-20

Hardening round. Project matching is deterministic for nested registered roots. Config errors surface instead of silently degrading output. All git subprocesses run under a 10-second budget. Symbol contracts match on word boundaries. Contracts whose components do not map to a registered project now fail instead of skipping. A configured git ref that does not resolve fails closed instead of falling back to the working tree. `pylon install` refuses a pylon below the repository root, detects another tool's `core.hooksPath` (override with `--force`), and reports hooks that drifted from the shipped assets. Gate hooks fail closed when no `pylon` binary is found; `PYLON_HOOKS_OPTIONAL=1` restores the old soft skip. `staleness --fix` patches frontmatter only. SARIF is rejected on commands that cannot render it; `pylon brief` without a pylon root exits 3. A one-line installer (`install.sh`) downloads a release into a project as a fresh pylon with its own git repository, verifies checksums, and wires the gate hooks; releases carry optimized per-platform binaries (`-s -w`, `CGO_ENABLED=0`, `-trimpath`) plus the skeleton and checksums, and `pylon --version` reports the release. The installed unit is now called a pylon: a plain-text map of the platform with a deterministic checker, replacing the working term knowledge base.

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
