// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Package golang is the Go front end of the algorithm: it turns Go source
// into simplified parse trees with stdlib go/parser and go/scanner. Keyword
// and punctuation tokens come from scanning the source gaps between a node's
// direct children, in source order, the way the reference implementation's
// Lezer builder does. Names and literals are the AST's identifier and literal
// nodes; true, false, nil, and iota read as literals.
package aroma

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// GoLanguage is the Go front end.
var GoLanguage = Language{
	ID:         "go",
	Extensions: []string{".go"},
	ParseMethods: func(source, path string) []Method {
		return ParseMethods(source, path)
	},
	ParseSnippet: func(source string, options SnippetOptions) (Node, bool) {
		return ParseSnippet(source, options)
	},
}

// scanTok is one token of the source, comments skipped.
type scanTok struct {
	offset int
	line   int
	text   string
}

type parser0 struct {
	source  string
	fset    *token.FileSet
	file    *token.File
	scanned []scanTok
}

func newParser(source string) (*parser0, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, 0)
	p := &parser0{source: source, fset: fset}
	if file != nil {
		p.file = fset.File(file.Pos())
	}
	p.scan()
	return p, file, err
}

// scan records every token of the source with comments skipped. Auto-inserted
// semicolons keep their position and read as ";".
func (p *parser0) scan() {
	var s scanner.Scanner
	s.Init(p.file, []byte(p.source), nil, 0)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			continue
		}
		offset := p.file.Offset(pos)
		text := lit
		if text == "" || tok == token.SEMICOLON {
			text = tok.String()
		}
		p.scanned = append(p.scanned, scanTok{
			offset: offset,
			line:   p.file.Line(pos),
			text:   text,
		})
	}
}

func (p *parser0) lineOf(pos token.Pos) int {
	return p.file.Line(pos)
}

// offsetOf is the byte offset of a position in the parsed file.
func (p *parser0) offsetOf(pos token.Pos) int {
	return p.file.Offset(pos)
}

// childrenOf maps each node to its direct child nodes, in source order. A
// single pass builds it: ast.Inspect calls its function before a node's
// children, so the still-open nodes form the ancestor stack, and with nil
// after them.
func childrenOf(root ast.Node) map[ast.Node][]ast.Node {
	children := make(map[ast.Node][]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		parent := ast.Node(nil)
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		children[parent] = append(children[parent], node)
		stack = append(stack, node)
		return true
	})
	return children
}

// parentsOf maps each node to its direct parent; the root's parent is nil.
func parentsOf(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		parent := ast.Node(nil)
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		parents[node] = parent
		stack = append(stack, node)
		return true
	})
	return parents
}

// identifierRole reports whether an identifier is a structural name rather
// than a variable reference: a selector's field, a composite-literal key, or
// a statement label. Those must never read as locals.
func identifierRole(node ast.Node, parents map[ast.Node]ast.Node) string {
	switch parent := parents[node].(type) {
	case *ast.SelectorExpr:
		if parent.Sel == node {
			return "field"
		}
	case *ast.KeyValueExpr:
		if parent.Key == node {
			return "key"
		}
	case *ast.LabeledStmt:
		if parent.Label == node {
			return "label"
		}
	case *ast.BranchStmt:
		if parent.Label == node {
			return "label"
		}
	}
	return "reference"
}

// collectDeclared gathers the names declared in a method's range: receiver,
// parameters, and named results of the method itself, plus everything
// declared inside the body. Type names and the method's own name are not
// variables.
func collectDeclared(decl *ast.FuncDecl) map[string]bool {
	local := map[string]bool{}
	if decl.Recv != nil {
		addFieldNames(decl.Recv, local)
	}
	if decl.Type != nil {
		addFieldNames(decl.Type.Params, local)
		addFieldNames(decl.Type.Results, local)
	}
	collectDeclaredIn(decl.Body, local)
	return local
}

// collectDeclaredIn gathers the names declared inside one body: :=, var and
// const specs, range DEFINE, and nested function signatures.
func collectDeclaredIn(body ast.Node, local map[string]bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						local[ident.Name] = true
					}
				}
			}
		case *ast.GenDecl:
			if n.Tok == token.VAR || n.Tok == token.CONST {
				for _, spec := range n.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range valueSpec.Names {
							local[name.Name] = true
						}
					}
				}
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				for _, target := range []ast.Expr{n.Key, n.Value} {
					if ident, ok := target.(*ast.Ident); ok {
						local[ident.Name] = true
					}
				}
			}
		case *ast.FuncLit:
			addFieldNames(n.Type.Params, local)
			addFieldNames(n.Type.Results, local)
		}
		return true
	})
}

