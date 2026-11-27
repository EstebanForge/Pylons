# What Pylons is

A Pylons is a plain-text ledger and map of a platform, and an enforcer of the rules the team lays down, checked against the code. Three jobs in one artifact:

- **The ledger.** Decisions, quirks, plans, and conventions live as Markdown files with structured frontmatter, versioned in git beside the code they describe. What was decided, what nearly broke, what is provisional.
- **The map.** Package docs record what each project does, what depends on what, and which interfaces are safe to build on, with typed contracts (`kind` + `id`) the engine resolves against real code.
- **The enforcer.** The `pylon` binary re-checks the claims on every commit. A symbol, route, or machine transition the map declares must exist in code, or the gate fails. The map cannot quietly rot into fiction.

The engine is deterministic: zero model calls, closed vocabularies, stable rule IDs (`PYLON0xx`), and two grades only. `FORCED` stops work; `ADVISORY` informs. That contract is the product's spine; see `SPINE.md`.

## Pylons the product

The installable copy of Pylons, what users will see and use, lives at: `pylons/`

## Pylons CLI

- Go module `github.com/estebanforge/pylons/cli` (Go 1.27+, single dep `gopkg.in/yaml.v3`). No cobra: hand-rolled dispatch in `cli/cmd/pylon/main.go`.
- Source guide with per-package layout: `cli/AGENTS.md` (read before editing the engine). Agent loop for ledger work: `protocols/agent.md`.
- Build: `cd cli && go build -o ../pylons/bin/pylon ./cmd/pylon` (no `go.mod` at repo root — never `go build` from the root).
- Test all: build first (`cd cli && go build -o ../pylons/bin/pylon ./cmd/pylon`), then `cd cli && go test ./...`; the `cmd/pylon` tests exec the built binary and a fresh clone fails without it. Single: `cd cli && go test ./internal/resolve -run Name`. Vet: `cd cli && go vet ./...`.
- Format: `gofmt -l cli` (release gate; run `gofmt -w` on touched files).
- Exit codes: `0` clean/advisory, `1` FORCED finding, `2` usage/setup error, `3` no ledger root.
- Lint the product's own ledger: `./pylons/bin/pylon lint --pylons pylons`; gates: `pylon gate pre-commit|pre-push`; refresh accepted debt with `pylon lint --update-baseline --pylons pylons`.

## Product project structure

- `cli/` — the engine (this is the product code): `cmd/pylon/` entry + dispatch; `internal/config/` pylons.yaml loader; `internal/resolve/` frontmatter, URI + contract resolvers, BM25 search, language/external extensions; `internal/rules/` PYLON catalog, lint passes, check/contract/staleness/review, package-manifest builder; `internal/report/` human/json/sarif renderers; `internal/install/` gate hooks + installer; `internal/protocols/` protocol loader/renderer; `internal/aroma/` + `internal/sync/` retrieval/refresh; `internal/version/` pins `Version` (the release tag must match it).
- `cli/docs/` — engine-facing reference (machine contract, vocabulary, extension protocol). Not shipped; end-user equivalents live in `pylons/pylons-docs/`.
- `pylons/` — the installable ledger payload: zones (`packages/ decisions/ quirks/ conventions/ plans/ tickets/ investigations/ proposals/ domain/ references/ prompts/ skills/ templates/ protocols/ extensions/`) plus `manifests/` and `pylons-docs/`. Releases archive this folder verbatim.
- `pylons/manifests/` — `rules.json`, `packages.json`, `parity.json`, `ratchet.json`, `vocabulary.json`, `extensions.json`, `baseline.json`, `pylons.yaml` (+ `.example.*` twins). Generated files: never hand-edit.
- `docs/` — source-repo manual: `USAGE.md` (canonical command dictionary, single source), `FEATURES.md`, `INSTALL.md`, `CHANGELOG.md`, `ROADMAP.md`, `SPINE.md`, `EXTENSIONS.md`, `INDEX.md`.
- `scripts/` — `install.sh` (bootstrap a ledger; `--bin-only .` drops the binary into an existing dir), `release.sh` (the CI: gates, cross-build, pack, checksum, publish).
- `dist/` — release artifacts. `examples/ledger/` — sample ledger. No CI workflows, no Makefile/Justfile: `scripts/release.sh` is the pipeline.

## Working on Pylons the product

- Determinism is the spine: no model calls, no network in the engine, closed vocabularies, stable `PYLON0xx` IDs. A change that breaks reproducibility is a design defect, not a trade-off.
- Test-first: `go test ./...` before and after every engine change. Bugs reproduce fail-first; new rules ship with tests.
- After editing rules in `internal/rules/catalog.go`, regenerate: `../pylons/bin/pylon manifest rules --pylons ../pylons`. Same for `manifest packages` after package-doc edits. Verify regeneration is byte-stable (it must be).
- Shipped work: add a `docs/CHANGELOG.md` entry in the same change; delete the item from `docs/ROADMAP.md` when it ships (never mark in place). `USAGE.md` is the command dictionary — a new or changed command updates it, and `FEATURES.md` points there, never duplicates it.
- Ledger docs are dogfooded: they pass this repo's own lint. Write ASD-STE100 (short sentences), keep frontmatter typed per `manifests/vocabulary.json`, underscore-prefixed dirs are never linted/indexed.
- Keep `internal/resolve/` dependency-light; language-specific resolvers go in `resolve/extension.go`, external ones as `pylon-ext-<name>` executables (protocol: `cli/docs/extension-protocol.md`).
- Release: `scripts/release.sh [tag]` — bare semver, tag must equal `cli/internal/version`'s `Version`; requires a pushed, clean tree; gates gofmt + vet + tests + own-ledger lint + AppleDouble sweep; cross-builds linux/darwin (amd64+arm64) + windows, packs the `pylons/` skeleton, smokes the install, then creates the draft release and publishes it. `scripts/install.sh` is the distribution channel (curl-piped): each install places a self-contained `pylon` inside its ledger, and several ledgers per machine are the designed model — no package-manager formula.
- Verify wire-level, not unit-only: gates green means `gate pre-commit` + `gate pre-push` exit 0 on the product ledger, not just package tests.

## Commits

Conventional Commits, imperative, no trailing period: `fix:`, `feat:`, `docs:`, `chore(release): X.Y.Z`. Short WHY-first body. No AI attribution or sign-offs.
