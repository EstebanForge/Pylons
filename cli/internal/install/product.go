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
	"runtime"
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
	"bin/AGENTS.md",
	"bin/THIRD_PARTY_NOTICES",
	"extensions/AGENTS.md",
	"protocols/AGENTS.md",
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
	"quirks", "domain", "investigations",
	"references", "tickets", "prompts", "skills",
}

var productKeepers = []string{
	"packages/archive", "plans/archive",
	"decisions/proposals", "decisions/archive",
	"conventions/proposals", "conventions/archive",
	"quirks/archive",
	"domain/entities", "domain/archive",
	"investigations/archive",
	"references/archive", "tickets/archive",
}

// productTrees ship whole directories: every regular file inside.
var productTrees = []string{"pylons", "templates", "extensions/_examples", "_examples"}

// ProductManifest is the release-time record of the product layer:
// one sha256 per file, consumed by self-update for three-way drift.
// Binary pins the shipped binary per platform ("os-arch", matching
// the release asset names): the updater refuses a staged binary its
// manifest does not pin.
type ProductManifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
	Binary  map[string]string `json:"binary_sha256,omitempty"`
}

// productOwnedPath reports whether rel names a path the updater may
// ever write: the product layer exactly, never user state. The staged
// manifest keys are release-controlled, so ownership is checked
// against this spec rather than trusted from the manifest.
func productOwnedPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, p := range productExactFiles {
		if rel == p {
			return true
		}
	}
	for _, z := range docZones {
		if rel == z+"/AGENTS.md" {
			return true
		}
	}
	for _, k := range productKeepers {
		if rel == k+"/.gitkeep" {
			return true
		}
	}
	for _, t := range productTrees {
		if strings.HasPrefix(rel, t+"/") {
			return true
		}
	}
	return rel == "manifests/product.json"
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
	// Pin the host build: a release manifest that cannot verify the
	// binary it ships with must not be produced. Cross-platform hashes
	// join through the manifest command's --binary flags at release time.
	if data, err := os.ReadFile(filepath.Join(root, "bin", "pylon")); err == nil {
		m.Binary = map[string]string{
			runtime.GOOS + "-" + runtime.GOARCH: sha256Hex(data),
		}
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

// PinBinary records a platform binary hash in the manifest. release.sh
// calls it once per cross build so the updater can verify every asset
// it may install.
func PinBinary(m *ProductManifest, platform, path string) error {
	if m.Binary == nil {
		m.Binary = map[string]string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256Hex(data)
	if prev, dup := m.Binary[platform]; dup {
		// The host auto-pin and the release script's explicit pin name
		// the same build twice; identical content is idempotent, a
		// conflicting hash is a release bug.
		if prev == sum {
			return nil
		}
		return fmt.Errorf("binary for %s already pinned with a different hash", platform)
	}
	m.Binary[platform] = sum
	return nil
}

// WriteProductManifest renders the manifest as JSON bytes, stable
// order, no self-entry (the file is added after it is built).
func WriteProductManifest(m ProductManifest) ([]byte, error) {
	// encoding/json sorts map keys, so the render stays stable.
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func quoteJSON(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