func addFieldNames(fields *ast.FieldList, into map[string]bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			into[name.Name] = true
		}
	}
}

// convert turns one AST node into a simplified parse tree. It leaves the
// children map untouched so callers can reuse it.
func (p *parser0) convert(node ast.Node, children map[ast.Node][]ast.Node, parents map[ast.Node]ast.Node, local func(string) bool) Node {
	switch n := node.(type) {
	case *ast.Ident:
		role := identifierRole(node, parents)
		isLocal := role == "reference" && local(n.Name)
		return Token{Text: n.Name, Keyword: false, Local: isLocal, Line: p.lineOf(n.Pos()), EndLine: p.lineOf(n.End())}
	case *ast.BasicLit:
		// A literal is never a local variable; only identifiers can be.
		return Token{Text: n.Value, Keyword: false, Local: false, Line: p.lineOf(n.Pos()), EndLine: p.lineOf(n.End())}
	}

	kids := children[node]
	var out []Node
	at := p.offsetOf(node.Pos())
	appendGap := func(end int) {
		// The scan is offset-ascending, so start at the first token past
		// `at` instead of rescanning from the top of the file.
		start := sort.Search(len(p.scanned), func(i int) bool { return p.scanned[i].offset >= at })
		for _, entry := range p.scanned[start:] {
			if entry.offset >= end {
				break
			}
			out = append(out, Token{Text: entry.text, Keyword: true, Line: entry.line, EndLine: entry.line})
		}
		at = end
	}
	for _, kid := range kids {
		appendGap(p.offsetOf(kid.Pos()))
		converted := p.convert(kid, children, parents, local)
		if converted != nil {
			out = append(out, converted)
		}
		at = p.offsetOf(kid.End())
	}
	appendGap(p.offsetOf(node.End()))
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 {
		switch only := out[0].(type) {
		case *Tree:
			return only
		case Token:
			if !only.Keyword {
				return only
			}
		}
	}
	return &Tree{Children: out}
}

// ParseMethods returns the methods of one Go file: FuncDecl bodies and the
// bodies of package-level FuncLit values. A partial AST from a file with
// syntax errors is still used; a file that cannot be read at all returns
// nothing.
func ParseMethods(source, path string) []Method {
	p, file, _ := newParser(source)
	if file == nil {
		return nil
	}
	var methods []Method
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			methods = append(methods, p.methodOf(fn))
			continue
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, value := range valueSpec.Values {
					if lit, ok := value.(*ast.FuncLit); ok {
						// var a, b = f(), g() pairs each literal with
						// its own name.
						name := ""
						if i < len(valueSpec.Names) {
							name = valueSpec.Names[i].Name
						}
						methods = append(methods, p.funcLitMethod(lit, name))
					}
				}
			}
		}
	}
	return methods
}

func (p *parser0) methodOf(fn *ast.FuncDecl) Method {
	local := collectDeclared(fn)
	children := childrenOf(fn.Body)
	parents := parentsOf(fn.Body)
	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		receiver := fn.Recv.List[0].Type
		name = p.source[p.offsetOf(receiver.Pos()):p.offsetOf(receiver.End())] + "." + name
	}
	return Method{
		Name:      name,
		StartLine: p.lineOf(fn.Pos()),
		EndLine:   p.lineOf(fn.End()),
		Body:      p.convert(fn.Body, children, parents, localLookup(local)),
	}
}

func (p *parser0) funcLitMethod(lit *ast.FuncLit, name string) Method {
	local := map[string]bool{}
	addFieldNames(lit.Type.Params, local)
	addFieldNames(lit.Type.Results, local)
	collectDeclaredIn(lit.Body, local)
	children := childrenOf(lit.Body)
	parents := parentsOf(lit.Body)
	return Method{
		Name:      name,
		StartLine: p.lineOf(lit.Pos()),
		EndLine:   p.lineOf(lit.End()),
		Body:      p.convert(lit.Body, children, parents, localLookup(local)),
	}
}

