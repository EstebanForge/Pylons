// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// transition rule ids: the machine layer on top of the parity set
// checks. The declared side is the machines: frontmatter; the code
// side is the marked transition map.
const (
	transitionUnparseableID  = "PYLON053" // error: a declaration or map that cannot be fully resolved
	transitionMismatchID     = "PYLON054" // error: declared and extracted triple sets differ
	transitionUnknownStateID = "PYLON055" // error: a triple names a state outside the declared set
	transitionUnreachableID  = "PYLON056" // error: a state unreachable from initial
	transitionDeadID         = "PYLON057" // error: a non-terminal state with no outgoing transitions
	transitionDuplicateID    = "PYLON058" // error: the same (from, event) declared twice
)

// errParityTransport marks not-evaluated conditions on the code side:
// an unregistered project, an unsafe or unreadable file, an unbound
// language, or a plugin transport failure. They grade per the
// machine's on_unevaluated knob; everything else is a hard error.
var errParityTransport = errors.New("parity transport failure")

// tripleKey is the comparison identity of one transition.
type tripleKey struct{ from, event, to string }

// scanMachineDeclarations judges the declared side only: shape,
// triple references, and the machine graph. It runs with or without
// --parity, so a declared machine is checked before its code side is
// bound.
func scanMachineDeclarations(docs []corpusDoc) []report.Finding {
	seen := map[string]string{}
	var out []report.Finding
	for _, doc := range docs {
		if doc.meta.Type != "domain-entity" {
			continue
		}
		for i := range doc.meta.Machines {
			m := &doc.meta.Machines[i]
			if m.Machine == "" {
				out = append(out, errFinding(transitionUnparseableID, doc.rel, 1,
					"TRANSITION_UNPARSEABLE: a machine entry carries no machine id"))
				continue
			}
			if prev, dup := seen[m.Machine]; dup {
				out = append(out, errFinding(transitionUnparseableID, doc.rel, 1,
					fmt.Sprintf("TRANSITION_UNPARSEABLE: machine id %s is declared again here (first in %s)", m.Machine, prev)))
				continue
			}
			seen[m.Machine] = doc.rel
			out = append(out, machineDeclaredChecks(doc.rel, m)...)
		}
	}
	return out
}

