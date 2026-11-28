// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// pylon import adopts existing markdown into the ledger: it grafts
// the type template's frontmatter onto a verbatim body. Dry-run is
// the default because scaffolding writes ledger state; --write
// applies. Valid frontmatter skips, unparseable frontmatter refuses,
// and nothing valid is ever guessed.
func cmdImport(kbRoot string, apply bool, docType, name string, sources []string) int {
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "pylon: import requires at least one markdown file")
		return 2
	}
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
	if name != "" {
		if len(sources) > 1 {
			fmt.Fprintln(os.Stderr, "pylon: --name applies to a single source file")
			return 2
		}
		if !isSlugName(name) {
			fmt.Fprintln(os.Stderr, "pylon: the --name must be kebab-case (lowercase letters, digits, dashes)")
			return 2
		}
	}
	failures := 0
	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Printf("unreadable %s: %v\n", src, err)
			failures++
			continue
		}
		body := string(data)
		if strings.HasPrefix(body, "---\n") {
			var meta resolve.DocMeta
			if resolve.ParseDocMeta(body, &meta) && meta.Title != "" && meta.Type != "" {
				fmt.Printf("skipped %s (already carries frontmatter)\n", src)
			} else {
				fmt.Printf("refused %s (unparseable frontmatter block; fix it or strip it)\n", src)
				failures++
			}
			continue
		}
		base := name
		if base == "" {
			base = strings.TrimSuffix(filepath.Base(src), ".md")
		}
		if spec.dated {
			base = importToday() + "-" + base
		}
		if spec.typ == "plan" && !hasPlanPrefix(base) {
			fmt.Printf("refused %s (plan names need a feature-, refactor-, chore-, docs-, or perf- prefix; pass --name)\n", src)
			failures++
			continue
		}
		title := headingTitle(body)
		if title == "" || !isPlainTitle(title) {
			title = humanizeName(base)
		}
		rel := spec.dir + "/" + base + ".md"
		if !apply {
			fmt.Printf("would write %s (%s, %s) from %s\n", rel, spec.typ, spec.status, src)
			continue
		}
		tmpl, err := readAnnotationInput(filepath.Join(kbRoot, "templates", spec.template+".md"))
		if err != nil {
			fmt.Printf("refused %s (the %s template is unreadable: %v)\n", src, spec.typ, err)
			failures++
			continue
		}
		out, err := importGraft(string(tmpl), body, spec, title)
		if err != nil {
			fmt.Printf("refused %s (the %s template is malformed: %v)\n", src, spec.typ, err)
			failures++
			continue
		}
		target := filepath.Join(kbRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fmt.Printf("cannot create %s: %v\n", filepath.Dir(rel), err)
			failures++
			continue
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				fmt.Printf("refused %s (%s already exists; nothing was overwritten)\n", src, rel)
				failures++
				continue
			}
			fmt.Printf("cannot write %s: %v\n", rel, err)
			failures++
			continue
		}
		if _, err := f.WriteString(out); err != nil {
			f.Close()
			fmt.Printf("cannot write %s: %v\n", rel, err)
			failures++
			continue
		}
		if err := f.Close(); err != nil {
			fmt.Printf("cannot write %s: %v\n", rel, err)
			failures++
			continue
		}
		fmt.Printf("wrote %s (%s, %s)\n", rel, spec.typ, spec.status)
	}
	if !apply {
		fmt.Println("dry run; rerun with --write to apply")
		return 0
	}
	if failures > 0 {
		return 1
	}
	return 0
}

// importGraft keeps the template's frontmatter block and swaps its
// body for the imported document, so fillNewFrontMatter can fill the
// mechanical keys over exactly the schema the template declares.
func importGraft(template, body string, spec *newDocType, title string) (string, error) {
	rest := template[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", newMissingKeyError
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return fillNewFrontMatter(template[:4+end]+"\n---\n"+body, spec, title)
}

func importToday() string {
	return time.Now().Format("2006-01-02")
}

func headingTitle(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

func hasPlanPrefix(name string) bool {
	for _, p := range newPlanPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
