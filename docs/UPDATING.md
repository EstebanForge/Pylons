---
title: "Updating a Pylon"
type: meta
status: active
created: 2026-11-27
updated: 2026-11-27
---

# Updating a Pylon

How to bring an existing pylon to a new release: the `pylon` binary, the product files beside it, or both. First install lives in [INSTALL.md](INSTALL.md); command flags live in the dictionary in [USAGE.md](USAGE.md). A release never touches your zones (`packages/`, `plans/`, `quirks/`, and the rest). It updates the engine and the product layer only.

## 1. Check what you run

`pylon --version` prints the engine version. The last applied release is recorded in `manifests/product.json` under `version`; that file exists after the first update or a fresh install.

## 2. Pick an update path

| Goal | Command | Touches |
|---|---|---|
| Everything, simplest | Re-run the piped installer against the ledger directory | Binary and product files |
| Everything, manual | Download a release, then `pylon self-update --from <dir>` | Binary and product files |
| Binary only | `install.sh --bin-only <ledger-dir>` | `bin/` only |

## 3. The piped installer (recommended)

Run the same command as the first install, with the ledger directory as the target:

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash -s -- path/to/ledger
```

The installer sees `manifests/pylons.yaml` in the target, downloads the latest release, verifies `checksums.txt`, and hands the staged release to the new binary. The binary runs `self-update` under the hood; section 5 says what that does. Pin a version, or read from a local bundle:

```bash
curl -fsSL ... | PYLONS_VERSION=0.10.4 bash -s -- path/to/ledger
curl -fsSL ... | PYLONS_BASE_URL=file:///mnt/offline PYLONS_VERSION=0.10.4 bash -s -- path/to/ledger
```

If the run ends with `update applied with kept edits`, some files carried your edits. Review them (section 6), then run the same command again; the rerun ends clean.

## 4. Manual self-update

Download `pylons-<tag>-<os>-<arch>.tar.gz` and `pylons-kb-<tag>.tar.gz` from the releases page. Verify both against `checksums.txt`, then stage and update:

```bash
mkdir stage
tar -xzf pylons-0.10.4-linux-amd64.tar.gz -C stage
tar -xzf pylons-kb-0.10.4.tar.gz -C stage        # the release manifest lands here
./stage/bin/pylon self-update --from stage --pylons path/to/ledger --dry-run   # preview
./stage/bin/pylon self-update --from stage --pylons path/to/ledger
```

Both archives go into one stage: the platform tarball brings `bin/`, the kb tarball brings the product files and `manifests/product.json`. `--from` is required; `self-update` refuses to run without a staged release directory. The staged files are checked against the release manifest before anything is written. Use `--force` only when you want the shipped copies to win over local edits (the backups of section 5 keep your versions).

## 5. What the update does to your files

The engine reads `manifests/product.json`: one hash per product file as the last applied release shipped it. Then, per staged file:

| State of the file on disk | Result |
|---|---|
| Unmodified since the last release (hash matches the record) | Replaced with the new shipped copy |
| Edited by you, unchanged upstream | Kept as you wrote it |
| Edited by you, changed upstream | Your edit stays; the new copy lands beside it as `<name>.pylons-new` |
| Missing on disk | Installed fresh |

Every replaced file is backed up under `_pylons-backup/<old-version>/` before it is overwritten. Same-version re-releases get `<old-version>-2`, `<old-version>-3`, and so on. The `pylon` binary swaps last. A `--dry-run` prints the plan and writes nothing.

## 6. Review drift, then converge

Exit code `4` means the update applied and some files carried your edits. That is the mechanism working, not a failure:

1. Open each `<name>.pylons-new` copy and compare with its neighbor.
2. Merge what you want, or delete the copy.
3. Run the same update command again. The release is now recorded, so your edited files count as current, and the run exits `0`.

Exit `1` is a real failure. The ledger is intact and the message names the problem.

## 7. Ledgers from before product manifests

A ledger without `manifests/product.json` has no release record to compare against. The first update therefore reports every product file as drifted. That is the expected first-pass shape. Either resolve each `.pylons-new` copy by hand, or run once with `--force` to accept the shipped state, then re-run clean. The run records `manifests/product.json`; every later update three-ways normally. On that first pass the backups land under `_pylons-backup/unknown/`, because there is no old version to name.

## 8. Binary-only updates

When only the engine changed, skip the file merge:

```bash
curl -fsSL ... | bash -s -- --bin-only path/to/ledger
```

`--bin-only` needs an existing directory. It strips the tarball's `bin/` into the ledger's `bin/` and stops; it never scaffolds, initializes git, or touches hooks. Gate hooks are thin shims that call `bin/pylon`, so a binary swap is enough.
