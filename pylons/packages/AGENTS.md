---
title: "Packages: The Architecture Map"
type: reference
status: active
---

# Packages

This folder holds one map file per tracked project or module (`<package-slug>.md`).

## Rules

- Scaffold from `../templates/package.md`.
- Register each tracked repo alias and local path in `../manifests/pylons.yaml`.
- Declare `exposes:` contracts using core kinds (`file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`) or enabled extension kinds.
- Declare `protected:` globs (`glob`, `owner`, `reason`) for generated, vendored, or human-only paths.
- Move retired packages to `archive/` with `status: retired`.
