// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package protocols

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/estebanforge/pylons/cli/templates"
)

// ProjectInfo summarizes a registered project for protocol template rendering.
type ProjectInfo struct {
	Alias string
	Ref   string
	Tier  string
}

// Data holds workspace context injected into protocol templates.
type Data struct {
	Root       string
	Bin        string
	Extensions []string
	Projects   []ProjectInfo
}

var funcMap = template.FuncMap{
	"join": strings.Join,
}

// List returns sorted names of protocols from <kbRoot>/protocols/ and embedded defaults.
func List(kbRoot string) []string {
	seen := make(map[string]bool)
	if entries, err := templates.FS.ReadDir("."); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				seen[strings.TrimSuffix(e.Name(), ".md")] = true
			}
		}
	}
	if kbRoot != "" {
		if entries, err := os.ReadDir(filepath.Join(kbRoot, "protocols")); err == nil {
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "README.md" || name == "AGENTS.md" {
					continue
				}
				seen[strings.TrimSuffix(name, ".md")] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// Render renders name using overridePath, then <data.Root>/protocols/<name>.md, then embedded defaults.
func Render(name string, data Data, overridePath string) (string, error) {
	if data.Bin == "" {
		data.Bin = "pylon"
	}
	var raw []byte
	var err error
	switch {
	case overridePath != "":
		raw, err = os.ReadFile(overridePath)
		if err != nil {
			return "", fmt.Errorf("read protocol override %s: %w", overridePath, err)
		}
	case name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, ".."):
		return "", fmt.Errorf("invalid protocol name %q", name)
	case data.Root != "":
		kbProto := filepath.Join(data.Root, "protocols", name+".md")
		if b, readErr := os.ReadFile(kbProto); readErr == nil {
			raw = b
		}
	}
	if raw == nil {
		raw, err = templates.FS.ReadFile(name + ".md")
		if err != nil {
			return "", fmt.Errorf("unknown protocol %q", name)
		}
	}

	tmpl, err := template.New(name).Funcs(funcMap).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse protocol template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute protocol template %q: %w", name, err)
	}
	return buf.String(), nil
}
