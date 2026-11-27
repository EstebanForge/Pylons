---
title: "Installing and Setting Up Pylons"
type: meta
status: active
created: 2026-11-19
updated: 2026-11-27
---

# Installing and Setting Up Pylons

Pylons has two parts: the pylon itself (a directory of Markdown, `manifests/pylons.yaml`, and gate hooks) and the `pylon` Go CLI binary.

## 1. Prerequisites

- One of: the one-line installer (needs `curl` or `wget`, plus `git`), Go 1.27+ to build `pylon` from `cli/`, or a release tarball from the [GitHub releases](https://github.com/EstebanForge/Pylons/releases).
- Git (optional, used for `--changed` staged-file checks and `@branch` validation).

## 2. Install the `pylon` CLI and a Pylon

The installer places a ready-to-use pylon in `./pylons`: the `pylon` binary in `bin/`, the skeleton, a fresh git repository, and wired gate hooks. It never writes outside that directory.

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash
```

Variants:

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash -s -- my-kb          # custom directory
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | PYLONS_VERSION=0.2.0 bash  # pinned version
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash -s -- --bin-only .    # binary only, into an existing directory
```

The installer needs `bash`, `curl` or `wget`, and a sha256 tool. Minimal containers without `bash` cannot run it; download the release tarballs manually there.

Released binaries are cross-built per platform (linux and macOS in amd64 and arm64; Windows amd64 only) with symbols and DWARF tables stripped (`-ldflags "-s -w"`, `CGO_ENABLED=0`, `-trimpath`). Every release carries `checksums.txt`; the installer verifies it before unpacking.

To install manually instead, download `pylons-X.Y.Z-<os>-<arch>.tar.gz` and `pylons-kb-X.Y.Z.tar.gz` from a release, unpack both into one directory, run `git init`, then `./bin/pylon install --pylons .`.

## 3. Build the `pylon` CLI from Source

Build the binary into `pylons/bin/pylon` or install it onto your `PATH`:

```bash
cd cli
go test ./...
go build -o ../pylons/bin/pylon ./cmd/pylon
```

To install globally on your `PATH`:

```bash
cd cli
go install ./cmd/pylon
```

## 4. Initialize a Pylons Workspace

If starting a fresh pylon, run:

```bash
pylon init /path/to/my-ledger
```

If cloning this repository directly, `manifests/pylons.yaml`, `templates/`, `manifests/vocabulary.json`, and `protocols/` are already in place. Section 2's installer produces the same shape without the clone.

## 5. Register Tracked Projects in `manifests/pylons.yaml`

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

## 6. Wire Pre-Commit and CI Gates

The repo ships gate hooks in `.githooks/`. Enable them once per clone:

```bash
./bin/pylon install
```

The command is idempotent: it writes only missing hook scripts and reports what it kept. `pylon init` wires the same hooks automatically when it runs inside a git repository.

The shipped hooks are thin shims: all gate logic lives in the compiled engine, so the shim files rarely change and `self-update` never needs to touch them. When `bin/pylon` is absent the shim falls back to whatever `pylon` is on your `PATH`, and that fallback has no version check: a stale PATH copy can run old gate logic, so keep `bin/pylon` present in the ledger you gate.

If another tool owns your hooks, `pylon install` refuses instead of silently leaving you ungated. Two ways out:

1. Chain the engine from your own hook template. Add one line to it: `bin/pylon gate pre-commit || exit 1` (and `bin/pylon gate pre-push || exit 1` for push). The engine entry is stable across releases.
2. Take over with `pylon install --force`. Your previous hook files are kept beside the new ones as `.bak` copies.

In either case, add `pylon lint --ci` and `pylon review gate-surface` to CI as the backstop.

`pre-commit` regenerates the manifests and auto-stages drift, lints changed docs, and reviews staged proposals and quirks. `pre-push` blocks on stale manifests, full-corpus lint findings, and quirk source validation.

In CI, run full corpus validation with:

```bash
./bin/pylon lint --ci --format sarif
```

To bring an existing pylon to a new release later, see [UPDATING.md](UPDATING.md).
