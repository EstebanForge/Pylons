// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"regexp"
	"strings"
)

// Line-parseable canonical machine shapes for the non-Go profiles.
// PHP: a class constant holding a nested array of quoted case values,
// marked by the directive above the const. Rust: a match function
// with (State::A, Event::B) => State::C arms, marked by the directive
// above the fn. Both compare case tokens directly; values for PHP
// enums, case names for Rust unit enums.

// machineDirectivePrefix marks the declaration line above a machine.
const machineDirectivePrefix = "pylon:machine "

// markerLineID returns the machine id on a // pylon:machine <id>
// line, or ok=false for any other line.
func markerLineID(line string) (string, bool) {
	text := strings.TrimSpace(line)
	text = strings.TrimPrefix(text, "//")
	text = strings.TrimSpace(text)
	rest, ok := strings.CutPrefix(text, machineDirectivePrefix)
	if !ok {
		return "", false
	}
	id := strings.TrimSpace(rest)
	return id, id != ""
}

// extractPhpTransitions walks the marked const array. Outer keys are
// quoted state values, inner keys quoted event values, inner values
// quoted state values. PHP silently overwrites duplicate array keys,
// so both entries are returned for the duplicate rule.
func extractPhpTransitions(source, machineID string) ([]parityTransition, error) {
	lines := strings.Split(source, "\n")
	found, sawID := false, ""
	var out []parityTransition
	for i, line := range lines {
		id, ok := markerLineID(line)
		if !ok {
			continue
		}
		if id != machineID {
			sawID = id
			continue
		}
		found = true
		triples, err := phpArrayTriples(lines[i+1:], id)
		if err != nil {
			return nil, err
		}
		out = append(out, triples...)
	}
	if !found {
		if sawID != "" {
			return nil, fmt.Errorf("no pylon:machine %s directive found (the file carries %s)", machineID, sawID)
		}
		return nil, fmt.Errorf("no pylon:machine %s directive found", machineID)
	}
	return out, nil
}

// phpArrayTriples parses the nested array body line by line until the
// closing bracket. Depth tracking counts brackets so nested arrays
// nest correctly.
func phpArrayTriples(lines []string, id string) ([]parityTransition, error) {
	var out []parityTransition
	depth := 0
	var from string
	seenBody := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if c := stripLineComment(trimmed); c != trimmed {
			trimmed = strings.TrimSpace(c)
		}
		trimmed = strings.TrimSuffix(trimmed, ",")
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		opens := strings.Count(trimmed, "[")
		closes := strings.Count(trimmed, "]")
		if opens > 0 && closes > 0 {
			return nil, fmt.Errorf("machine %s: a line mixes brackets; the canonical php shape is one bracket per line", id)
		}
		if opens > 0 {
			if depth == 0 && !seenBody {
				// The const header line, e.g. public const MAP = [.
				depth += opens
				seenBody = true
				continue
			}
			key, ok := phpQuoted(trimmed)
			if !ok {
				return nil, fmt.Errorf("machine %s: an outer array key is not a quoted case value; the canonical php shape is quoted literals only", id)
			}
			from = key
			depth += opens
			continue
		}
		if closes > 0 {
			depth -= closes
			if depth <= 0 {
				if !seenBody {
					return nil, fmt.Errorf("machine %s: an empty machine array", id)
				}
				return out, nil
			}
			continue
		}
		// Inner entry: 'event' => 'to'.
		parts := strings.Split(trimmed, "=>")
		if len(parts) != 2 {
			return nil, fmt.Errorf("machine %s: an entry does not parse as a key-value pair", id)
		}
		event, ok := phpQuoted(parts[0])
		if !ok {
			return nil, fmt.Errorf("machine %s: an event key is not a quoted literal", id)
		}
		to, ok := phpQuoted(parts[1])
		if !ok {
			return nil, fmt.Errorf("machine %s: a state value is not a quoted literal", id)
		}
		out = append(out, parityTransition{from: from, event: event, to: to})
	}
	return nil, fmt.Errorf("machine %s: the transitions array never closes", id)
}

// phpQuoted extracts the first single- or double-quoted literal from
// a trimmed fragment.
func phpQuoted(fragment string) (string, bool) {
	for _, q := range []string{"'", "\""} {
		start := strings.Index(fragment, q)
		if start < 0 {
			continue
		}
		rest := fragment[start+1:]
		end := strings.Index(rest, q)
		if end < 0 {
			return "", false
		}
		return rest[:end], true
	}
	return "", false
}

