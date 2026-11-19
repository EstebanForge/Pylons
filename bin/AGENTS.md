# Binaries (`bin/`)

Holds the compiled `pylon` binary built from `../cli/`.

## Rules

- Do not hand-edit files in this directory.
- Rebuild after `../cli/` changes:

```bash
cd ../cli && go build -o ../bin/pylon ./cmd/pylon
```
