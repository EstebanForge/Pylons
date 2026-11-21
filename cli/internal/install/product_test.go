// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProductTree materializes the minimum real product layer: the
// exact files, one file per tree, and the zone seeds.
func writeProductTree(t *testing.T, root string) {
	t.Helper()
	zoneSeeds := []string{
		"packages", "proposals", "plans", "decisions", "conventions",
		"quirks", "domain", "investigations", "audits",
		"references", "tickets", "prompts", "skills",
	}
	keepers := []string{
		"packages/archive", "plans/archive",
		"decisions/proposals", "decisions/archive",
		"conventions/proposals", "conventions/archive",
		"quirks/archive",
		"domain/entities", "domain/archive",
		"investigations/archive", "audits/archive",
		"references/archive", "tickets/archive",
	}
	write(t, filepath.Join(root, "AGENTS.md"), "# guidelines\n")
	write(t, filepath.Join(root, "LICENSE"), "apache\n")
	write(t, filepath.Join(root, "NOTICE"), "notice\n")
	write(t, filepath.Join(root, "bin", "THIRD_PARTY_NOTICES"), "third party\n")
	write(t, filepath.Join(root, "manifests", "rules.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "vocabulary.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "parity.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "extensions.example.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "packages.example.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "pylons.example.yaml"), "version: 1\n")
	write(t, filepath.Join(root, "protocols", "agent.md"), "agent\n")
	write(t, filepath.Join(root, "protocols", "reflect.md"), "reflect\n")
	write(t, filepath.Join(root, "meta", "MANIFESTO.md"), "manifesto\n")
	write(t, filepath.Join(root, "templates", "plan.md"), "plan template\n")
	write(t, filepath.Join(root, "extensions", "_examples", "pylon-ext-js"), "plugin\n")
	write(t, filepath.Join(root, "_examples", "ledger", "README.md"), "example ledger\n")
	for _, zone := range zoneSeeds {
		write(t, filepath.Join(root, zone, "AGENTS.md"), "# "+zone+"\n")
	}
	for _, k := range keepers {
		write(t, filepath.Join(root, k, ".gitkeep"), "")
	}
}

// The product layer is the exact allowlist the engine ships; a file
// outside it must never enter a product manifest, because the
// updater replaces everything the manifest lists.
func TestProductManifestIsTheAllowlist(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	// User-layer decoys: present on disk, never in the manifest.
	write(t, filepath.Join(root, "manifests", "pylons.yaml"), "version: 1\n")
	write(t, filepath.Join(root, "AGENTS.project.md"), "feel free to edit\n")
	write(t, filepath.Join(root, "plans", "mine.md"), "# mine\n")
	write(t, filepath.Join(root, "manifests", "baseline.json"), "{}\n")

	m, err := BuildProductManifest(root, "9.9.9")
	if err != nil {
		t.Fatalf("BuildProductManifest: %v", err)
	}
	if m.Version != "9.9.9" {
		t.Fatalf("Version = %q", m.Version)
	}
	for _, user := range []string{"manifests/pylons.yaml", "AGENTS.project.md", "plans/mine.md", "manifests/baseline.json"} {
		if _, ok := m.Files[user]; ok {
			t.Errorf("%s is user state and must not be in the manifest", user)
		}
	}
	for _, want := range []string{
		"AGENTS.md", "LICENSE", "NOTICE",
		"meta/MANIFESTO.md", "templates/plan.md",
		"protocols/agent.md", "protocols/reflect.md",
		"manifests/vocabulary.json", "manifests/parity.json",
		"manifests/rules.json", "manifests/pylons.example.yaml",
		"extensions/_examples/pylon-ext-js",
		"_examples/ledger/README.md",
		"packages/AGENTS.md", "plans/archive/.gitkeep", "domain/entities/.gitkeep",
		"bin/THIRD_PARTY_NOTICES",
	} {
		if _, ok := m.Files[want]; !ok {
			t.Errorf("product manifest missing %s", want)
		}
	}
	if _, ok := m.Files["manifests/product.json"]; ok {
		t.Error("product.json lists itself; it is written after the manifest is built")
	}
}

func TestProductManifestHashesMatchDisk(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	m, err := BuildProductManifest(root, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	sumHex := hex.EncodeToString(sum[:])

	if m.Files["AGENTS.md"] != sumHex {
		t.Fatalf("AGENTS.md hash %q, want %q", m.Files["AGENTS.md"], sum)
	}
}

func TestMissingProductFileFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	if err := os.Remove(filepath.Join(root, "LICENSE")); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildProductManifest(root, "1.0.0"); err == nil {
		t.Fatal("a missing product file must fail the manifest build")
	}
}

func TestProductManifestJSONRoundTrip(t *testing.T) {
	m := ProductManifest{Version: "1.2.3", Files: map[string]string{"AGENTS.md": "abc"}}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back ProductManifest
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != "1.2.3" || back.Files["AGENTS.md"] != "abc" {
		t.Fatalf("round trip = %+v", back)
	}
}
