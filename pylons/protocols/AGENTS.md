# CLI Protocol Templates

This directory is consumed directly by the `pylon` CLI (`pylon protocol <name>`, `pylon reflect`).

## Rules

- Edit `.md` files here only to customize the Go `text/template` protocols rendered by `pylon`.
- Preserve template variables (`{{.Root}}`, `{{.Bin}}`, `{{.Extensions}}`, `{{.Projects}}`).
- If a file is removed from this directory, `pylon` falls back to its compiled-in default in `cli/templates/`.
