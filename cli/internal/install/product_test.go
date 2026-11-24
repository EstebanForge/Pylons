// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		"quirks", "domain", "investigations",
		"references", "tickets", "prompts", "skills",
	}
	keepers := []string{
		"packages/archive", "plans/archive", "plans/proposals",
		"decisions/proposals", "decisions/archive",
		"conventions/proposals", "conventions/archive",
		"quirks/archive",
		"domain/entities", "domain/archive",
		"investigations/archive",
		"references/archive", "tickets/archive",
	}
	write(t, filepath.Join(root, "AGENTS.md"), "# guidelines\n")
	write(t, filepath.Join(root, "LICENSE"), "apache\n")
	write(t, filepath.Join(root, "NOTICE"), "notice\n")
	write(t, filepath.Join(root, "bin", "pylon"), "#!/bin/sh\necho test\n")
	write(t, filepath.Join(root, "bin", "THIRD_PARTY_NOTICES"), "third party\n")
	write(t, filepath.Join(root, "bin", "AGENTS.md"), "# bin guide\n")
	write(t, filepath.Join(root, "extensions", "AGENTS.md"), "# extensions guide\n")
	write(t, filepath.Join(root, "protocols", "AGENTS.md"), "# protocols guide\n")
	write(t, filepath.Join(root, "manifests", "rules.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "vocabulary.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "parity.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "extensions.example.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "packages.example.json"), "{}\n")
	write(t, filepath.Join(root, "manifests", "pylons.example.yaml"), "version: 1\n")
	write(t, filepath.Join(root, "protocols", "agent.md"), "agent\n")
	write(t, filepath.Join(root, "protocols", "reflect.md"), "reflect\n")
	write(t, filepath.Join(root, "templates", "plan.md"), "plan template\n")
	write(t, filepath.Join(root, "templates", "manifesto.md"), "manifesto\n")
	write(t, filepath.Join(root, "pylons-docs", "adopting.md"), "adopting\n")
	write(t, filepath.Join(root, "extensions", "_examples", "pylon-ext-js"), "plugin\n")
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
// Tree-tracked product directories follow their contents: a new file
// inside a shipped tree enters the manifest without an exact-file
// edit. protocols/ is the case that motivated this: a third protocol
// doc must ship on the next release, not wait for a code change.
func TestProductTreesTrackProtocols(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	write(t, filepath.Join(root, "protocols", "third.md"), "third protocol\n")
	m, err := BuildProductManifest(root, "1.0.0")
	if err != nil {
		t.Fatalf("BuildProductManifest: %v", err)
	}
	for _, want := range []string{"protocols/agent.md", "protocols/reflect.md", "protocols/third.md", "protocols/AGENTS.md"} {
		if _, ok := m.Files[want]; !ok {
			t.Errorf("product manifest missing %s: the protocols tree must track new files", want)
		}
	}
}

// TestRequiredTreeFilesGuard pins the release guard the reviewer's
// repro found: the tree walk alone ships a protocols/ hole, so the
// two protocol docs must fail the build when absent.
func TestRequiredTreeFilesGuard(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	for _, rel := range []string{"protocols/agent.md", "protocols/reflect.md"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
	_, err := BuildProductManifest(root, "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "protocols/agent.md") {
		t.Fatalf("missing protocol docs must fail the build naming the file, got %v", err)
	}
}

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
		"templates/plan.md",
		"protocols/agent.md", "protocols/reflect.md",
		"manifests/vocabulary.json", "manifests/parity.json",
		"manifests/rules.json", "manifests/pylons.example.yaml",
		"extensions/_examples/pylon-ext-js",
		"pylons-docs/adopting.md",
		"packages/AGENTS.md", "plans/archive/.gitkeep", "domain/entities/.gitkeep",
		"bin/THIRD_PARTY_NOTICES", "bin/AGENTS.md", "extensions/AGENTS.md", "protocols/AGENTS.md",
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

// The ledger payload no longer carries the product manual (pylons/)
// or the example ledger (_examples/): both moved to the source
// repository. A legacy-shaped file must not enter the manifest, or
// the updater would claim and overwrite a user path of that name.
func TestProductTreesDropManualAndExamples(t *testing.T) {
	root := t.TempDir()
	writeProductTree(t, root)
	write(t, filepath.Join(root, "pylons", "manual.md"), "old manual\n")
	write(t, filepath.Join(root, "_examples", "ledger", "README.md"), "old example\n")
	m, err := BuildProductManifest(root, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"pylons/manual.md", "_examples/ledger/README.md"} {
		if _, ok := m.Files[legacy]; ok {
			t.Errorf("%s left the product layer and must not ship", legacy)
		}
	}
}

// TestProductManifestCoversRealLedger builds the manifest against the
// repository's own ledger: a layout drift in pylons/ (a tree that
// moved, a file that vanished, an empty product tree) fails here and
// not only in a release build.
func TestProductManifestCoversRealLedger(t *testing.T) {
	if os.Getenv("PYLONS_SKIP_REPO_TESTS") != "" {
		t.Skip("no real ledger in this checkout")
	}
	root := repoRoot(t)
	m, err := BuildProductManifest(filepath.Join(root, "pylons"), "test")
	if err != nil {
		t.Fatalf("the real ledger must build a manifest: %v", err)
	}
	for _, rel := range []string{
		"protocols/agent.md", "protocols/reflect.md",
		"templates/plan.md", "templates/quirk.md",
		"pylons-docs/adopting.md", "pylons-docs/parity-spec.md", "pylons-docs/pylons-yaml.md",
		"extensions/_examples/pylon-ext-parity",
		"manifests/vocabulary.json", "manifests/parity.json", "manifests/rules.json",
	} {
		if _, ok := m.Files[rel]; !ok {
			t.Errorf("the real ledger's manifest misses %s", rel)
		}
	}
}
