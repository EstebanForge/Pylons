// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

import (
	"strconv"
	"unicode/utf8"
)

// VAR is what every local variable is called in a feature, so renaming one
// changes nothing.
const VAR = "#VAR"

const separator = "\x1f"

// maxNameLength is the length past which a literal's features keep its start
// and its length, not its whole text. The reference clips in UTF-16 units;
// this port clips by rune count and differs for non-ASCII over this length.
const maxNameLength = 48

// TokenFeature is the token itself.
func TokenFeature(name string) string {
	return "T" + separator + name
}

// ParentFeature is the token under an ancestor: index is the 1-based
// position, among the ancestor's non-keyword children, of the child that
// leads to the token. The parent, grandparent, and great-grandparent all
// produce this one kind.
func ParentFeature(name string, index int, ancestorLabel string) string {
	return "P" + separator + name + separator + strconv.Itoa(index) + separator + ancestorLabel
}

// SiblingFeature is two tokens with no other non-keyword token between them.
func SiblingFeature(previous, next string) string {
	return "S" + separator + previous + separator + next
}

// UsageFeature is the contexts of two consecutive uses of one local variable.
func UsageFeature(previousContext, nextContext string) string {
	return "U" + separator + previousContext + separator + nextContext
}

// PositionContext is the context of a variable that is the indexth
// non-keyword child of a tree.
func PositionContext(index int, parentLabel string) string {
	return strconv.Itoa(index) + ":" + parentLabel
}

// TokenContext is the context of a variable in a member access: the member's
// name.
func TokenContext(name string) string { return "." + name }

// TokenFeatures pairs one non-keyword token with its features.
type TokenFeatures struct {
	Token    Token
	Features []string
}

// FeaturizeOptions tunes featurization.
type FeaturizeOptions struct {
	// MemberAccessLabels are labels of a member access, where a variable's
	// context is the member's name. nil means the default {#.#}.
	MemberAccessLabels map[string]bool
	// Select featurizes the snippet made of only the tokens it accepts:
	// neighbors and consecutive uses are found among them, and their
	// ancestors are the tree's. nil means all non-keyword tokens.
	Select func(Token) bool
}

type frame struct {
	tree   *Tree
	label  string
	index  int
	parent *frame
}

type leaf struct {
	token  Token
	name   string
	index  int
	parent *frame
}

// Featurize extracts the features of each non-keyword token of a simplified
// parse tree, in source order (Section 3.2): the token, its place under up to
// three ancestors, its neighbors, and, for a local variable, how its previous
// and next uses relate. A snippet's features are the multiset union of its
// tokens' features.
func Featurize(root Node, options FeaturizeOptions) []TokenFeatures {
	memberAccessLabels := options.MemberAccessLabels
	if memberAccessLabels == nil {
		memberAccessLabels = map[string]bool{"#.#": true}
	}

	var leaves []leaf
	var visit func(node Node, index int, parent *frame)
	visit = func(node Node, index int, parent *frame) {
		if token, ok := node.(Token); ok {
			if !token.Keyword {
				name := token.Text
				if token.Local {
					name = VAR
				}
				leaves = append(leaves, leaf{token: token, name: clip(name), index: index, parent: parent})
			}
			return
		}
		tree := node.(*Tree)
		current := &frame{tree: tree, label: tree.Label(), index: index, parent: parent}
		position := 0
		for _, child := range tree.Children {
			_, isTree := child.(*Tree)
			isKeyword := false
			if token, ok := child.(Token); ok && token.Keyword {
				isKeyword = true
			}
			if isTree || !isKeyword {
				position++
				visit(child, position, current)
			}
		}
	}
	visit(root, 1, nil)

	selectTokens := options.Select
	var chosen []leaf
	for _, entry := range leaves {
		if selectTokens == nil || selectTokens(entry.token) {
			chosen = append(chosen, entry)
		}
	}

	memberNames := make(map[*Tree]string, len(chosen))
	contextOf := func(entry leaf) string {
		if entry.parent == nil {
			return PositionContext(entry.index, "")
		}
		parent := entry.parent
		if memberAccessLabels[parent.label] {
			name, known := memberNames[parent.tree]
			if !known {
				name = firstOtherName(parent.tree)
				memberNames[parent.tree] = name
			}
			if name != "" {
				return TokenContext(name)
			}
		}
		return PositionContext(entry.index, parent.label)
	}

	result := make([]TokenFeatures, len(chosen))
	lastUse := make(map[string]int)
	for position, entry := range chosen {
		features := []string{TokenFeature(entry.name)}
		index := entry.index
		ancestor := entry.parent
		for level := 0; level < 3 && ancestor != nil; level++ {
			features = append(features, ParentFeature(entry.name, index, ancestor.label))
			index = ancestor.index
			ancestor = ancestor.parent
		}
		result[position] = TokenFeatures{Token: entry.token, Features: features}
	}
	for position := range chosen {
		entry := chosen[position]
		if position+1 < len(chosen) {
			next := chosen[position+1]
			pair := SiblingFeature(entry.name, next.name)
			result[position].Features = append(result[position].Features, pair)
			result[position+1].Features = append(result[position+1].Features, pair)
		}
		if !entry.token.Local {
			continue
		}
		name := entry.token.Text
		if earlier, known := lastUse[name]; known {
			pair := UsageFeature(contextOf(chosen[earlier]), contextOf(entry))
			result[earlier].Features = append(result[earlier].Features, pair)
			result[position].Features = append(result[position].Features, pair)
		} else {
			// A variable's only use still has no usage feature, but the next
			// use of the same name needs this position.
		}
		lastUse[name] = position
	}
	return result
}

// firstOtherName is the first non-keyword token under a tree that is not a
// local variable. An empty result means none exists.
func firstOtherName(tree *Tree) string {
	for _, child := range tree.Children {
		if sub, ok := child.(*Tree); ok {
			if found := firstOtherName(sub); found != "" {
				return found
			}
			continue
		}
		token := child.(Token)
		if !token.Keyword && !token.Local {
			return clip(token.Text)
		}
	}
	return ""
}

func clip(text string) string {
	if utf8.RuneCountInString(text) <= maxNameLength {
		return text
	}
	clipped := make([]rune, 0, maxNameLength)
	for _, r := range text {
		clipped = append(clipped, r)
		if len(clipped) == maxNameLength {
			break
		}
	}
	return string(clipped) + separator + strconv.Itoa(utf8.RuneCountInString(text))
}