// extractRustTransitions walks the match arms of the function under
// the directive. Arms parse as (State::A, Event::B) => State::C;
// wildcard fallthrough arms are skipped, never triples.
func extractRustTransitions(source, machineID string) ([]parityTransition, error) {
	lines := strings.Split(source, "\n")
	found, sawID := false, ""
	var out []parityTransition
	inBody := false
	var arm strings.Builder
	armLines := 0
	for _, line := range lines {
		if id, ok := markerLineID(line); ok {
			if found && inBody {
				break
			}
			if id == machineID {
				found = true
				continue
			}
			sawID = id
			continue
		}
		if !found {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !inBody {
			// The match header starts the arm body; enum, impl, and
			// fn lines before it carry their own braces and are
			// skipped, never mistaken for the body.
			if strings.Contains(trimmed, "match") && strings.Contains(trimmed, "{") {
				inBody = true
			}
			continue
		}
		if trimmed == "}" {
			break
		}
		if comment := strings.Index(trimmed, "//"); comment >= 0 {
			trimmed = strings.TrimSpace(trimmed[:comment])
			if trimmed == "" {
				continue
			}
		}
		if arm.Len() == 0 {
			// Structural lines (the fn signature, the match header)
			// carry no triples. An arm opens on => or on a lone (
			// (rustfmt's wrapped tuple pattern).
			if trimmed != "(" && !strings.Contains(trimmed, "=>") {
				continue
			}
		}
		if arm.Len() > 0 {
			// Continuation lines keep their commas: they separate the
			// successor list and tuple-pattern items.
			arm.WriteString(" ")
			arm.WriteString(trimmed)
		} else {
			arm.WriteString(trimmed)
		}
		armLines++
		text := arm.String()
		hasEndComma := strings.HasSuffix(text, ",")
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), ","))
		depth := strings.Count(text, "[") - strings.Count(text, "]")
		depth += strings.Count(text, "(") - strings.Count(text, ")")
		// A trailing => continues only without a comma: `A => ,` is an
		// empty value and must fail, not swallow the next arm.
		if depth > 0 || (!hasEndComma && strings.HasSuffix(text, "=>")) {
			if armLines > 50 {
				return nil, fmt.Errorf("machine %s: adjacency arm never closes", machineID)
			}
			// rustfmt wraps long successor lists and tuple patterns;
			// keep reading until brackets close and the value lands.
			continue
		}
		arm.Reset()
		armLines = 0
		trs, err := parseRustArm(text)
		if err != nil {
			return nil, err
		}
		out = append(out, trs...)
	}
	if arm.Len() > 0 {
		return nil, fmt.Errorf("machine %s: adjacency arm never closes", machineID)
	}
	if !found {
		if sawID != "" {
			return nil, fmt.Errorf("no pylon:machine %s directive found (the file carries %s)", machineID, sawID)
		}
		return nil, fmt.Errorf("no pylon:machine %s directive found", machineID)
	}
	if !inBody {
		return nil, fmt.Errorf("machine %s: no match body under the directive", machineID)
	}
	return out, nil
}

