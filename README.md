# Pylons

> Power Overwhelming!

Pylons is a plain-text ledger & map of your platform, and an enforcer of the rules you lay down, checked against the code to keep the map honest. That is the whole idea. Everything else is detail stacked on those 2 halves.

The map is a library of plain text files: Markdown with structured front matter, kept under version control beside the code it describes. It records what every package does, what depends on what, which interfaces are safe to build on, which decisions were already made and why, and which bugs nearly ate someone's weekend.

The checker (`pylon`) is a small command-line binary that reads those files and compares them against the actual code on every commit. If the map claims a symbol, route, or file exists and it is gone, the check fails. The map cannot quietly rot into fiction.

## The problem it exists for

Few real platforms live in 1 codebase. A typical one is a set of applications, libraries, services, and tests. Each piece sits in its own repository, moves on its own clock, and depends on several of the others.

The trouble shows up where they touch. 4 failures come back again and again, and every one of them is older than AI:

- Architectural drift: someone improvises a pattern, the code stops matching the agreed design, and the divergence ships as permanent debt.
- Branch collisions: 2 teams build on the same surface in parallel. Nobody notices until staging breaks, at the worst possible hour.
- Opaque boundaries: a developer guesses which interface is safe to build on. The guess is wrong, a refactor breaks it, and the fix crosses 3 repositories.
- Lessons paid twice: a nasty bug gets fixed, nobody writes down why, and a fresh pair of hands reintroduces it 6 months later.

AI coding agents poured gas on all 4. An agent writes in minutes what a human writes in days, and it wakes up knowing nothing about your system. Left to guess, it invents a pattern that already exists 3 files over, or leans on an interface somebody is halfway through rewriting.

Pylons attacks the root cause. It gives any tracked project or multi-repo platform a shared memory, and a deterministic lie detector for that memory. That is the pitch: engineering standards that check themselves.

## What lives in a pylon

A pylon is organized by rate of change. Slow-moving knowledge sits apart from fast-moving knowledge, so nothing fights over the same files.

| Zone | Folder | Holds |
|---|---|---|
| Packages | `packages/` | One map per tracked project or module: role, dependencies, public surfaces, and protected globs. |
| Proposals | `proposals/` | Draft plans under review. |
| Plans | `plans/` | Accepted and in-progress feature or refactor plans (`plans/archive/` for shipped or abandoned). |
| Decisions | `decisions/` | Architecture Decision Records, immutable once accepted (`decisions/proposals/`, `decisions/archive/`). |
| Conventions | `conventions/` | Cross-project patterns and closed vocabularies (`conventions/proposals/`, `conventions/archive/`). |
| Domain | `domain/` | Ubiquitous language and domain entity definitions. |
| Flows | `flows/` | Mermaid sequence diagrams for cross-component transactions. |
| Quirks | `quirks/` | Append-only ledger of verified traps: symptom, cause, resolution, and affected contracts. |
| Investigations | `investigations/` | Structured debugging records for complex bugs (`investigations/archive/`). |
| Tickets | `tickets/` | Local work items: bugs, tasks, improvements, and chores (`tickets/archive/`). |
| Audits | `audits/` | Point-in-time health and compliance audits (`audits/archive/`). |
| References | `references/` | External 3rd-party API and tool references (`references/archive/`). |
| Templates | `templates/` | Canonical schema templates for every document type. |
| Protocols | `protocols/` | CLI-consumed protocol templates (`pylon protocol`, `pylon reflect`). |
| Extensions | `extensions/` | Drop-in `pylon-ext-<name>` plugins for language or domain contract checks. |
| Manifests | `manifests/` | Machine-generated indexes (`rules.json`, `packages.json`). |
| Meta | `meta/` | Documentation about Pylons itself. |

## Contracts, the core unit

A contract is a written promise about a piece of code: its name, what it does, and whether other code may depend on it. Every contract carries an explicit status, and the status is the whole game.

`stable` means safe to build on. `unstable` means the surface is about to move, so coordinate first. `editing` means somebody is in there right now: stop, and resolve the collision before you type.

Contracts are typed. Pylons ships 7 language-agnostic core kinds (`file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`) so promises about routes, CLI commands, events, or schemas carry the same checkable form as symbols. Drop-in extensions enabled in `pylons.yaml` add language-specific kinds (`go`, `rust`, `typescript`, `python`, `php`) or custom external `pylon-ext-<name>` resolvers.

## The checker

The `pylon` binary lives in `cli/` and runs deterministically with 0 model calls. Every check is schema validation, pattern or symbol matching, git inspection, or exact set intersections.

`check` is the collision detector: point it at the files you are about to touch, and it names every active plan editing or destabilizing that ground, plus protected globs. `lint` walks the whole library for drift: broken links, orphaned documents, schema violations, stale dates, unresolved contracts, and style. `context` hands an agent or human only the slice that applies to the files at hand. `brief`, `map`, and `search` orient sessions and rank matching documents with BM25. `contract`, `staleness`, `review`, `review-brief`, `protocol`, `reflect`, `manifest`, and `explain` verify plans, post-mortems, review packets, and rule specs.

Findings carry 2 grades: `FORCED` failures exit 1 (broken contracts, active collisions, protected paths), and `ADVISORY` notes exit 0 (context and guidance).

## Controlled language

Standards need a controlled language, so the corpus uses ASD-STE100 Simplified Technical English: short sentences, active voice, a closed vocabulary, and clear structure. Documentation reads identically to a human and to a model, stays cheap to load, and stays short enough to maintain.

## The loop that keeps it honest

Every session starts by reading the map (`pylon brief`, `pylon map`) instead of reverse-engineering the platform from scratch. Before the first edit, `pylon check <paths...>` names every active plan and protected glob on the same ground. While working, new quirks, conventions, and contracts get recorded as they are discovered. At the gate, every change passes `pylon lint` so a documented surface that does not exist in code fails.

## Examples

`_examples/` ships a miniature ledger (one machine, one plan, one quirk, and the Go code they check) plus a runnable parity plugin. Lint it, break it, and watch the checker catch the drift:

    ./bin/pylon lint --pylons _examples/ledger --parity

## Quick start

1. Install a pylon into your project: a plain-text map of your platform, a deterministic checker that fails when the map drifts from the code, and gate hooks that enforce it, tracked in its own git repository (downloads the `pylon` binary, unpacks the pylon into `pylons/`):

```bash
curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash
```

2. Register tracked repositories and extensions in `pylons/manifests/pylons.yaml`.
3. Run `pylons/bin/pylon brief` at session start, `pylons/bin/pylon check <paths...>` before edits, and `pylons/bin/pylon lint` before commits.

Prefer to build from source, or want the pylon to live inside this repository instead of a fresh install? See [meta/INSTALL.md](meta/INSTALL.md).

See [meta/INDEX.md](meta/INDEX.md) for installation, usage, drop-in extensions, and design philosophy. Planned work lives in the [roadmap](meta/ROADMAP.md); shipped work lands in the [changelog](meta/CHANGELOG.md).

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
