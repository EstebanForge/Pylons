# Reflection Time: Periodic Pylons Verification and Hygiene

Run this protocol periodically (weekly or monthly) to keep Pylons (`{{.Root}}`) accurate, ordered, and aligned with the live codebases registered in `pylons.yaml`.

## Workspace Context

- **Pylons root**: `{{.Root}}`
- **CLI binary**: `{{.Bin}}`
- **Enabled extensions**: {{if .Extensions}}{{join .Extensions ", "}}{{else}}(none){{end}}
- **Tracked projects**: {{len .Projects}}
{{- range .Projects}}
  - `{{.Alias}}` (ref: `{{.Ref}}`{{if .Tier}}, tier: `{{.Tier}}`{{end}})
{{- end}}

## Core Principles

- **Code is truth**: Verify every claim, symbol, route, CLI command, event, schema, and file path against source code on each project's configured base branch (`ref:` in `pylons.yaml` or package frontmatter, defaulting to `main`), never against other docs or unverified `status: shipped` claims.
- **Self-contained paths**: Run all checks via `{{.Bin}}`.
- **Closed vocabularies**: Follow `pylon://conventions/vocabulary.md` for `status:`, `remediation:`, `guard:`, and folder placement.
- **Immutable decisions**: Never edit accepted decision bodies or renumber `DNNNN-*` files. When superseding or deprecating, update only `status:` and move to `decisions/archive/`.
- **ASD-STE100**: Every scanned `.md` file you create or edit must pass ASD-STE100 (active voice, present tense, <= 20 words per procedural sentence, <= 25 words per descriptive sentence, no suppression markers).
- **URI schemes**: Use `repo://<alias>[@<branch>]/<path>` and `pylon://<path>` per `AGENTS.md`, never local machine paths.
- **Stage before `--changed`**: `{{.Bin}} lint --changed` ignores untracked files. Stage new docs with `git add` before running `--changed`.
- **Scale**: Delegate large zones (`skills/`, large `packages/` sets) to read-only subagents if needed, and merge all findings into one central ledger.

## Zone Coverage

| Zone | Owning Phase | Verification Source | Lifecycle / Archive Rule |
|---|---|---|---|
| `plans/`, `proposals/` | Phase 2 | PRs, base branch, `{{.Bin}} contract` | `shipped` / `abandoned` move to `plans/archive/` |
| `decisions/`, `decisions/proposals/` | Phase 2 | Live architecture and code | `superseded` / `deprecated` move to `decisions/archive/` |
| `conventions/`, `conventions/proposals/` | Phase 2 | Tooling, CI, codebase patterns | `superseded` move to `conventions/archive/` |
| `audits/` | Phase 2 | Remediation plans and code | `superseded` / `withdrawn` / `retired` move to `audits/archive/` |
| `investigations/` | Phase 2 | Fix commits and live code | `resolved` / `abandoned` move to `investigations/archive/` |
| `tickets/` | Phase 2 | Local work items and plans | `done` / `wontfix` move to `tickets/archive/` |
| `packages/` | Phase 3 | `pylons.yaml` registered `repo://` checkouts | `retired` move to `packages/archive/` |
| `references/` | Phase 3 | Code, hooks, 3rd-party docs | `superseded` / `retired` move to `references/archive/` |
| `quirks/`, `flows/`, `domain/` | Phase 4 | Code, tests, `review postmortem` | Retired / superseded docs move to `<zone>/archive/` |
| `skills/`, `prompts/`, `templates/` | Phase 4 | Frontmatter, paths, `conventions/vocabulary.md` | Promote mature prompts to `skills/` |
| `meta/`, `manifests/` | Phase 5 | `manifest rules`, `manifest packages` | Refresh indexes and record pass in `meta/` |

## Working Ledger

Track counts and paths across phases:
- `archived`: docs moved to `archive/`
- `updated`: docs synced to live code
- `pruned`: duplicate or obsolete docs merged or deleted
- `unverified`: docs skipped because target repo or branch is unavailable locally
- `flagged`: ambiguities, open-PR collisions, or decision-vs-code conflicts for human review

---

## Phase 0: Preconditions

1. **Clean tree**: Inside `{{.Root}}`, run `git status -s` (when git-initialized). If dirty, stop and report to the human; do not stash or discard work.
2. **Sync Pylons**: Pull latest changes on the base branch inside `{{.Root}}`.
3. **Refresh tracked project refs**: Fetch `origin` in local project checkouts listed in `pylons.yaml` without modifying local working trees. Record unreachable repos under `unverified`.

**Done when**: Trees are clean and available project refs are current.

---

## Phase 1: Baseline Audit

1. Run baseline diagnostics:
   - `{{.Bin}} brief`
   - `{{.Bin}} lint --ci --format json`
   - `{{.Bin}} map --all --json`
   - `{{.Bin}} staleness`
2. Record initial finding counts and doc counts per zone.