func toNodes(decls []ast.Decl) []ast.Node {
	nodes := make([]ast.Node, len(decls))
	for i, decl := range decls {
		nodes[i] = decl
	}
	return nodes
}

func localLookup(local map[string]bool) func(string) bool {
	return func(name string) bool { return local[name] }
}

// ParseSnippet parses a code fragment that may not be a whole statement or
// method. It first reads the fragment as statements inside a function, then
// as top-level declarations, then as one expression. It reports false when
// nothing parses or no names or literals result.
func ParseSnippet(source string, options SnippetOptions) (Node, bool) {
	source = dedent(source)
	if strings.TrimSpace(source) == "" {
		return nil, false
	}
	p, file, err := newParser("package _p\n\nfunc _snippet() {\n" + source + "\n}\n")
	if err == nil && file != nil && len(file.Decls) > 0 {
		if fn, ok := file.Decls[0].(*ast.FuncDecl); ok && fn.Body != nil {
			children := childrenOf(fn.Body)
			parents := parentsOf(fn.Body)
			node := p.convert(fn.Body, children, parents, snippetLocal([]ast.Node{fn}, options))
			if node != nil {
				return node, true
			}
		}
	}
	p, file, err = newParser("package _p\n\n" + source + "\n")
	if err == nil && file != nil && len(file.Decls) > 0 {
		children := childrenOf(file)
		parents := parentsOf(file)
		var parts []Node
		for _, decl := range file.Decls {
			converted := p.convert(decl, children, parents, snippetLocal(toNodes(file.Decls), options))
			if converted != nil {
				parts = append(parts, converted)
			}
		}
		if len(parts) > 0 {
			return &Tree{Children: parts}, true
		}
	}
	p, file, err = newParser("package _p\n\nvar _snippet = (" + source + ")\n")
	if err == nil && file != nil && len(file.Decls) > 0 {
		if gen, ok := file.Decls[0].(*ast.GenDecl); ok && len(gen.Specs) > 0 {
			if valueSpec, ok := gen.Specs[0].(*ast.ValueSpec); ok && len(valueSpec.Values) > 0 {
				children := childrenOf(valueSpec.Values[0])
				parents := parentsOf(valueSpec.Values[0])
				node := p.convert(valueSpec.Values[0], children, parents, snippetLocal([]ast.Node{valueSpec.Values[0]}, options))
				if node != nil {
					return node, true
				}
			}
		}
	}
	return nil, false
}

// snippetLocal decides whether a snippet name is a local variable. A name the
// snippet declares is local. An undeclared name is local unless it is called,
// capitalized, marked global by the caller, or reads as a package name: the
// left side of a selector, which a snippet cannot resolve through imports.
func snippetLocal(roots []ast.Node, options SnippetOptions) func(string) bool {
	declared := map[string]bool{}
	called := map[string]bool{}
	selectors := map[string]bool{}
	for _, root := range roots {
		if fn, ok := root.(*ast.FuncDecl); ok {
			declared = collectDeclared(fn)
		}
	}
	for _, root := range roots {
		ast.Inspect(root, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.AssignStmt:
				if n.Tok == token.DEFINE {
					for _, lhs := range n.Lhs {
						if ident, ok := lhs.(*ast.Ident); ok {
							declared[ident.Name] = true
						}
					}
				}
			case *ast.CallExpr:
				if ident, ok := n.Fun.(*ast.Ident); ok {
					called[ident.Name] = true
				}
			case *ast.SelectorExpr:
				if ident, ok := n.X.(*ast.Ident); ok {
					selectors[ident.Name] = true
				}
			}
			return true
		})
	}
	return func(name string) bool {
		if declared[name] {
			return true
		}
		first, _ := utf8.DecodeRuneInString(name)
		if called[name] || unicode.IsUpper(first) || options.Globals[name] || selectors[name] {
			return false
		}
		return true
	}
}

// dedent strips the indentation every non-empty line shares, so an indented
// fragment parses.
func dedent(source string) string {
	lines := strings.Split(source, "\n")
	common := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if common < 0 || indent < common {
			common = indent
		}
	}
	if common <= 0 {
		return source
	}
	for i, line := range lines {
		if len(line) >= common {
			lines[i] = line[common:]
		}
	}
	return strings.Join(lines, "\n")
}
