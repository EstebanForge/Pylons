// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// isSlugName accepts kebab-case names only: lowercase letters,
// digits, and inner dashes.
func isSlugName(name string) bool {
	if len(name) == 0 || name[0] == '-' {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
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
	prefix := loadConfigOrWarn(kbRoot).DecisionPrefixOrDefault()
	if numbered {
		if spec.typ != "decision" {
			fmt.Fprintf(os.Stderr, "pylon: --number applies to decisions only\n")
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
	data, err := readAnnotationInput(filepath.Join(kbRoot, "templates", spec.template+".md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pylon: the %s template is unreadable: %v\n", spec.typ, err)
		return 2
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
	out := fillNewFrontMatter(string(data), spec, title)
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
	fmt.Printf("link it from another document or the orphan advisory (PYLON004) fires\n")
	return 0
}

// fillNewFrontMatter rewrites the mechanical frontmatter keys of the
// template copy: title, type, status, created, and updated. Everything
// else, body included, stays verbatim so the template's required
// headings and closed-key discipline arrive intact.
func fillNewFrontMatter(template string, spec *newDocType, title string) string {
	today := time.Now().Format("2006-01-02")
	if !strings.HasPrefix(template, "---\n") {
		return template
	}
	rest := template[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return template
	}
	fm := rest[:end]
	body := rest[end+1:]
	var b strings.Builder
	for _, line := range strings.Split(fm, "\n") {
		switch {
		case strings.HasPrefix(line, "title:"):
			line = "title: \"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(title) + "\""
		case strings.HasPrefix(line, "type:"):
			line = "type: " + spec.typ
		case strings.HasPrefix(line, "status:"):
			line = "status: " + spec.status
		case strings.HasPrefix(line, "created:"):
			line = "created: " + today
		case strings.HasPrefix(line, "updated:"):
			line = "updated: " + today
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return "---\n" + b.String() + body
}

// nextDecisionNumber scans decisions/ recursively for the configured
// prefix's four-digit numbers and returns the next free one, so a
// numbered proposal keeps a valid PYLON010 name when it is accepted
// and moved under decisions/.
func nextDecisionNumber(kbRoot, prefix string) (int, error) {
	next := 1
	root := filepath.Join(kbRoot, "decisions")
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
		if !strings.HasPrefix(base, prefix) || len(base) <= len(prefix)+4 {
			return nil
		}
		if n, err := strconv.Atoi(base[len(prefix) : len(prefix)+4]); err == nil && n >= next {
			next = n + 1
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}
