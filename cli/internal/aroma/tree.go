// Echo: structural code similarity search, ported from Aroma
// (Luan, Yang, Barnaby, Sen, Chandra; OOPSLA 2019, arXiv:1812.01158) through
// the L9Labs/aroma-typescript reference. See NOTICE for provenance.
//
// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

// Package core holds the language-agnostic Aroma algorithm: the simplified
// parse tree, featurization, pruning, search, and recommend. Language front
// ends live in internal/lang and its subpackages.
package aroma

import "strings"

// One token of a program. Keyword tokens are the ones a language fixes, such
// as `if`, `{`, and `+`; every other token is a name or a literal.
type Token struct {
	Text string
	// Keyword reports a token whose value the language fixes.
	Keyword bool
	// Local reports a name or literal that is a local variable, which
	// featurizes as #VAR.
	Local bool
	// Line and EndLine are the 1-based lines the token starts and ends on.
	Line    int
	EndLine int
}

// Tree is a simplified parse tree (Section 3.1 of the paper): a non-empty
// list of tokens and subtrees, with no node types, so that every language has
// the same representation. A list never holds a single subtree and nothing
// else.
type Tree struct {
	Children []Node
}

// Node is either a Token or a *Tree.
type Node interface{ isNode() }

func (Token) isNode() {}
func (*Tree) isNode() {}

// Label joins a tree's keyword tokens in order, with # for each other token
// and each subtree.
func (t *Tree) Label() string {
	var b strings.Builder
	for _, child := range t.Children {
		if token, ok := child.(Token); ok && token.Keyword {
			b.WriteString(token.Text)
		} else {
			b.WriteByte('#')
		}
	}
	return b.String()
}

// NewTree builds a tree and applies the collapse rule: a list that would hold
// a single subtree and nothing else becomes that subtree. A single keyword
// token stays a tree, because its text is part of a label.
func NewTree(children []Node) Node {
	if len(children) == 1 {
		if _, isToken := children[0].(Token); isToken && children[0].(Token).Keyword {
			return &Tree{Children: children}
		}
		return children[0]
	}
	return &Tree{Children: children}
}
