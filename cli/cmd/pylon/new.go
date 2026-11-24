// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// newDocType maps a ledger document type to its template file, its
// zone folder, and the zone's default status. Placement and the
// default status are engine rules (PYLON007 status/folder matching,
// the status vocabulary), so a created document passes its own lint
// on birth. Investigations additionally carry the PYLON030 date
// prefix, applied here instead of guessed by the caller.
type newDocType struct {
	typ      string
	template string
	dir      string
	status   string
	dated    bool
}

var newDocTypes = []newDocType{
	{typ: "audit", template: "audit", dir: "investigations", status: "active"},
	{typ: "convention", template: "convention", dir: "conventions/proposals", status: "proposed"},
	{typ: "decision", template: "decision", dir: "decisions/proposals", status: "proposed"},
	{typ: "domain-entity", template: "domain-entity", dir: "domain/entities", status: "proposed"},
	{typ: "investigation", template: "investigation", dir: "investigations", status: "active", dated: true},
	{typ: "package", template: "package", dir: "packages", status: "active"},
	{typ: "plan", template: "plan", dir: "plans/proposals", status: "proposed"},
	{typ: "quirk", template: "quirk", dir: "quirks", status: "active"},
	{typ: "reference", template: "reference", dir: "references", status: "active"},
	{typ: "ticket", template: "ticket", dir: "tickets", status: "open"},
}

// newSlugRe is kebab-case: lowercase letters and digits joined by
// single inner dashes, no leading or trailing dash.
var newSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// newPlanPrefixes mirrors the PYLON038 filename rule for plans and
// proposals; the catalog's FixHint names the same set.
var newPlanPrefixes = []string{"feature-", "refactor-", "chore-", "docs-", "perf-"}

// newPrefixRe is the decision-numbering prefix: the same shape
// config.Load validates, re-checked here because new reads the
// prefix through the warn-and-continue config path.
var newPrefixRe = regexp.MustCompile(`^[A-Z]{1,4}$`)

// isSlugName reports whether name is a usable ledger slug.
func isSlugName(name string) bool {
	return newSlugRe.MatchString(name)
}

// isPlainTitle rejects control characters and invalid UTF-8: the
// title is written into a YAML scalar, and anything outside printable
// text would corrupt the frontmatter on disk.
func isPlainTitle(title string) bool {
	if !utf8.ValidString(title) {
		return false
	}
	for _, r := range title {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func cmdNew(kbRoot, docType, name, title string, numbered bool) int {
	var spec *newDocType
	for i := range newDocTypes {
		if newDocTypes[i].typ == docType {
			spec = &newDocTypes[i]
			break
		}
	}
	if spec == nil {
		names := make([]string, 0, len(newDocTypes))
		for _, d := range newDocTypes {
			names = append(names, d.typ)
		}
		fmt.Fprintf(os.Stderr, "pylon: unknown document type %q; valid types: %s\n", docType, strings.Join(names, ", "))
		return 2
	}
	if !isSlugName(name) {
		fmt.Fprintf(os.Stderr, "pylon: the name must be kebab-case (lowercase letters, digits, dashes)\n")
		return 2
	}
	if numbered {
		if spec.typ != "decision" {
			fmt.Fprintf(os.Stderr, "pylon: --number applies to decisions only\n")
			return 2
		}
		prefix := loadConfigOrWarn(kbRoot).DecisionPrefixOrDefault()
		if !newPrefixRe.MatchString(prefix) {
			fmt.Fprintf(os.Stderr, "pylon: decision_prefix must be one to four uppercase letters, got %q\n", prefix)
			return 2
		}
		next, err := nextDecisionNumber(kbRoot, prefix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pylon: cannot scan decisions: %v\n", err)
			return 1
		}
		if next > 9999 {
			fmt.Fprintf(os.Stderr, "pylon: the %s numbering is exhausted at 9999\n", prefix)
			return 2
		}
		name = fmt.Sprintf("%s%04d-%s", prefix, next, name)
	}
	if spec.typ == "plan" {
		prefixed := false
		for _, p := range newPlanPrefixes {
			prefixed = prefixed || strings.HasPrefix(name, p)
		}
		if !prefixed {
			fmt.Fprintf(os.Stderr, "pylon: the plan name must start with feature-, refactor-, chore-, docs-, or perf- (PYLON038)\n")
			return 2
		}
	}
	data, err := readAnnotationInput(filepath.Join(kbRoot, "templates", spec.template+".md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: the %s template is unreadable (templates/ ships with the product layer): %v\n", spec.typ, err)
		return 1
	}
	base := name
	if spec.dated {
		base = time.Now().Format("2006-01-02") + "-" + name
	}
	rel := spec.dir + "/" + base + ".md"
	target := filepath.Join(kbRoot, filepath.FromSlash(rel))
	if title == "" {
		title = strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "-", " ")
	}
	if !isPlainTitle(title) {
		fmt.Fprintf(os.Stderr, "pylon: the title must be printable text without control characters\n")
		return 2
	}
	out, err := fillNewFrontMatter(string(data), spec, title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: the %s template is malformed: %v\n", spec.typ, err)
		return 2
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: cannot create %s: %v\n", filepath.Dir(rel), err)
		return 1
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			fmt.Fprintf(os.Stderr, "pylon: %s already exists; nothing was overwritten\n", rel)
			return 2
		}
		fmt.Fprintf(os.Stderr, "pylon: cannot write %s: %v\n", rel, err)
		return 1
	}
	if _, err := f.WriteString(out); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "pylon: cannot write %s: %v\n", rel, err)
		return 1
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "pylon: cannot write %s: %v\n", rel, err)
		return 1
	}
	fmt.Printf("wrote %s (%s, %s)\n", rel, spec.typ, spec.status)
	// The orphan walk skips these zone prefixes, so their docs cannot
	// be orphans and the hint would be noise.
	if orphanable := !strings.HasPrefix(spec.dir, "packages") &&
		!strings.HasPrefix(spec.dir, "conventions") &&
		!strings.HasPrefix(spec.dir, "domain"); orphanable {
		fmt.Printf("link it from another document or the FORCED orphan check (PYLON004) fails the lint\n")
	}
	return 0
}

