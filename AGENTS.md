# Pylons

This folder is the product's own manual. Everything here travels with every installed pylon: it documents the tool and the ledger model, not any one team's content. Team content lives in the zones beside this folder.

## What a pylon is

A pylon is a plain-text ledger and map of a platform, and an enforcer of the rules the team lays down, checked against the code. Three jobs in one artifact:

- **The ledger.** Decisions, quirks, plans, and conventions live as Markdown files with structured frontmatter, versioned in git beside the code they describe. What was decided, what nearly broke, what is provisional.
- **The map.** Package docs record what each project does, what depends on what, and which interfaces are safe to build on, with typed contracts (`kind` + `id`) the engine resolves against real code.
- **The enforcer.** The `pylon` binary re-checks the claims on every commit. A symbol, route, or machine transition the map declares must exist in code, or the gate fails. The map cannot quietly rot into fiction.

The engine is deterministic: zero model calls, closed vocabularies, stable rule IDs (`PYLON0xx`), and two grades only. `FORCED` stops work; `ADVISORY` informs. That contract is the product's spine; see `MANIFESTO.md`.

## How to behave inside a pylon

- **Source code is truth.** When the map and the code disagree, the map is wrong until the code changes or the map is updated. Never "fix" a failure by weakening a check you do not own.
- **Frontmatter is typed data.** Each zone declares required keys and a closed `status:` set (`manifests/vocabulary.json`). The engine parses it strictly and tells you the line when you break it.
- **Generated manifests are not hand-edited.** `manifests/rules.json` and `manifests/packages.json` are rewritten by the tool and by the gate hooks. Edit the documents, regenerate via the hooks or `pylon manifest`.
- **Underscore directories are not content.** Anything under a `_`-prefixed directory is scratch or examples; the CLI never lints, indexes, or dispatches it.
- **URI schemes, never machine paths.** Reference code with `repo://<alias>/<path>` and documents with `pylon://<path>`.
- **Write ASD-STE100.** Short, direct sentences: 20 words for procedures, 25 for descriptions. The linter enforces it.
- **Create documents with the command, not by hand.** `pylon new <type> <name>` places the file in the right zone, stamps the dates, and fills the template; `pylon new decision <slug> --number` also assigns the next decision number.

## What lives in this folder

- `INDEX.md` - the reading order for all of it.
- `USAGE.md` - daily workflow and the canonical command and flag dictionary (single source; FEATURES points here).
- `FEATURES.md` - what each zone and command fixes, with proof notes.
- `INSTALL.md` - installing, updating, wiring gates.
- `EXTENSIONS.md` - language extensions and external plugins.
- `MANIFESTO.md` - design philosophy and the determinism stance.
- `docs/` - end-user reference schemas: `pylons-yaml.md`, `parity-spec.md`, `adopting.md`. Engine-facing reference pages (machine contract, closed vocabulary, extension protocol) live in `cli/docs/` of the Pylons source repository.
- `CHANGELOG.md` - what shipped, when.
- `ROADMAP.md` - what is planned, source-free.

## Working on this folder

Keep these docs aligned with `../README.md`, `../AGENTS.md`, and `../cli/`. A claim here that the binary contradicts is a defect, not a wording problem. The authoritative agent loop for working in any pylon is `../protocols/agent.md`.
