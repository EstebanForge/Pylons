# Extensions (`extensions/`)

This directory holds drop-in `pylon-ext-<name>` executables consumed by the `pylon` CLI.

## Rules

- Name executables `pylon-ext-<name>` and mark them executable (`chmod +x`).
- Follow the v1 JSON stdin/stdout contract in `../manifests/extensions.json`.
- Enable each extension in `../manifests/pylons.yaml` under `extensions:` (globally or per project).
- Inspect active and available extensions with `../bin/pylon extensions list`.