// machineDeclaredChecks validates one machine declaration: the
// mandatory fields, the triple references against the declared state
// set, duplicates, and the graph (reachability from initial, dead
// states).
func machineDeclaredChecks(rel string, m *resolve.MachineDecl) []report.Finding {
	var out []report.Finding
	unparsable := func(msg string) {
		out = append(out, errFinding(transitionUnparseableID, rel, 1, "TRANSITION_UNPARSEABLE: "+msg))
	}
	switch m.OnUnevaluated {
	case "", "advisory", "error":
	default:
		unparsable(fmt.Sprintf("machine %s: on_unevaluated must be advisory or error", m.Machine))
	}
	states := nonEmptyList(m.States)
	if len(states) == 0 {
		unparsable(fmt.Sprintf("machine %s declares no states", m.Machine))
	}
	if len(nonEmptyList(m.Initial)) == 0 {
		unparsable(fmt.Sprintf("machine %s declares no initial states", m.Machine))
	}
	// terminal is optional: a machine can legitimately never
	// terminate (a sync loop returns to idle forever).
	if len(out) > 0 {
		return out
	}
	stateSet := map[string]bool{}
	for _, s := range states {
		stateSet[s] = true
	}
	adj := map[string][]string{}
	outgoing := map[string]bool{}
	seenPair := map[[3]string]bool{}
	for _, tr := range m.Transitions {
		if tr.From == "" || tr.To == "" {
			unparsable(fmt.Sprintf("machine %s carries an incomplete triple", m.Machine))
			continue
		}
		for _, tok := range []string{tr.From, tr.To} {
			if !stateSet[tok] {
				out = append(out, errFinding(transitionUnknownStateID, rel, 1,
					fmt.Sprintf("UNKNOWN_STATE_IN_TRIPLE: %s is not in machine %s's declared states", tok, m.Machine)))
			}
		}
		dup := duplicateKey(tripleKey{tr.From, tr.Event, tr.To})
		if seenPair[dup] {
			out = append(out, errFinding(transitionDuplicateID, rel, 1,
				fmt.Sprintf("DUPLICATE_TRANSITION: %s is declared twice in machine %s", dupLabel(dup), m.Machine)))
			continue
		}
		seenPair[dup] = true
		adj[tr.From] = append(adj[tr.From], tr.To)
		outgoing[tr.From] = true
	}
	if len(out) > 0 {
		return out
	}
	reachable := map[string]bool{}
	queue := nonEmptyList(m.Initial)
	for _, s := range queue {
		reachable[s] = true
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	terminal := map[string]bool{}
	for _, s := range nonEmptyList(m.Terminal) {
		terminal[s] = true
	}
	for _, s := range states {
		if !reachable[s] {
			out = append(out, errFinding(transitionUnreachableID, rel, 1,
				fmt.Sprintf("UNREACHABLE_STATE: %s is not initial and cannot be reached in machine %s", s, m.Machine)))
			continue
		}
		if !outgoing[s] && !terminal[s] {
			out = append(out, errFinding(transitionDeadID, rel, 1,
				fmt.Sprintf("DEAD_STATE: %s has no outgoing transitions and is not terminal in machine %s", s, m.Machine)))
		}
	}
	return out
}

// nonEmptyList trims blank tokens from a declared list.
func nonEmptyList(list []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// machineParityFindings compares declared machines against the code
// side: the states set check through the shipped set-check rules and
// the triple comparison through the transition extractors. Runs under
// lint --parity only.
func machineParityFindings(kbRoot string, cfg config.Config, allowExternal bool) []report.Finding {
	var out []report.Finding
	_ = filepath.WalkDir(filepath.Join(kbRoot, "domain"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "README.md" || name == "AGENTS.md" || filepath.Ext(name) != ".md" || strings.HasPrefix(name, "._") {
			return nil
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(data), &meta) || meta.Type != "domain-entity" {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		out = append(out, machineCodeFindings(kbRoot, rel, meta, cfg, allowExternal)...)
		return nil
	})
	return out
}

func machineCodeFindings(kbRoot, rel string, meta resolve.DocMeta, cfg config.Config, allowExternal bool) []report.Finding {
	var out []report.Finding
	for i := range meta.Machines {
		m := &meta.Machines[i]
		if m.StatesSymbol != "" && len(nonEmptyList(m.States)) > 0 {
			cases, err := resolveEnumCases(kbRoot, cfg, m.StatesSymbol, allowExternal)
			if err != nil {
				out = append(out, machineNotEvaluated(rel, m, err.Error()))
			} else {
				out = append(out, compareStates(rel, m.States, cases)...)
			}
		}
		if m.TransitionsSymbol == "" {
			if len(m.Transitions) > 0 {
				out = append(out, machineNotEvaluated(rel, m,
					fmt.Sprintf("machine %s declares transitions but no transitions_symbol; the code side is not judged", m.Machine)))
			}
			continue
		}
		triples, err := resolveMachineTransitions(kbRoot, cfg, m.TransitionsSymbol, m.Machine, allowExternal)
		if err != nil {
			if errors.Is(err, errParityTransport) {
				out = append(out, machineNotEvaluated(rel, m, err.Error()))
			} else {
				out = append(out, errFinding(transitionUnparseableID, rel, 1, "TRANSITION_UNPARSEABLE: "+err.Error()))
			}
			continue
		}
		out = append(out, compareTriples(rel, m, triples)...)
		out = append(out, scanPersistedKeyWrites(kbRoot, cfg, rel, m)...)
		out = append(out, checkMachineVerifiedBy(kbRoot, cfg, rel, m)...)
	}
	return out
}

// machineNotEvaluated grades a not-evaluated condition per the
// machine's on_unevaluated knob: advisory by default, error when the
// declaration opts in.
func machineNotEvaluated(rel string, m *resolve.MachineDecl, msg string) report.Finding {
	f := advisoryFinding(parityNotEvaluatedID, rel, 1, "PARITY_NOT_EVALUATED: "+msg)
	if m.OnUnevaluated == "error" {
		f.Severity = report.SeverityError
		f.Grade = report.GradeForced
	}
	return f
}

// resolveMachineTransitions resolves a transitions_symbol pointer and
// extracts the triple list through the project's parity profile.
func resolveMachineTransitions(kbRoot string, cfg config.Config, symbol, machineID string, allowExternal bool) ([]parityTransition, error) {
	u, ok := resolve.ParseURI(symbol)
	if !ok || u.Scheme != "repo" {
		return nil, fmt.Errorf("%w: transitions_symbol is not a repo:// URI", errParityTransport)
	}
	root := cfg.ProjectPath(kbRoot, u.Alias)
	if root == "" {
		return nil, fmt.Errorf("%w: project %q is not registered", errParityTransport, u.Alias)
	}
	filePart := uriSurfacePath(u.Path)
	target, safe := resolve.SafeJoin(root, filePart)
	if !safe {
		return nil, fmt.Errorf("%w: transitions_symbol path %q is unsafe", errParityTransport, filePart)
	}
	source, err := readBounded(target)
	if err != nil {
		return nil, fmt.Errorf("%w: the transitions_symbol file %s is unreadable", errParityTransport, filePart)
	}
	lang, prof, bound := parityProfileFor(cfg, u.Alias, filePart)
	if !bound {
		ext := strings.ToLower(filepath.Ext(filePart))
		var claimants []string
		for _, candidate := range builtinProfileOrder {
			if slices.Contains(config.BuiltinProfileExtensions[candidate], ext) {
				claimants = append(claimants, candidate)
			}
		}
		return nil, fmt.Errorf("%w: no parity profile binds %s, which claims %s", errParityTransport, strings.Join(claimants, " or "), filePart)
	}
	use, name, _ := strings.Cut(prof.Use, ":")
	if use == "builtin" {
		extract, ok := builtinParityTransitionExtractors[name]
		if !ok {
			return nil, fmt.Errorf("%w: the builtin parity profile for %s has no in-process transition extractor yet", errParityTransport, name)
		}
		return extract(source, machineID)
	}
	return extractExternalTransitions(cfg, u.Alias, lang, machineID, source, prof, kbRoot, allowExternal)
}

// duplicateKey identifies a transition whose repetition is an error:
// the (from, event) pair when an event exists, the exact from->to
// pair for event-less adjacency transitions.
func duplicateKey(t tripleKey) [3]string {
	if t.event != "" {
		return [3]string{t.from, t.event, ""}
	}
	return [3]string{t.from, "", t.to}
}

func dupLabel(k [3]string) string {
	if k[1] == "" {
		return k[0] + " --> " + k[2]
	}
	return k[0] + " --" + k[1] + "-->"
}

// compareTriples compares declared and extracted triple sets in both
// directions, order-insensitive, and flags duplicate extracted
// transitions a map literal would silently overwrite.
func compareTriples(rel string, m *resolve.MachineDecl, extracted []parityTransition) []report.Finding {
	var out []report.Finding
	dupCount := map[[3]string]int{}
	extractedCount := map[tripleKey]int{}
	for _, t := range extracted {
		extractedCount[tripleKey{t.from, t.event, t.to}]++
		dupCount[duplicateKey(tripleKey{t.from, t.event, t.to})]++
	}
	pairs := slices.Collect(maps.Keys(dupCount))
	slices.SortFunc(pairs, func(a, b [3]string) int { return strings.Compare(dupLabel(a), dupLabel(b)) })
	for _, pair := range pairs {
		if n := dupCount[pair]; n > 1 {
			out = append(out, errFinding(transitionDuplicateID, rel, 1,
				fmt.Sprintf("DUPLICATE_TRANSITION: the code map declares %s %d times", dupLabel(pair), n)))
		}
	}
	declared := map[tripleKey]bool{}
	for _, tr := range m.Transitions {
		declared[tripleKey{tr.From, tr.Event, tr.To}] = true
	}
	var declOnly, codeOnly []string
	for _, tr := range m.Transitions {
		key := tripleKey{tr.From, tr.Event, tr.To}
		if _, ok := extractedCount[key]; !ok {
			declOnly = append(declOnly, fmt.Sprintf("%s --%s--> %s", tr.From, tr.Event, tr.To))
		}
	}
	for key := range extractedCount {
		if !declared[key] {
			codeOnly = append(codeOnly, fmt.Sprintf("%s --%s--> %s", key.from, key.event, key.to))
		}
	}
	slices.Sort(declOnly)
	slices.Sort(codeOnly)
	if len(declOnly) > 0 || len(codeOnly) > 0 {
		out = append(out, errFinding(transitionMismatchID, rel, 1,
			fmt.Sprintf("TRANSITION_MISMATCH: declared only %q; code only %q", declOnly, codeOnly)))
	}
	return out
}

// bypassedWriteID is the advisory: a direct write of the machine's
// persisted state key outside the machine surface.
const bypassedWriteID = "PYLON059"

// persistedKeyWriteRes is the write-shape per language family: a
// field assignment (go, rust, ts, python, csharp, ruby), a php arrow
// assignment, or a bracketed hash-key assignment.
var persistedKeyWriteRes = []*regexp.Regexp{
	regexp.MustCompile(`\.\s*%s\s*=[^=]`),
	regexp.MustCompile(`->\s*%s\s*=[^=]`),
	regexp.MustCompile(`\[\s*["']%s["']\s*]\s*=[^=]`),
}

// scanPersistedKeyWrites looks for direct writes of the machine's
// persisted key in the project source, outside the machine surface
// (the transitions file) and test files. Advisory: a constructor or
// rehydration may be legitimate, and lint cannot prove intent.
func scanPersistedKeyWrites(kbRoot string, cfg config.Config, rel string, m *resolve.MachineDecl) []report.Finding {
	if strings.TrimSpace(m.PersistedKey) == "" || m.TransitionsSymbol == "" {
		return nil
	}
	u, ok := resolve.ParseURI(m.TransitionsSymbol)
	if !ok || u.Scheme != "repo" {
		return nil
	}
	root := cfg.ProjectPath(kbRoot, u.Alias)
	if root == "" {
		return nil
	}
	surfaceRel := uriSurfacePath(u.Path)
	key := regexp.QuoteMeta(strings.TrimSpace(m.PersistedKey))
	res := make([]*regexp.Regexp, 0, len(persistedKeyWriteRes))
	for _, tmpl := range persistedKeyWriteRes {
		res = append(res, regexp.MustCompile(fmt.Sprintf(tmpl.String(), key)))
	}
	var out []report.Finding
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_test.rs") || strings.HasSuffix(name, "Test.php") || strings.HasPrefix(name, "._") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == surfaceRel {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		claimed := false
		for _, langs := range config.BuiltinProfileExtensions {
			if slices.Contains(langs, ext) {
				claimed = true
				break
			}
		}
		if !claimed {
			return nil
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		hits := 0
		for i, line := range strings.Split(string(data), "\n") {
			for _, re := range res {
				if re.MatchString(line) {
					hits++
					if hits == 1 {
						out = append(out, advisoryFinding(bypassedWriteID, rel, i+1,
							fmt.Sprintf("BYPASSED_STATE_WRITE: %s in machine %s writes the persisted state key %q outside the machine surface; route the write through the machine", name, m.Machine, m.PersistedKey)))
					}
					break
				}
			}
		}
		return nil
	})
	return out
}

// checkMachineVerifiedBy resolves the machine's proof pointers. A
// pointer that does not resolve is a dead proof: the not-evaluated
// advisory names it.
func checkMachineVerifiedBy(kbRoot string, cfg config.Config, rel string, m *resolve.MachineDecl) []report.Finding {
	var out []report.Finding
	for _, v := range m.VerifiedBy {
		if strings.TrimSpace(v) == "" {
			continue
		}
		u, ok := resolve.ParseURI(v)
		if !ok || u.Scheme != "repo" {
			out = append(out, advisoryFinding(parityNotEvaluatedID, rel, 1,
				fmt.Sprintf("PARITY_NOT_EVALUATED: machine %s: verified_by %q is not a repo:// URI", m.Machine, v)))
			continue
		}
		root := cfg.ProjectPath(kbRoot, u.Alias)
		if root == "" {
			out = append(out, advisoryFinding(parityNotEvaluatedID, rel, 1,
				fmt.Sprintf("PARITY_NOT_EVALUATED: machine %s: verified_by %q names an unregistered project", m.Machine, v)))
			continue
		}
		target, safe := resolve.SafeJoin(root, uriSurfacePath(u.Path))
		if !safe {
			out = append(out, advisoryFinding(parityNotEvaluatedID, rel, 1,
				fmt.Sprintf("PARITY_NOT_EVALUATED: machine %s: verified_by path %q is unsafe", m.Machine, v)))
			continue
		}
		if _, err := readBounded(target); err != nil {
			out = append(out, advisoryFinding(parityNotEvaluatedID, rel, 1,
				fmt.Sprintf("PARITY_NOT_EVALUATED: machine %s: verified_by %q does not resolve", m.Machine, v)))
		}
	}
	return out
}
