// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
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
			if strings.HasPrefix(trimmed, "{") || strings.Contains(trimmed, "{") {
				inBody = true
			}
			continue
		}
		if trimmed == "}" {
			break
		}
		trimmed = strings.TrimSuffix(trimmed, ",")
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || !strings.Contains(trimmed, "=>") {
			// Structural lines (the fn signature, the match header)
			// carry no triples.
			continue
		}
		tr, ok := rustArmShape(trimmed)
		if !ok {
			// A non-triple arm (wildcard or catch-all) is legal
			// fallthrough, never a transition.
			continue
		}
		out = append(out, tr)
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

// rustArmRe parses one match arm: (Path::A, Path::B) => Path::C.
var rustArmShape = func(arm string) (parityTransition, bool) {
	before, after, ok := strings.Cut(arm, "=>")
	if !ok {
		return parityTransition{}, false
	}
	before = strings.TrimSpace(before)
	before = strings.TrimPrefix(before, "(")
	before = strings.TrimSuffix(before, ")")
	before = strings.TrimSpace(before)
	parts := strings.Split(before, ",")
	if len(parts) != 2 {
		return parityTransition{}, false
	}
	from, ok := rustCaseToken(parts[0])
	if !ok {
		return parityTransition{}, false
	}
	event, ok := rustCaseToken(parts[1])
	if !ok {
		return parityTransition{}, false
	}
	to, ok := rustCaseToken(after)
	if !ok {
		return parityTransition{}, false
	}
	return parityTransition{from: from, event: event, to: to}, true
}

// rustCaseToken reads State::Pending or a bare wildcard; a wildcard
// is not a token.
func rustCaseToken(fragment string) (string, bool) {
	fragment = strings.TrimSpace(fragment)
	if fragment == "_" {
		return "", false
	}
	_, token, found := strings.Cut(fragment, "::")
	if !found {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
