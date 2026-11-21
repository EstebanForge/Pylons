// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"slices"
	"strings"
)

// RuleSpec describes one deterministic Pylons lint rule.
type RuleSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	FixHint     string `json:"fix_hint"`
}

// Catalog holds all Pylons rules keyed by rule ID.
var Catalog = map[string]RuleSpec{
	"PYLON001": {ID: "PYLON001", Name: "frontmatter_invalid", Severity: "error", Description: "Document is missing valid YAML frontmatter or required title/type fields.", FixHint: "Add valid YAML frontmatter from templates/."},
	"PYLON003": {ID: "PYLON003", Name: "contract_unresolved", Severity: "error", Description: "Declared contract entry could not be resolved against tracked project source.", FixHint: "Update contract id/kind, enable the needed extension in pylons.yaml, or mark pending: true on package exposes."},
	"PYLON004": {ID: "PYLON004", Name: "orphan_document", Severity: "error", Description: "Document has no incoming markdown or frontmatter link.", FixHint: "Link the document from a package, plan, or index."},
	"PYLON005": {ID: "PYLON005", Name: "unlinked_domain_term", Severity: "error", Description: "Domain entity term is mentioned without linking to domain/entities/.", FixHint: "Link the first mention to the entity document."},
	"PYLON006": {ID: "PYLON006", Name: "ste100_violation", Severity: "error", Description: "Sentence exceeds ASD-STE100 maximum length (25 words descriptive).", FixHint: "Split long sentences into short active-voice statements."},
	"PYLON007": {ID: "PYLON007", Name: "status_folder_mismatch", Severity: "error", Description: "Document status does not match its folder.", FixHint: "Move the document to the folder matching its status or update status:."},
	"PYLON008": {ID: "PYLON008", Name: "unknown_status", Severity: "error", Description: "Document status is outside the closed vocabulary for its root folder.", FixHint: "Use an allowed status from conventions/vocabulary.md."},
	"PYLON009": {ID: "PYLON009", Name: "shipped_plan_no_artifact", Severity: "error", Description: "Shipped plan must declare a non-empty artifact:.", FixHint: "Set artifact: to the shipping PR, commit, or file path."},
	"PYLON010": {ID: "PYLON010", Name: "accepted_decision_unnumbered", Severity: "error", Description: "Accepted decision must use a numbered DNNNN-slug.md filename.", FixHint: "Rename the file to DNNNN-<slug>.md."},
	"PYLON011": {ID: "PYLON011", Name: "move_target_collision", Severity: "error", Description: "Archive destination already exists for terminal plan.", FixHint: "Resolve or remove the duplicate archive file."},
	"PYLON012": {ID: "PYLON012", Name: "broken_internal_link", Severity: "error", Description: "Markdown link points to a missing local document.", FixHint: "Fix or remove the broken relative markdown link."},
	"PYLON013": {ID: "PYLON013", Name: "dependency_violation", Severity: "error", Description: "Package depends_on references an unknown package.", FixHint: "Add the missing package map or fix depends_on:."},
	"PYLON014": {ID: "PYLON014", Name: "contract_falsifier", Severity: "error", Description: "Accepted or in-progress plan contract.stable entry is missing wrong_if:.", FixHint: "Add wrong_if: describing what invalidates the contract."},
	"PYLON015": {ID: "PYLON015", Name: "stale_documentation", Severity: "advisory", Description: "Document updated: date trails recent changes.", FixHint: "Review and bump updated:."},
	"PYLON016": {ID: "PYLON016", Name: "accepted_plan_missing_evidence", Severity: "error", Description: "Accepted plan must cite at least one ticket: entry.", FixHint: "Add ticket: metadata to the plan frontmatter."},
	"PYLON018": {ID: "PYLON018", Name: "audit_finding_row_invalid", Severity: "error", Description: "Audit findings table row has an unrecognized status.", FixHint: "Use open, fixed, rejected, or moved."},
	"PYLON019": {ID: "PYLON019", Name: "supersession_link_broken", Severity: "error", Description: "Document supersedes: target does not exist.", FixHint: "Fix the superseded path or slug."},
	"PYLON020": {ID: "PYLON020", Name: "proposal_frontmatter_invalid", Severity: "error", Description: "Plan or proposal lacks valid frontmatter fields.", FixHint: "Ensure title, type: plan, and status are set."},
	"PYLON021": {ID: "PYLON021", Name: "proposal_section_missing", Severity: "error", Description: "Plan or proposal is missing a required section heading.", FixHint: "Add ## Problem, ## Design, and ## Milestones."},
	"PYLON022": {ID: "PYLON022", Name: "postmortem_section_missing", Severity: "error", Description: "Quirk post-mortem is missing a required section heading.", FixHint: "Add ## Symptom, ## Cause, and ## Resolution."},
	"PYLON024": {ID: "PYLON024", Name: "pending_contract_resolved", Severity: "advisory", Description: "Pending package exposes entry already resolves in code.", FixHint: "Remove pending: true from the exposes entry."},
	"PYLON025": {ID: "PYLON025", Name: "postmortem_source_not_found", Severity: "error", Description: "Quirk sources: entry does not exist on disk.", FixHint: "Fix or remove the missing source path."},
	"PYLON026": {ID: "PYLON026", Name: "reaudit_overdue", Severity: "advisory", Description: "Active audit reaudit: date is in the past.", FixHint: "Run a new audit or update reaudit:."},
	"PYLON027": {ID: "PYLON027", Name: "in_progress_plan_no_trace", Severity: "advisory", Description: "In-progress plan has no artifact: and no matching branch.", FixHint: "Create a feature branch or set artifact:."},
	"PYLON028": {ID: "PYLON028", Name: "quirk_guard", Severity: "error", Description: "Quirk guard: is invalid or missing after 30 days.", FixHint: "Set guard: to lint-rule, convention, test, or accepted-risk."},
	"PYLON029": {ID: "PYLON029", Name: "protected_glob", Severity: "error", Description: "Protected path entry is malformed or matches a queried edit target.", FixHint: "Ensure each protected entry has glob, owner, and reason."},
	"PYLON030": {ID: "PYLON030", Name: "investigation_filename_invalid", Severity: "error", Description: "Investigation filename must match YYYY-MM-DD-<slug>.md.", FixHint: "Rename the investigation file to YYYY-MM-DD-<slug>.md."},
	"PYLON031": {ID: "PYLON031", Name: "investigation_closure_invalid", Severity: "error", Description: "Closed investigation must include ## Root Cause and ## Resolution.", FixHint: "Add ## Root Cause and ## Resolution sections."},
	"PYLON032": {ID: "PYLON032", Name: "inert_contract_glob", Severity: "error", Description: "Plan editing or unstable glob matches zero files.", FixHint: "Fix or remove the inert glob pattern."},
	"PYLON033": {ID: "PYLON033", Name: "path_mention_style", Severity: "error", Description: "Document contains a raw machine path instead of repo:// or pylon:// URI.", FixHint: "Replace machine paths with repo://<alias>/<path> or pylon://<path>."},
	"PYLON034": {ID: "PYLON034", Name: "folder_entry_agents", Severity: "advisory", Description: "Zone folder has documents but no AGENTS.md entry point.", FixHint: "Add an AGENTS.md guide in the zone folder."},
	"PYLON035": {ID: "PYLON035", Name: "contract_verified_by", Severity: "error", Description: "Contract verified_by: or falsified_by: test symbol did not resolve.", FixHint: "Update the test file::symbol reference."},
	"PYLON036": {ID: "PYLON036", Name: "quirk_contract", Severity: "error", Description: "Quirk contracts: entry is missing kind or id.", FixHint: "Specify both kind and id on each quirk contract."},
	"PYLON037": {ID: "PYLON037", Name: "stale_branch_mention", Severity: "advisory", Description: "Branch qualifier in repo://alias@branch/ does not exist locally.", FixHint: "Update or remove the stale branch qualifier."},
	"PYLON038": {ID: "PYLON038", Name: "plan_filename_invalid", Severity: "error", Description: "Plan or proposal filename must start with a valid type prefix.", FixHint: "Prefix filename with feature-, refactor-, chore-, docs-, or perf-."},
	"PYLON039": {ID: "PYLON039", Name: "term_no_avoid", Severity: "advisory", Description: "Domain entity declares no avoid: anti-terms.", FixHint: "Add avoid: entries naming the wrong near-synonyms beside the term."},
	"PYLON040": {ID: "PYLON040", Name: "contract_unproven", Severity: "advisory", Description: "Provable contract entry carries no verified_by: or falsified_by: proof pointer.", FixHint: "Add verified_by: naming the table test or QA check that proves the contract."},
	"PYLON041": {ID: "PYLON041", Name: "postmortem_field_token", Severity: "error", Description: "Post-mortem field uses a token outside its closed vocabulary.", FixHint: "Use an allowed token from conventions/vocabulary.md."},
	"PYLON042": {ID: "PYLON042", Name: "ratchet_regression", Severity: "error", Description: "Advisory count for a package or zone rose above the committed baseline.", FixHint: "Fix the new advisories, or re-baseline with pylon review ratchet --update after review."},
	"PYLON043": {ID: "PYLON043", Name: "import_undeclared", Severity: "advisory", Description: "A file imports another registered project the owning package does not declare in depends_on.", FixHint: "Declare the dependency in depends_on: or remove the import."},
	"PYLON044": {ID: "PYLON044", Name: "ripple_open", Severity: "advisory", Description: "The target surface is referenced by other ledger artifacts that must move in the same change.", FixHint: "Update the listed artifacts in the same session, or accept the stale neighbors knowingly."},
	"PYLON045": {ID: "PYLON045", Name: "slice_incomplete", Severity: "advisory", Description: "A slice plan lacks a behavior contract or the test surface that proves it.", FixHint: "Declare contracts on every layer the slice touches: behavior plus a test file or glob."},
	"PYLON046": {ID: "PYLON046", Name: "contract_no_scenario", Severity: "advisory", Description: "A slice plan's behavior contract has no covering declared scenario.", FixHint: "Add a scenario entry with given/when/expect and covers naming the contract id."},
	"PYLON047": {ID: "PYLON047", Name: "merge_danger", Severity: "error", Description: "The pull request body lacks a valid merge danger line, or the derived minimum door outranks the author door.", FixHint: "Add 'Merge danger: <one-way|two-way> | blast radius: <scope>' to the body (conventions/pr-template.md)."},
	"PYLON048": {ID: "PYLON048", Name: "release_surface", Severity: "error", Description: "A pinned deployment consumes a public surface that is removed or changed against the pinned release tag, or a pin cannot be judged.", FixHint: "Restore the surface at HEAD or cut and ship a new release for the pinned deployment; unregistered pins fail closed."},
	"PYLON049": {ID: "PYLON049", Name: "parity_not_evaluated", Severity: "advisory", Description: "A parity declaration has one half without the other, or the declared enum cannot be resolved; a skipped set must never read as parity.", FixHint: "Declare states and states_symbol together, and point states_symbol at a repo:// file and enum name that exists."},
	"PYLON050": {ID: "PYLON050", Name: "parity_count_mismatch", Severity: "error", Description: "The declared state count differs from the code enum's case count.", FixHint: "Align the states list with the enum's cases in the same change."},
	"PYLON051": {ID: "PYLON051", Name: "parity_state_mismatch", Severity: "error", Description: "A state token appears on only one side of the declared-vs-code comparison.", FixHint: "Align the states list with the enum's cases in the same change."},
	"PYLON052": {ID: "PYLON052", Name: "parity_route_mismatch", Severity: "advisory", Description: "No source line in the mapped project mentions the declared route path.", FixHint: "Register the route in the mapped project, or drop the expose entry."},
}

// AllRules returns the sorted rule catalog.
func AllRules() []RuleSpec {
	out := make([]RuleSpec, 0, len(Catalog))
	for _, r := range Catalog {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b RuleSpec) int { return strings.Compare(a.ID, b.ID) })
	return out
}
