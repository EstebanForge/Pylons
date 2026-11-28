---
title: "Pylons Docs Index"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-27
---

# Pylons docs index

Documentation about Pylons and the `pylon` CLI. This folder is the source repository's manual; the reference pages that travel inside a ledger live in `pylons/pylons-docs/`.

- [FEATURES.md](FEATURES.md): feature guide: what each zone and command fixes, with the workflows and the exit codes; the command dictionary lives in USAGE.md.
- [INSTALL.md](INSTALL.md): how to install `pylon`, initialize a Pylons ledger, register projects, and wire git hooks or CI.
- [UPDATING.md](UPDATING.md): how to update an existing pylon to a new release: the piped installer, manual `self-update`, the three-way file merge, and drift review.
- [USAGE.md](USAGE.md): daily workflow, CLI command reference, contracts, and lifecycle rules.
- [EXTENSIONS.md](EXTENSIONS.md): how to enable built-in language extensions and author custom `pylon-ext-<name>` plugins.
- [SPINE.md](SPINE.md): design philosophy, zero-model determinism, and the closed agent loop.
- [CHANGELOG.md](CHANGELOG.md): release history and origin timeline.
- [benchmarks/](benchmarks/): dated latency receipts from `scripts/bench.sh`, the only accepted form of a performance claim.
- [../pylons/pylons-docs/pylons-yaml.md](../pylons/pylons-docs/pylons-yaml.md): schema and examples for `manifests/pylons.yaml`, the per-ledger configuration file (ships inside every ledger).
- [../pylons/pylons-docs/adopting.md](../pylons/pylons-docs/adopting.md): bringing a ledger that predates Pylons under the engine, knob by knob (ships inside every ledger).
- [../pylons/pylons-docs/parity-spec.md](../pylons/pylons-docs/parity-spec.md): the declarative parity spec: language profiles, machine declarations, and the machine discipline (ships inside every ledger).
