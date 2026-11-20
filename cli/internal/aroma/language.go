// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma

// SnippetOptions tune snippet parsing.
type SnippetOptions struct {
	// Path is a path whose extension picks a dialect, such as .tsx for JSX.
	Path string
	// Globals are names to keep as they are when a snippet neither declares
	// nor imports them.
	Globals map[string]bool
}

// Language turns source into simplified parse trees. It is the one
// language-specific step of the algorithm: it needs the language's grammar,
// to tell keyword tokens from the rest, and its scoping rules, to tell local
// variables from globals. Everything else in this package works on the trees
// alone.
type Language struct {
	// ID separates languages: methods of different languages are never
	// compared.
	ID string
	// Extensions are the file extensions, with the dot, of files this
	// language parses.
	Extensions []string
	// MemberAccessLabels are the labels of a member access such as x.f;
	// nil defaults to {#.#}.
	MemberAccessLabels map[string]bool
	// ParseMethods returns the methods of a file, outermost only, without
	// comments.
	ParseMethods func(source, path string) []Method
	// ParseSnippet parses a code fragment that may not be a whole method,
	// or reports false when it has no tokens.
	ParseSnippet func(source string, options SnippetOptions) (Node, bool)
}

// LanguageForPath picks the language whose extension a path ends with; the
// longest extension wins.
func LanguageForPath(languages []Language, path string) (Language, bool) {
	best := -1
	bestLength := 0
	for i, candidate := range languages {
		for _, extension := range candidate.Extensions {
			if len(path) >= len(extension) && path[len(path)-len(extension):] == extension && len(extension) > bestLength {
				best = i
				bestLength = len(extension)
			}
		}
	}
	if best < 0 {
		return Language{}, false
	}
	return languages[best], true
}
