# Extension examples

Reference plugins for the `pylon-ext-<name>` protocol. Copy one into `extensions/` (or anywhere on `PATH`), make it executable, and enable its name in `manifests/pylons.yaml`.

- `pylon-ext-golang`, `pylon-ext-js`, `pylon-ext-php`, `pylon-ext-ruby`, `pylon-ext-rust` answer contract resolution for their language.
- `pylon-ext-parity` answers the parity `states` and `transitions` ops for a small canonical form; copy it and swap the parsers for your language.

The CLI ignores every directory starting with `_`, so nothing here is ever dispatched as a live extension. See meta/EXTENSIONS.md for the full protocol.
