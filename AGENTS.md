# Pylons

Pylons stores shared architectural context, domain knowledge, and decisions. Keep documentation accurate or delete it. Source Code is Truth.

## Path Schemes

Pylons documents reference files with URI schemes, never machine paths.

| Scheme | Names | Behavior |
|---|---|---|
| `repo://<alias>/<path>` | Any file inside a project registered in `manifests/pylons.yaml` | Resolved against the project's base branch (`ref:` in `manifests/pylons.yaml` or package frontmatter, else `main`). |
| `repo://<alias>@<branch>/<path>` | In-flight code on a named branch | Naming only, never resolved against base branch. |
| `pylon://<path>` | Any Pylons document | Resolved relative to the Pylons root. |

## Rules

1. **Check before coding.** Run `pylon check <paths...>` or scan `plans/` for overlapping contracts.
2. **Use templates.** Scaffold new documents from `templates/`. Preserve frontmatter fields.
3. **Define typed contracts.** Contract entries require `kind` and `id`. Built-in kinds: `file`, `glob`, `symbol`, `route`, `cli`, `event`, `schema`. Enabled extensions in `manifests/pylons.yaml` add language-specific kinds.
4. **Keep decisions immutable.** Never edit accepted ADRs in place. Write a new ADR that supersedes the old decision.
5. **Archive terminal documents.** Set `status: shipped` (with `artifact:`) or `status: abandoned` and move to `plans/archive/`.
6. **Use Simplified Technical English (ASD-STE100).** Write direct technical English. Limit procedural sentences to 20 words and descriptive sentences to 25 words. Use active voice and present tense.
7. **Use closed vocabularies.** Frontmatter `status:` accepts only the values listed in `manifests/vocabulary.json`.
8. **Honor check grades.** `FORCED` (exit 1) stops work. `ADVISORY` (exit 0) informs and lets work proceed.
9. **Underscore directories are not content.** Any directory whose name starts with `_` (for example `extensions/_examples/`) is example or scratch space; the CLI never lints, indexes, lists, or dispatches anything under it.

## CLI Commands

- `pylon lint [--changed] [--doc-only] [--fix] [--parity] [--update-baseline] [--ci] [--format=human|json|sarif]` - judge the ledger, or the staged docs, against every rule.
- `pylon contract <plan> [--ci] [--format=human|json|sarif]` - resolve one plan's typed contracts against real code.
- `pylon check <paths...> [--stop-block] [--format=human|json|sarif]` - ground-check specific paths before editing them.
- `pylon brief [--format=human|json]` - one-line orientation: root, projects, extensions, doc count.
- `pylon context <paths...> [--preamble] [--format=human|json]` - load the docs and contracts that surround given paths.
- `pylon context --diff-since <ref> [--manifest]` - list surfaces whose code moved since a ref but whose docs did not.
- `pylon context --graph <package> [--depth N]` - compare a package's actual import graph against its declared `depends_on`.
- `pylon staleness [--package <pkg>] [--fix]` - find documents past their zone's age gate.
- `pylon review plan <path>` - judge one plan's structure and contract entries.
- `pylon review postmortem [path] [--check-sources]` - judge quirk postmortems and verify their cited sources.
- `pylon review staleness [--package <pkg>] [--fix]` - the staleness pass as a review lens.
- `pylon review ratchet [--update]` - block when advisory counts rise above the recorded baseline.
- `pylon review rework [--window <period>] [--package <pkg>]` - measure fix and refactor churn over feat-changed paths.
- `pylon review merge-danger --body <file|-> [--base <ref>]` - validate a PR body's declared merge door against the derived blast radius.
- `pylon review release-surface --pinned-releases <dir> [--base <ref>]` - compare pinned release surfaces against HEAD.
- `pylon review gate-surface [--base <ref>]` - flag edits to the gates themselves.
- `pylon review annotations --from <file|-> [--path-prefix <p>]` - convert findings into GitHub check annotations for CI.
- `pylon harvest pr --diff <patch|-> [--metadata <json>] [--record <file>]` - extract symbol and quirk candidates from a merged PR's diff.
- `pylon review retro --findings <file|-> [--window <period>] [--min <n>] [--shape <s>]` - turn repeated review findings into one promotion candidate.
- `pylon sync [--detached]` - fetch origin in the background and fast-forward when the tree is clean.
- `pylon review-brief <plan> [--stdout] [--base <ref>] [--validate <file> | --validate-resolutions <file>]` - generate a reviewer packet for a plan.
- `pylon protocol [list|<name>] [--template <path>]` - render an agent protocol (`reflect` is the hygiene pass; `agent` is the session loop).
- `pylon reflect [--template <path>]` - run the hygiene reflection protocol.
- `pylon extensions [list|schema]` - list enabled language extensions and their contract kinds.
- `pylon map [--type <t>] [--package <p>] [--tag <t>] [--status <s>] [--folder <dir>] [--query <q>] [--limit <n>] [--all]` - find documents by zone, status, or query.
- `pylon search <query> [--limit <n>] [--all]` - full-text query over the indexed ledger.
- `pylon gate pre-commit|pre-push` - the commit and push gates, callable directly; the shipped hooks exec these.
- `pylon manifest [rules|packages|product] [--check]` - write or verify the generated manifest schemas.
- `pylon self-update --from <dir> [--dry-run] [--force]` - apply a staged release, protecting user edits.
- `pylon explain <rule_id>` - print one rule's semantics and fix hint.
- `pylon aroma search|recommend --corpus <dir> (--lines <path>:a-b | --snippet <file|-> | --diff <file|->) [--json]` - structural code-similarity search over indexed projects.
- `pylon install` - wire the gate hooks into this ledger's git repository.
- `pylon init [dir]` - scaffold a fresh pylon.
- `pylon new <type> <name> [--title <t>] [--number]` - create a typed document from the ledger's own template.


Read the contents of @AGENTS.project.md
