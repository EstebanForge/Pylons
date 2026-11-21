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

- `pylon lint [--changed] [--doc-only] [--fix] [--parity] [--ci] [--format=human|json|sarif]`
- `pylon contract <plan> [--ci] [--format=human|json|sarif]`
- `pylon check <paths...> [--stop-block] [--format=human|json|sarif]`
- `pylon brief [--format=human|json]`
- `pylon context <paths...> [--preamble] [--format=human|json]`
- `pylon context --diff-since <ref> [--manifest]`
- `pylon context --graph <package> [--depth N]`
- `pylon review plan <path>`
- `pylon review postmortem [path] [--check-sources]`
- `pylon review staleness [--package <pkg>] [--fix]`
- `pylon review ratchet [--update]`
- `pylon review rework [--window <period>] [--package <pkg>]`
- `pylon review merge-danger --body <file|-> [--base <ref>]`
- `pylon review release-surface --pinned-releases <dir> [--base <ref>]`
- `pylon review annotations --from <file|-> [--path-prefix <p>]`
- `pylon sync [--detached]`
- `pylon review-brief <plan> [--stdout] [--base <ref>] [--validate <file> | --validate-resolutions <file>]`
- `pylon protocol [list|<name>] [--template <path>]`
- `pylon reflect [--template <path>]`
- `pylon extensions [list|schema] [--format=human|json]`
- `pylon map [--type <t>] [--package <p>] [--tag <t>] [--status <s>] [--folder <dir>] [--query <q>] [--limit <n>] [--all]`
- `pylon search <query> [--limit <n>] [--all]`
- `pylon manifest [rules|packages] [--check]`
- `pylon explain <rule_id>`
- `pylon aroma search|recommend --corpus <dir> (--lines <path>:a-b | --snippet <file|-> | --diff <file|->) [--json]`
- `pylon install`
- `pylon init [dir]`

# Project AGENTS extension
Read the contents of @AGENTS.project.md