**Done when**: Baseline counts are captured in the working ledger.

---

## Phase 2: Plan, Decision, Convention, Audit, and Investigation Lifecycle

1. **Plans & Proposals (`plans/`, `proposals/`)**:
   - Run `{{.Bin}} contract <plan>` on active plans.
   - Move `shipped` (with non-empty `artifact:`) and `abandoned` plans to `plans/archive/`.
   - Flag `in-progress` plans with no trace (`PYLON027`) or stale `repo://alias@branch/` mentions (`PYLON037`).
2. **Decisions (`decisions/`)**:
   - Verify `accepted` decisions use `DNNNN-slug.md` filenames (`PYLON010`).
   - Move `superseded` or `deprecated` decisions to `decisions/archive/`.
3. **Conventions, Audits, and Investigations**:
   - Move `superseded` conventions to `conventions/archive/`.
   - Check `audits/` for overdue `reaudit:` dates (`PYLON026`) and valid finding row statuses (`PYLON018`).
   - Move `resolved` or `abandoned` investigations (with `## Root Cause` and `## Resolution`) to `investigations/archive/`.
4. Update `updated: YYYY-MM-DD` only on files whose content or frontmatter changed.

**Done when**: Every lifecycle doc sits in its vocabulary-defined folder with an accurate `status:`.

---

## Phase 3: Code-as-Truth Package and Reference Sync

1. **Packages (`packages/*.md`)**:
   - Compare `exposes:`, `depends_on:`, `protected:`, and `ref:` against each registered project checkout.
   - Flag `pending: true` entries that already resolve in code (`PYLON024`) and remove `pending: true` once shipped.
   - Move `retired` package docs to `packages/archive/`.
   - If a registered project is not checked out locally, record it under `unverified`.
2. **References (`references/`)**:
   - Sync `references/` against live dependencies and external APIs. Move `superseded` or `retired` references to `references/archive/`.
3. Update `updated: YYYY-MM-DD` only on modified files.

**Done when**: Package and reference docs match live code or are explicitly marked `unverified`.

---

## Phase 4: Quirks, Flows, Domain, Skills, and Consolidation

1. **Quirks, Flows, and Domain**:
   - Run `{{.Bin}} review postmortem --check-sources`. Verify `sources:` and `contracts:` exist in code; move retired or obsolete quirks to `quirks/archive/`.
   - Verify `flows/*.md` and `domain/**/*.md` match current code (`entities/` status: `proposed` -> `accepted` when shipped); move superseded or retired docs to `<zone>/archive/`.
   - Run `{{.Bin}} check` and `{{.Bin}} context` on hot paths to confirm no stale `FORCED` warnings remain.
2. **Skills, Prompts, and Templates**:
   - Check `skills/` for valid frontmatter, triggers, dead paths, and overlap with `conventions/`.
   - Check `prompts/` for prompts ready to promote to `skills/`, and verify `templates/` match `conventions/vocabulary.md`.
3. **Deduplicate, Summarize, and Prune**:
   - Merge overlapping docs and update incoming links.
   - Condense verbose docs into high-signal facts and tables without stripping frontmatter contracts, `sources:`, `verified_by:`, or linked anchors.
   - Identify orphan docs (`PYLON004`). Do not delete >3 files or any decision, plan, or audit without human confirmation.
4. Run `{{.Bin}} lint --fix` after moves or merges and inspect `git diff --stat`.

**Done when**: Cross-package docs, skills, and templates are consistent and free of duplicates.

---

## Phase 5: Meta Refresh, Verification, and Report

1. **Manifests**:
   - Refresh `manifests/` with `{{.Bin}} manifest rules` and `{{.Bin}} manifest packages`, and verify both with `--check`.
2. **Final Gate** (record exit codes and before/after delta):
   - `{{.Bin}} lint --ci` (must exit 0, or report remaining manual items)
   - `{{.Bin}} review postmortem --check-sources`
   - `{{.Bin}} staleness`
   - `{{.Bin}} map --all --json` (compare against Phase 1 baseline)
3. **Pylons & CLI Improvement Feedback**:
   - Review friction, recurring agent mistakes, zone misuse, lint blind spots, and `AGENTS.md` or template gaps discovered during Phases 1–4.
   - Propose concrete `pylon` lint rules, extension improvements, or `AGENTS.md` clarifications for human review.
4. **Commit & push**: Wait for human approval if you flagged architectural conflicts or queued non-trivial deletions. Otherwise commit inside `{{.Root}}` grouped logically by topic/zone.

## Phase 6: End

Announce the end of the reflection pass. Output a concise summary with:
- Before/after lint and doc counts per zone
- **Archived**, **Updated**, **Consolidated/Pruned**, **Unverified**, and **Flagged for Human** lists
- **Pylons / CLI Improvements**: proposed lint rules, extension enhancements, or template fixes
- Verification command exit codes
