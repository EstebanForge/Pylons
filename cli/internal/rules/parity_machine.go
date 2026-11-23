// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// parityTransition is one extracted (from, event, to) triple in
// comparison-token space.
type parityTransition struct {
	from  string
	event string
	to    string
}

// parityMachineMarkerRe matches the pylon:machine declaration
// directive on the line above a transition map.
var parityMachineMarkerRe = `^// pylon:machine (\S+)\s*$`

// builtinParityTransitionExtractors holds the in-process transition
// extractors per language. A binding for a language missing here has
// no shipped transition extractor yet.
var builtinParityTransitionExtractors = map[string]func(source, machineID string) ([]parityTransition, error){
	"go":   extractGoTransitions,
	"php":  extractPhpTransitions,
	"rust": extractRustTransitions,
}

// goConstEntry is one const-table row: the token a case identifier
// resolves to, and whether a literal backing value was present.
type goConstEntry struct {
	token  string
	valued bool
}

// goConstTables walks every const block and maps each case name to
// its comparison token: the quoted string value when present, else
// the identifier. Iota and unvalued continuations compare as names,
// matching the shipped set-check rule.
func goConstTables(f *ast.File) map[string]goConstEntry {
	table := map[string]goConstEntry{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name == "_" {
					continue
				}
				entry := goConstEntry{token: name.Name}
				if i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						entry.token = strings.Trim(lit.Value, "\"`")
						entry.valued = true
					}
				}
				table[name.Name] = entry
			}
		}
	}
	return table
}

// extractGoTransitions finds the transition map marked with
// // pylon:machine <machineID> and walks the canonical shape:
// GenDecl(var) -> ValueSpec -> CompositeLit(map[S]map[E]S)
// -> KeyValueExpr(Ident: CompositeLit{Ident: Ident}). Every key and
// value must be a const identifier from the file's tables. Anything
// else fails closed. Duplicate outer keys parse but do not compile;
// both triples are returned so the duplicate rule can judge them.
func extractGoTransitions(source, machineID string) ([]parityTransition, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "machine.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("the machine source does not parse: %w", err)
	}
	table := goConstTables(f)
	var out []parityTransition
	var sawID string
	found := false
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		id, ok := machineDirectiveID(gd.Doc)
		if !ok || id != machineID {
			if ok {
				sawID = id
			}
			continue
		}
		found = true
		triples, err := goTransitionTriples(gd, table, id)
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

// machineDirectiveID scans a doc comment group for the pylon:machine
// directive and returns its id.
func machineDirectiveID(doc *ast.CommentGroup) (string, bool) {
	if doc == nil {
		return "", false
	}
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
		if rest, ok := strings.CutPrefix(text, "pylon:machine "); ok {
			id := strings.TrimSpace(rest)
			if id != "" {
				return id, true
			}
		}
	}
	return "", false
}

// goTransitionTriples walks one marked var declaration's nested map
// literal and emits the resolved triples.
func goTransitionTriples(gd *ast.GenDecl, table map[string]goConstEntry, id string) ([]parityTransition, error) {
	var out []parityTransition
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Values) != 1 {
			return nil, fmt.Errorf("machine %s must be a single package-level map var", id)
		}
		outer, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("machine %s must be a map literal", id)
		}
		mt, ok := outer.Type.(*ast.MapType)
		if !ok {
			return nil, fmt.Errorf("machine %s must be typed map[S]map[E]S", id)
		}
		inner, ok := mt.Value.(*ast.MapType)
		if !ok {
			return nil, fmt.Errorf("machine %s must be typed map[S]map[E]S, not a flat map", id)
		}
		for _, elt := range outer.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("machine %s: outer entries must be key-value pairs", id)
			}
			from, err := goTripleToken(kv.Key, table, id, "state")
			if err != nil {
				return nil, err
			}
			innerLit, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("machine %s: the value for %s must be a nested map literal", id, from)
			}
			for _, ielt := range innerLit.Elts {
				ikv, ok := ielt.(*ast.KeyValueExpr)
				if !ok {
					return nil, fmt.Errorf("machine %s: inner entries for %s must be key-value pairs", id, from)
				}
				event, err := goTripleToken(ikv.Key, table, id, "event")
				if err != nil {
					return nil, err
				}
				to, err := goTripleToken(ikv.Value, table, id, "state")
				if err != nil {
					return nil, err
				}
				out = append(out, parityTransition{from: from, event: event, to: to})
			}
		}
		_ = inner
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("machine %s declares no transitions", id)
	}
	return out, nil
}

// goTripleToken resolves one triple position: an identifier that must
// exist in the const tables. Comparison-token space follows the
// per-case rule (value when valued, else name).
func goTripleToken(e ast.Expr, table map[string]goConstEntry, id, role string) (string, error) {
	ident, ok := e.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("machine %s: a %s position is not a const identifier; the canonical shape allows const identifiers only", id, role)
	}
	entry, ok := table[ident.Name]
	if !ok {
		return "", fmt.Errorf("machine %s: %s is not a known const identifier", id, ident.Name)
	}
	return entry.token, nil
}
