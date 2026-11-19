# Pylon CLI Source (`cli/`)

Go module (`github.com/estebanforge/pylons/cli`) for the `pylon` command-line tool.

## Layout

- `cmd/pylon/`: CLI entry point, flag dispatch, and `review-brief` command.
- `internal/config/`: `pylons.yaml` loader, project registry, and per-project extension config.
- `internal/resolve/`: YAML frontmatter parser, `repo://`/`pylon://` URI parser, core contract resolvers, drop-in language/external extensions, and BM25 search index.
- `internal/rules/`: `PYLON*` rule catalog (`manifests/rules.json`), linter passes, collision `check`, `contract`, `staleness`, `review`, and package manifest builder.
- `internal/report/`: `human`, `json`, `sarif`, and `--stop-block` renderers.
- `internal/protocols/`: Protocol loader and `text/template` renderer (reads `../protocols/*.md`, falls back to `templates/*.md`).
- `templates/`: Compiled-in default `.md` protocol templates (`//go:embed *.md`).

## Build and Test

```bash
cd cli
go test ./...
go build -o ../bin/pylon ./cmd/pylon
```

## Rules

- Run `go test ./...` before and after every change.
- After adding or editing rules in `internal/rules/catalog.go`, regenerate `../manifests/rules.json` via `../bin/pylon manifest rules --pylons ..`.
- Keep core contract resolution dependency-light; language-specific or external resolvers belong in `internal/resolve/extension.go` or external `pylon-ext-<name>` executables.
