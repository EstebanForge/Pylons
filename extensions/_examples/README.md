# Extension examples

Reference plugins for the `pylon-ext-<name>` protocol. Copy one into `extensions/` (or anywhere on `PATH`), make it executable, and enable its name in `manifests/pylons.yaml`.

The CLI ignores every directory starting with `_`, so nothing here is ever dispatched as a live extension. See meta/EXTENSIONS.md for the full protocol.
