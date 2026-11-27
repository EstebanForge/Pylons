# Pylon CLI Source (`cli/`)

Go module (`github.com/estebanforge/pylons/cli`) for the `pylon` command-line tool.

## Layout

- `cmd/pylon/`: CLI entry point, flag dispatch, and `review-brief` command.
- `internal/config/`: `pylons.yaml` loader, project registry, and per-project extension config.
- `internal/resolve/`: YAML frontmatter parser, `repo://`/`pylon://` URI parser, core contract resolvers, drop-in language/external extensions, and BM25 search index.
- `internal/rules/`: `PYLON*` rule catalog (`manifests/rules.json`), linter passes, collision `check`, `contract`, `staleness`, `review`, and package manifest builder.
- `internal/report/`: `human`, `json`, `sarif`, and `--stop-block` renderers.
- `internal/protocols/`: Protocol loader and `text/template` renderer (reads `../protocols/*.md`, falls back to `templates/*.md`).
- `docs/`: engine-facing reference pages (`machine-contract.md`, `vocabulary.md`, `extension-protocol.md`). Not shipped to ledgers; `pylons/pylons-docs/` carries the end-user equivalents.
- The ledger lives in `pylons/` and the built binary at `pylons/bin/pylon`; build with `go build -o ../pylons/bin/pylon ./cmd/pylon`.
- `templates/`: Compiled-in default `.md` protocol templates (`//go:embed *.md`).

## Build and Test

```bash
cd cli
go build -o ../pylons/bin/pylon ./cmd/pylon
go test ./...
```

The `cmd/pylon` tests exec the built binary at `pylons/bin/pylon`; `go test` never rebuilds it, so build before testing or a fresh clone fails those tests.

## Rules

- Run `go test ./...` before and after every change.
- After adding or editing rules in `internal/rules/catalog.go`, regenerate `pylons/manifests/rules.json` via `../pylons/bin/pylon manifest rules --pylons ../pylons`.
- Keep core contract resolution dependency-light; language-specific or external resolvers belong in `internal/resolve/extension.go` or external `pylon-ext-<name>` executables.
