---
title: "Installing and Setting Up Pylons"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-19
---

# Installing and Setting Up Pylons

Pylons has two parts: the knowledge-base directory (Markdown + `manifests/pylons.yaml`) and the `pylon` Go CLI binary.

## 1. Prerequisites

- Go 1.24+ (tested with Go 1.27) to build `pylon` from `cli/`.
- Git (optional, used for `--changed` staged-file checks and `@branch` validation).

## 2. Build the `pylon` CLI

Build the binary into `bin/pylon` or install it onto your `PATH`:

```bash
cd pylons/cli
go test ./...
go build -o ../bin/pylon ./cmd/pylon
```

To install globally on your `PATH`:

```bash
cd pylons/cli
go install ./cmd/pylon
```

## 3. Initialize a Pylons Workspace

If starting a fresh ledger directory, run:

```bash
pylon init /path/to/my-ledger
```

If cloning this repository directly, `manifests/pylons.yaml`, `templates/`, `conventions/vocabulary.md`, and `protocols/` are already in place.

## 4. Register Tracked Projects in `manifests/pylons.yaml`

Add each project or repository you want Pylons to track in `manifests/pylons.yaml`:

```yaml
version: 1
extensions:
  - go
projects:
  core-api:
    path: ../core-api
    ref: main
    tier: core
    extensions: [typescript]
```

- `path`: Local path to the project checkout (relative to the Pylons root or absolute).
- `ref`: Default base branch for the project (defaults to `main`).
- `extensions`: Language or custom extensions enabled globally or per project.

## 5. Wire Pre-Commit and CI Gates

The repo ships gate hooks in `.githooks/`. Enable them once per clone:

```bash
./bin/pylon install
```

The command is idempotent: it writes only missing hook scripts and reports what it kept. `pylon init` wires the same hooks automatically when it runs inside a git repository.

`pre-commit` regenerates the manifests and auto-stages drift, lints changed docs, and reviews staged proposals and quirks. `pre-push` blocks on stale manifests, full-corpus lint findings, and quirk source validation.

In CI, run full corpus validation with:

```bash
./bin/pylon lint --ci --format sarif
```
