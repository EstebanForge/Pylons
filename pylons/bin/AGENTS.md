# Binaries (`bin/`)

Holds the compiled `pylon` binary. The installer and `self-update` place it here; a source checkout builds it with `go build -o ../pylons/bin/pylon ./cmd/pylon` from the engine's `cli/` directory.

## Rules

- Do not hand-edit files in this directory.
- Update the binary by running the installer again or `self-update`; it replaces the binary and keeps your ledger files.
