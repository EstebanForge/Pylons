// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"regexp"
	"slices"
	"strings"
)

var regexpDriveLetter = regexp.MustCompile(`^[A-Za-z]:`)

// URI represents a parsed repo:// or pylon:// reference.
type URI struct {
	Scheme string
	Alias  string
	Branch string
	Path   string
}

// ParseURI parses repo://<alias>[@<branch>]/<path> and pylon://<path>,
// rejecting empty paths, double slashes, and .. traversal segments.
// stripSymbolFragment removes a trailing ::-anchored symbol fragment
// before path validation. Fragments name code symbols (PHP namespaces
// carry backslashes), so only the file path itself is path-checked; an
// empty fragment leaves the string untouched.
func stripSymbolFragment(path string) string {
	if i := strings.Index(path, "::"); i > 0 && i < len(path)-2 {
		return path[:i]
	}
	return path
}

func ParseURI(raw string) (URI, bool) {
	switch {
	case strings.HasPrefix(raw, "repo://"):
		rest := strings.TrimPrefix(raw, "repo://")
		slash := strings.IndexByte(rest, '/')
		if slash <= 0 {
			return URI{}, false
		}
		head := rest[:slash]
		path := stripSymbolFragment(rest[slash+1:])
		if !isSafeRelPath(path) {
			return URI{}, false
		}
		alias, branch, hasBranch := strings.Cut(head, "@")
		if alias == "" || (hasBranch && branch == "") {
			return URI{}, false
		}
		return URI{Scheme: "repo", Alias: alias, Branch: branch, Path: path}, true
	case strings.HasPrefix(raw, "pylon://"):
		path := stripSymbolFragment(strings.TrimPrefix(raw, "pylon://"))
		if !isSafeRelPath(path) {
			return URI{}, false
		}
		return URI{Scheme: "pylon", Path: path}, true
	default:
		return URI{}, false
	}
}

func isSafeRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "//") || strings.Contains(p, "\\") {
		return false
	}
	if strings.HasSuffix(p, "/") {
		return false
	}
	segs := strings.Split(p, "/")
	if regexpDriveLetter.MatchString(segs[0]) {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !slices.Contains(segs, "..") && !slices.Contains(segs, ".")
}