// parseRustArm reads one match arm. Three canonical shapes: tuple
// arms (State::A, Event::B) => State::C (or-patterns with | give one
// triple each), adjacency arms State::A => &[State::B, State::C],
// and direct event-less arms State::A => State::B. Wrapped values
// like Ok(State::B) read through to the inner state path. A
// wildcard-only pattern is legal fallthrough; an arm that has state
// tokens but parses to nothing is a canonical-shape error, never a
// silent skip. A multi-line arm body ({ after =>) is a
// canonical-shape error too.
func parseRustArm(arm string) ([]parityTransition, error) {
	before, after, ok := strings.Cut(arm, "=>")
	if !ok {
		return nil, nil
	}
	if guardRe.MatchString(before) {
		return nil, fmt.Errorf("rust arm %q: guarded arms are outside the canonical form; declare the guard as its own from-state", arm)
	}
	value := strings.TrimSpace(after)
	if strings.HasSuffix(strings.TrimSpace(before), "{") || strings.HasPrefix(value, "{") {
		return nil, fmt.Errorf("rust arm %q: multi-line arm bodies are outside the canonical form; write a bare state path after =>", arm)
	}
	if strings.HasPrefix(value, "&[") {
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("rust arm %q: adjacency value must close with ]", arm)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(value, "&["), "]")
		from, ok := rustStateToken(strings.TrimSpace(before))
		if !ok {
			return nil, fmt.Errorf("rust arm %q: cannot read the source state", arm)
		}
		var out []parityTransition
		for _, part := range strings.Split(inner, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			to, ok := rustStateToken(part)
			if !ok {
				return nil, fmt.Errorf("rust arm %q: cannot read successor %q", arm, part)
			}
			out = append(out, parityTransition{from: from, to: to})
		}
		return out, nil
	}
	var triples []parityTransition
	for _, alt := range strings.Split(before, "|") {
		alt = strings.TrimSpace(alt)
		if rustPatternIsWildcard(alt) {
			return triples, nil
		}
		to, ok := rustStateToken(value)
		if !ok {
			return nil, fmt.Errorf("rust arm %q: cannot read the target state", arm)
		}
		alt = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(alt), "("), ")")
		var parts []string
		for _, p := range strings.Split(alt, ",") {
			if strings.TrimSpace(p) != "" {
				parts = append(parts, strings.TrimSpace(p))
			}
		}
		switch len(parts) {
		case 1:
			from, ok := rustStateToken(strings.TrimSpace(parts[0]))
			if !ok {
				return nil, fmt.Errorf("rust arm %q: cannot read the source state", arm)
			}
			triples = append(triples, parityTransition{from: from, to: to})
		case 2:
			from, ok := rustStateToken(strings.TrimSpace(parts[0]))
			if !ok {
				return nil, fmt.Errorf("rust arm %q: cannot read the source state", arm)
			}
			event, ok := rustStateToken(strings.TrimSpace(parts[1]))
			if !ok {
				return nil, fmt.Errorf("rust arm %q: cannot read the event", arm)
			}
			triples = append(triples, parityTransition{from: from, event: event, to: to})
		default:
			return nil, fmt.Errorf("rust arm %q: pattern must be State::A or (State::A, Event::B)", arm)
		}
	}
	return triples, nil
}

// rustPatternIsWildcard reports a fallthrough arm: _ or (_, _).
func rustPatternIsWildcard(alt string) bool {
	alt = strings.TrimPrefix(alt, "(")
	alt = strings.TrimSuffix(alt, ")")
	for _, part := range strings.Split(alt, ",") {
		if strings.TrimSpace(part) != "_" {
			return false
		}
	}
	return true
}

// rustStateToken reads a state or event token from a pattern or
// value fragment. The preferred form is a path (State::Pending,
// crate::m::State::Pending: the last segment wins); a bare
// identifier is accepted, wrapper names (Ok, Some, Err, None, vec)
// are ignored, and a wildcard is not a token.
func rustStateToken(fragment string) (string, bool) {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" || fragment == "_" {
		return "", false
	}
	paths := rustPathRe.FindAllString(fragment, -1)
	if len(paths) > 0 {
		last := paths[len(paths)-1]
		if i := strings.LastIndex(last, "::"); i >= 0 {
			last = last[i+2:]
		}
		return last, last != ""
	}
	for _, tok := range strings.FieldsFunc(fragment, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_')
	}) {
		if !rustWrapperTokens[tok] {
			return tok, true
		}
	}
	return "", false
}

var rustPathRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:::[A-Za-z_][A-Za-z0-9_]*)+`)

var rustWrapperTokens = map[string]bool{
	"Ok": true, "Some": true, "Err": true, "None": true,
	"ok": true, "some": true, "err": true, "none": true, "vec": true, "Vec": true,
}

// guardRe matches a Rust match-arm guard: `State if cond =>`, a
// conditional transition the canonical flat form cannot represent.
var guardRe = regexp.MustCompile(`(^|[^A-Za-z0-9_])if([^A-Za-z0-9_]|$)`)

// The canonical typescript shape is the nested switch reducer: the outer
// switch reads the state, inner switches read the event, and every
// transition is a string-literal return. Absorbs (state-level or
// non-literal returns), default arms, and throw guards declare no triples;
// anything else inside the switch fails closed.
var (
	tsSwitchRe    = regexp.MustCompile(`^\s*switch\s*\(\s*[A-Za-z_]\w*\s*\)\s*\{\s*$`)
	tsCaseRe      = regexp.MustCompile(`^\s*case\s+("((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*:\s*$`)
	tsReturnRe    = regexp.MustCompile(`^\s*return\s+("((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')\s*;\s*$`)
	tsReturnAnyRe = regexp.MustCompile(`^\s*return\b`)
	tsDefaultRe   = regexp.MustCompile(`^\s*default\s*:\s*(.*)$`)
	tsCloseRe     = regexp.MustCompile(`^\s*\}\s*;?\s*$`)
)

func tsLiteral(m []string) string {
	if strings.HasPrefix(m[1], `"`) {
		return m[2]
	}
	return m[3]
}

func extractTSTransitions(source, machineID string) ([]parityTransition, error) {
	lines := strings.Split(source, "\n")
	found, sawID := false, ""
	var out []parityTransition
	for i, line := range lines {
		id, ok := markerLineID(line)
		if !ok {
			continue
		}
		if id != machineID {
			sawID = id
			continue
		}
		found = true
		triples, err := tsSwitchTriples(lines[i+1:], id)
		if err != nil {
			return nil, err
		}
		out = append(out, triples...)
	}
	if !found {
		if sawID != "" {
			return nil, fmt.Errorf("no pylon:machine %s directive found (the file carries %s)", machineID, sawID)
		}
		return nil, fmt.Errorf("no pylon:machine %s directive found", machineID)
	}
	return out, nil
}

func tsSwitchTriples(lines []string, id string) ([]parityTransition, error) {
	depth := 0 // 1 = the state switch, 2 = an event switch
	fromSet, eventSet := false, false
	var from, event string
	var out []parityTransition
	leading, leadBudget := true, 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		if tsSwitchRe.MatchString(trimmed) {
			leading = false
			depth++
			continue
		}
		if leading {
			leadBudget++
			if leadBudget > 10 {
				return nil, fmt.Errorf("machine %s: no switch follows the directive; the canonical typescript shape is a nested switch reducer", id)
			}
			if strings.Contains(trimmed, "case ") || strings.Contains(trimmed, "return ") {
				return nil, fmt.Errorf("machine %s: reducer logic appears before the state switch; the canonical typescript shape opens with switch (state) {", id)
			}
			continue
		}
		if tsCloseRe.MatchString(trimmed) {
			if depth == 0 {
				return nil, fmt.Errorf("machine %s: an unbalanced closing brace inside the reducer", id)
			}
			depth--
			if depth <= 0 {
				if len(out) == 0 {
					return nil, fmt.Errorf("machine %s: the switch reducer declares no transitions", id)
				}
				return out, nil
			}
			eventSet = false
			event = ""
			continue
		}
		if m := tsCaseRe.FindStringSubmatch(trimmed); m != nil {
			switch depth {
			case 1:
				from, fromSet = tsLiteral(m), true
				event, eventSet = "", false
			case 2:
				if !fromSet {
					return nil, fmt.Errorf("machine %s: an event case appears without a state case; the outer switch must read the state", id)
				}
				event, eventSet = tsLiteral(m), true
			default:
				return nil, fmt.Errorf("machine %s: switches nest two levels deep (state, then event); the canonical typescript shape is two levels", id)
			}
			continue
		}
		if m := tsReturnRe.FindStringSubmatch(trimmed); m != nil {
			if eventSet {
				out = append(out, parityTransition{from: from, event: event, to: tsLiteral(m)})
			}
			continue
		}
		if tsReturnAnyRe.MatchString(trimmed) {
			if eventSet {
				return nil, fmt.Errorf("machine %s: a computed return inside an event case cannot be compared; return a string literal", id)
			}
			continue
		}
		if m := tsDefaultRe.FindStringSubmatch(trimmed); m != nil {
			rest := strings.TrimSpace(m[1])
			if rest == "" || strings.HasPrefix(rest, "throw ") || strings.HasPrefix(rest, "break") {
				continue
			}
			return nil, fmt.Errorf("machine %s: a default arm must not declare a transition; defaults are guards, not triples", id)
		}
		if strings.HasPrefix(trimmed, "throw ") || strings.HasPrefix(trimmed, "break") {
			continue
		}
		return nil, fmt.Errorf("machine %s: %q does not parse; the canonical typescript shape is a nested switch reducer with string-literal cases and returns", id, trimmed)
	}
	return nil, fmt.Errorf("machine %s: the state switch never closes", id)
}
