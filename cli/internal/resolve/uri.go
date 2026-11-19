// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"slices"
	"strings"
)

// URI represents a parsed repo:// or pylon:// reference.
type URI struct {
	Scheme string
	Alias  string
	Branch string
	Path   string
}

// ParseURI parses repo://<alias>[@<branch>]/<path> and pylon://<path>,
// rejecting empty paths, double slashes, and .. traversal segments.
func ParseURI(raw string) (URI, bool) {
	switch {
	case strings.HasPrefix(raw, "repo://"):
		rest := strings.TrimPrefix(raw, "repo://")
		slash := strings.IndexByte(rest, '/')
		if slash <= 0 {
			return URI{}, false
		}
		head := rest[:slash]
		path := rest[slash+1:]
		if !isSafeRelPath(path) {
			return URI{}, false
		}
		alias, branch, hasBranch := strings.Cut(head, "@")
		if alias == "" || (hasBranch && branch == "") {
			return URI{}, false
		}
		return URI{Scheme: "repo", Alias: alias, Branch: branch, Path: path}, true
	case strings.HasPrefix(raw, "pylon://"):
		path := strings.TrimPrefix(raw, "pylon://")
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
	return !slices.Contains(strings.Split(p, "/"), "..")
}
