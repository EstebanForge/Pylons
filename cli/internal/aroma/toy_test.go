// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Toy is the reference implementation's toy test language, kept here as a
// test-only front end. A bracket notation for writing the paper's simplified
// parse trees directly: `[` and `]` group a subtree, `$name` marks a local
// variable, and words and numbers are non-keyword tokens unless the language
// fixes them. Methods are `method <name>` ... `end` blocks.
package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"

	"regexp"
	"strings"
)

var keywords = map[string]bool{
	"if": true, "else": true, "for": true, "while": true,
	"int": true, "instanceof": true, "return": true, "new": true,
}

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$|^\d+$`)

// Language is the toy front end.
var Language = aroma.Language{
	ID:                 "toy",
	Extensions:         []string{".toy"},
	MemberAccessLabels: map[string]bool{"#.#": true},
	ParseMethods:       parseMethods,
	ParseSnippet:       parseSnippet,
}

// ParseMethods parses `method <name>` ... `end` blocks.
func parseMethods(source, path string) []aroma.Method {
	var methods []aroma.Method
	lines := strings.Split(source, "\n")
	start := -1
	name := ""
	for index, line := range lines {
		if header := regexp.MustCompile(`^method (\S+)$`).FindStringSubmatch(strings.TrimSpace(line)); header != nil && start < 0 {
			start = index
			name = header[1]
			continue
		}
		if strings.TrimSpace(line) == "end" && start >= 0 {
			body := parse(lines[start+1:index], start+2)
			if body != nil {
				methods = append(methods, aroma.Method{Name: name, StartLine: start + 2, EndLine: index, Body: body})
			}
			start = -1
		}
	}
	return methods
}

func parseSnippet(source string, options aroma.SnippetOptions) (aroma.Node, bool) {
	return parse(strings.Split(source, "\n"), 1), true
}

func parse(lines []string, firstLine int) aroma.Node {
	var stack [][]aroma.Node
	stack = append(stack, []aroma.Node{})
	for offset, line := range lines {
		for _, word := range strings.Fields(line) {
			switch word {
			case "[":
				stack = append(stack, []aroma.Node{})
			case "]":
				children := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				top := len(stack) - 1
				stack[top] = append(stack[top], collapse(children))
			default:
				top := len(stack) - 1
				stack[top] = append(stack[top], leaf(word, firstLine+offset))
			}
		}
	}
	root := stack[0]
	if len(root) == 0 {
		return nil
	}
	return collapse(root)
}

// collapse keeps the toy language's own rule: a list holding one subtree
// alone becomes it, and anything else stays a tree.
func collapse(children []aroma.Node) aroma.Node {
	if len(children) == 1 {
		if _, ok := children[0].(*aroma.Tree); ok {
			return children[0]
		}
	}
	return &aroma.Tree{Children: children}
}

func leaf(word string, line int) aroma.Token {
	local := strings.HasPrefix(word, "$")
	text := strings.TrimPrefix(word, "$")
	keyword := !local && (keywords[text] || !nameRe.MatchString(text))
	return aroma.Token{Text: text, Keyword: keyword, Local: local, Line: line, EndLine: line}
}
