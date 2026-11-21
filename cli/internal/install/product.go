// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The product layer is everything Pylons ships and may replace on
// update. Everything else in an installed pylon is user state and
// the updater never touches it: zone documents, manifests/pylons.yaml
// and the generated manifests users own (baseline, ratchet, packages),
// AGENTS.project.md, user extensions, .git, .githooks content.
//
// The manifest is an allowlist built from this spec, never from a
// tree walk, so a stray dogfood file can never ride into a release
// and be overwritten on user machines.

var productExactFiles = []string{
	"AGENTS.md", "LICENSE", "NOTICE",
	"bin/THIRD_PARTY_NOTICES",
	"manifests/rules.json",
	"manifests/vocabulary.json",
	"manifests/parity.json",
	"manifests/extensions.example.json",
	"manifests/packages.example.json",
	"manifests/pylons.example.yaml",
	"protocols/agent.md",
	"protocols/reflect.md",
}

// docZones is every top-level zone that ships an AGENTS.md entry
// point; archive keepers keep empty directories alive in clones.
var docZones = []string{
	"packages", "proposals", "plans", "decisions", "conventions",
	"quirks", "flows", "domain", "investigations", "audits",
	"references", "tickets", "prompts", "skills",
}

var productKeepers = []string{
	"packages/archive", "plans/archive",
	"decisions/proposals", "decisions/archive",
	"conventions/proposals", "conventions/archive",
	"quirks/archive", "flows/archive",
	"domain/entities", "domain/archive",
	"investigations/archive", "audits/archive",
	"references/archive", "tickets/archive",
}

// productTrees ship whole directories: every regular file inside.
var productTrees = []string{"meta", "templates", "extensions/_examples"}

// ProductManifest is the release-time record of the product layer:
// one sha256 per file, consumed by self-update for three-way drift.
type ProductManifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// BuildProductManifest hashes the product layer of a checkout. Any
// missing exact file or empty tree fails the build: a release with a
// hole in the product layer must never ship.
func BuildProductManifest(root, version string) (ProductManifest, error) {
	m := ProductManifest{Version: version, Files: map[string]string{}}
	var wanted []string
	wanted = append(wanted, productExactFiles...)
	for _, zone := range docZones {
		wanted = append(wanted, filepath.ToSlash(filepath.Join(zone, "AGENTS.md")))
	}
	for _, k := range productKeepers {
		wanted = append(wanted, filepath.ToSlash(filepath.Join(k, ".gitkeep")))
	}
	for _, rel := range wanted {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return m, fmt.Errorf("product layer missing %s", rel)
		}
		m.Files[rel] = sha256Hex(data)
	}
	for _, tree := range productTrees {
		base := filepath.Join(root, filepath.FromSlash(tree))
		count := 0
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, relErr := filepath.Rel(root, p)
			if relErr != nil {
				return relErr
			}
			data, readErr := os.ReadFile(p)
			if readErr != nil {
				return readErr
			}
			m.Files[filepath.ToSlash(rel)] = sha256Hex(data)
			count++
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return m, err
		}
		if count == 0 {
			return m, fmt.Errorf("product tree %s is empty", tree)
		}
	}
	return m, nil
}

// WriteProductManifest renders the manifest as JSON bytes, stable
// order, no self-entry (the file is added after it is built).
func WriteProductManifest(m ProductManifest) ([]byte, error) {
	names := make([]string, 0, len(m.Files))
	for name := range m.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("{\n  \"version\": " + quoteJSON(m.Version) + ",\n  \"files\": {\n")
	for i, name := range names {
		b.WriteString("    " + quoteJSON(name) + ": " + quoteJSON(m.Files[name]))
		if i < len(names)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n}\n")
	return []byte(b.String()), nil
}

func quoteJSON(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
