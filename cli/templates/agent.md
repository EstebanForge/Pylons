# Agent Session Loop for a Pylons Platform

Run this protocol at the start of every work session on a platform mapped by Pylons (`{{.Root}}`). The loop is fixed. One pass through it turns an agent's session from loose exploration into checked, recorded work.

## Workspace Context

- **Pylon root**: `{{.Root}}`
- **CLI binary**: `{{.Bin}}`
- **Enabled extensions**: {{if .Extensions}}{{join .Extensions ", "}}{{else}}(none){{end}}
- **Tracked projects**: {{len .Projects}}
{{- range .Projects}}
  - `{{.Alias}}` (ref: `{{.Ref}}`{{if .Tier}}, tier: `{{.Tier}}`{{end}})
{{- end}}

## The Loop

1. **Orient.** Run `{{.Bin}} brief --pylons {{.Root}}`. List the tracked projects and read the package maps for the area you will touch. Run `{{.Bin}} map` when you need to find documents by topic.
2. **Check ground.** Run `{{.Bin}} check <paths>` on every path you plan to edit. A `FORCED` finding is a stop signal: do not edit contested ground. Read the `ADVISORY` findings and the `pylon context <paths>` output first.
3. **Slice the work.** Take one plan as the task boundary; for a `slice: true` plan, `{{.Bin}} context --slice <plan>` loads its contracts, linked quirks, and domain terms and nothing else. If no plan exists, write one in `plans/` with its contracts before editing code. One plan per session. Do not widen scope past the plan's contracts.
4. **Record discoveries.** A trap you hit becomes a `quirks/` post-mortem. A promise the code makes becomes a contract entry on the plan. A settled choice becomes a `decisions/` record. Write these when you learn them, not at the end.
5. **Gate.** Before every commit, run `{{.Bin}} lint --changed` on staged files. Before every push, the gate hooks run the full corpus. Fix `FORCED` findings; read advisories.
6. **Close.** Mark shipped work with its plan status, record the artifact, and let the plan move to `plans/archive/` when it is done.

## Rules

- **Code is truth.** Verify every claim against source on the project's configured branch, never against another document.
- **Deterministic checks only.** The engine never calls a model. Findings are reproducible; do not argue with an AST resolver.
- **Two grades.** Halt on `FORCED`. Read `ADVISORY`, then decide.
- **ASD-STE100.** Every `.md` file you create or edit uses short sentences, active voice, and the closed vocabulary in `pylon://conventions/vocabulary.md`.
- **URI schemes.** Reference code with `repo://<alias>/<path>` and documents with `pylon://<path>`. Never write local machine paths into documents.
- **No direct ledger writes from the engine.** Capture produces candidate documents for human review. A human or an approved skill commits them.

## Why This Order

The loop moves from cheap reads to bounded writes. Orientation and ground checks cost seconds and prevent the two expensive failures: editing on contested ground, and shipping a surface change with stale neighbors. The record step keeps the map honest while the knowledge is fresh, and the gate keeps drift out of history.