// fillNewFrontMatter rewrites the mechanical frontmatter keys of the
// template copy: title, type, status, created, and updated. Everything
// else, body included, stays verbatim so the template's required
// headings and closed-key discipline arrive intact.
// newMissingKeyError marks a template whose frontmatter lacks one of
// the five mechanical keys; new fails closed rather than writing a
// document that cannot pass its own lint.
var newMissingKeyError = errors.New("template frontmatter is missing a mechanical key")

func fillNewFrontMatter(template string, spec *newDocType, title string) (string, error) {
	today := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(template, "---\n") {
		return "", newMissingKeyError
	}
	rest := template[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", newMissingKeyError
	}
	fm := rest[:end]
	body := rest[end+1:]
	seen := map[string]bool{"title": false, "type": false, "status": false, "created": false, "updated": false}
	var b strings.Builder
	for line := range strings.SplitSeq(fm, "\n") {
		key := ""
		if i := strings.IndexByte(line, ':'); i > 0 {
			key = strings.TrimSpace(line[:i])
		}
		dup := seen[key]
		switch key {
		case "title":
			line = "title: \"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(title) + "\""
		case "type":
			line = "type: " + spec.typ
		case "status":
			line = "status: " + spec.status
		case "created":
			line = "created: " + today
		case "updated":
			line = "updated: " + today
		}
		if _, known := seen[key]; known {
			if dup {
				return "", newMissingKeyError
			}
			seen[key] = true
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, key := range slices.Sorted(maps.Keys(seen)) {
		if !seen[key] {
			return "", fmt.Errorf("%w: %s", newMissingKeyError, key)
		}
	}
	// The template body's placeholder heading carries the title, so the
	// created document reads finished rather than half-stamped.
	body = newPlaceholderRe.ReplaceAllString(body, title)
	return "---\n" + b.String() + body, nil
}

// newPlaceholderRe matches a template body placeholder such as
// {{Quirk title}}; created documents substitute the real title.
var newPlaceholderRe = regexp.MustCompile(`\{\{[^}]*\}\}`)

// nextDecisionNumber scans decisions/ recursively for the configured
// prefix's four-digit numbers and returns the next free one, so a
// numbered proposal keeps a valid PYLON010 name when it is accepted
// and moved under decisions/.
func nextDecisionNumber(kbRoot, prefix string) (int, error) {
	next := 1
	root := evalRoot(filepath.Join(kbRoot, "decisions"))
	// The name must be exactly the PYLON010 shape <prefix><NNNN>- so a
	// six-digit file (D12345678.md) or a sign-bearing name never
	// inflates the numbering.
	numbered := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `[0-9]{4}-`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := d.Name()
		if n, err := strconv.Atoi(base[len(prefix) : len(prefix)+4]); numbered.MatchString(base) && err == nil && n+1 > next {
			next = n + 1
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}

// evalRoot follows a symlinked scan root; filepath.WalkDir does not
// follow a root symlink, so the numbering scan would read an absent
// tree while the write follows the link.
func evalRoot(root string) string {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		return resolved
	}
	return root
}
