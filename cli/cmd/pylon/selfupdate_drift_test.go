// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/install"
)

// Drift is the update working as designed, not a failure: the exit
// code must tell installers apart (0 clean, 4 applied-with-drift,
// 1 real error), so the piped installer can report drift as success
// with a review note instead of dying after the work is done.
func TestSelfUpdateDriftExitsWithReviewCode(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	write := func(root, rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(kb, "templates/one.md", "# user edition\n")
	write(kb, filepath.Join("manifests", "pylons.yaml"), "version: 1\n")
	if out, err := exec.Command("git", "init", "-q", "-b", "main", kb).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	binBody := "#!/bin/sh\necho 1.1.0\n"
	write(staged, "templates/one.md", "# shipped edition\n")
	write(staged, "bin/pylon", binBody)
	write(staged, "AGENTS.md", "# ledger rules\n")
	write(staged, "LICENSE", "Apache-2.0\n")
	write(staged, "NOTICE", "NOTICE\n")
	write(staged, "bin/AGENTS.md", "# bin\n")
	write(staged, "bin/THIRD_PARTY_NOTICES", "notices\n")
	write(staged, "extensions/AGENTS.md", "# extensions\n")
	write(staged, "manifests/rules.json", "{}\n")
	write(staged, "manifests/vocabulary.json", "{}\n")
	write(staged, "manifests/parity.json", "{}\n")
	write(staged, "manifests/extensions.example.json", "{}\n")
	write(staged, "manifests/packages.example.json", "{}\n")
	write(staged, "manifests/pylons.example.yaml", "version: 1\n")
	for _, zone := range []string{"packages", "proposals", "plans", "decisions", "conventions", "quirks", "domain", "investigations", "references", "tickets", "prompts", "skills"} {
		write(staged, zone+"/AGENTS.md", "# "+zone+"\n")
	}
	for _, keeper := range []string{"packages/archive", "plans/archive", "plans/proposals", "decisions/proposals", "decisions/archive", "conventions/proposals", "conventions/archive", "quirks/archive", "domain/entities", "domain/archive", "investigations/archive", "references/archive", "tickets/archive"} {
		write(staged, keeper+"/.gitkeep", "\n")
	}
	write(staged, "extensions/_examples/example.sh", "#!/bin/sh\n")
	write(staged, "pylons-docs/adopting.md", "# adopting\n")
	write(staged, "protocols/agent.md", "# agent\n")
	write(staged, "protocols/reflect.md", "# reflect\n")
	m, err := install.BuildProductManifest(staged, "1.1.0")
	if err != nil {
		t.Fatalf("BuildProductManifest: %v", err)
	}
	sum := sha256.Sum256([]byte(binBody))
	m.Binary = map[string]string{"linux-amd64": hex.EncodeToString(sum[:])}
	data, err := install.WriteProductManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	write(staged, filepath.Join("manifests", "product.json"), string(data))

	// runPylon sets cmd.Dir to the ledger; --from stays absolute.
	code, out, _ := runPylon(t, kb, "self-update", "--from", staged, "--pylons", ".")
	if code != 4 {
		t.Fatalf("drift exit code = %d, want 4 (applied with drift), output:\n%s", code, out)
	}
	if body, err := os.ReadFile(filepath.Join(kb, "templates", "one.md")); err != nil || string(body) != "# user edition\n" {
		t.Error("drift must keep user content")
	}
}
