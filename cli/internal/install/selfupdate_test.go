// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildInstalled materializes an installed pylon at version with its
// product manifest.
func buildInstalled(t *testing.T, root, version string) {
	t.Helper()
	writeProductTree(t, root)
	writeProductManifestFor(t, root, version)
}

// buildStaged materializes a staged release; mutations run before the
// manifest is built so the manifest hashes the mutated tree.
func buildStaged(t *testing.T, dir, version string, mutate func(dir string)) {
	t.Helper()
	writeProductTree(t, dir)
	if mutate != nil {
		mutate(dir)
	}
	// The binary is written before the manifest: the manifest now pins
	// its hash, so the staged tree must be final before it is hashed.
	write(t, filepath.Join(dir, "bin", "pylon"), "#!/bin/sh\necho "+version+"\n")
	writeProductManifestFor(t, dir, version)
}

func writeProductManifestFor(t *testing.T, root, version string) {
	t.Helper()
	m, err := BuildProductManifest(root, version)
	if err != nil {
		t.Fatalf("BuildProductManifest: %v", err)
	}
	data, err := WriteProductManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "manifests", "product.json"), string(data))
}

func readInstalled(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func requireNoFile(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist", rel)
	}
}

func TestSelfUpdateReplacesUnmodifiedAndKeepsUserData(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	write(t, filepath.Join(kb, "plans", "mine.md"), "# my plan\n")
	write(t, filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "MANIFESTO.md"), "manifesto v2\n")
	})

	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if res.Version != "1.1.0" {
		t.Fatalf("Version = %q", res.Version)
	}
	if readInstalled(t, kb, "pylons/MANIFESTO.md") != "manifesto v2\n" {
		t.Error("unmodified product file must be replaced")
	}
	if readInstalled(t, kb, "plans/mine.md") != "# my plan\n" {
		t.Error("user zone doc must be untouched")
	}
	if readInstalled(t, kb, "manifests/pylons.yaml") != "version: 1\n" {
		t.Error("pylons.yaml must be untouched")
	}
	if res.BinaryNew {
		bin := readInstalled(t, kb, "bin/pylon")
		if bin != "#!/bin/sh\necho 1.1.0\n" {
			t.Errorf("binary = %q, want the staged binary", bin)
		}
	} else {
		t.Error("the staged binary must be applied")
	}
	if res.BackupDir == "" {
		t.Error("a backup directory must be recorded")
	}
}

func TestSelfUpdateDriftWritesNewBesideAndForceOverwrites(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	write(t, filepath.Join(kb, "AGENTS.md"), "# my own rules\n")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		write(t, filepath.Join(dir, "AGENTS.md"), "# shipped rules\n")
	})

	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if len(res.Drifted) != 1 || res.Drifted[0] != "AGENTS.md" {
		t.Fatalf("Drifted = %v, want AGENTS.md", res.Drifted)
	}
	if readInstalled(t, kb, "AGENTS.md") != "# my own rules\n" {
		t.Error("a drifted file must keep the user content")
	}
	if readInstalled(t, kb, "AGENTS.md.pylons-new") != "# shipped rules\n" {
		t.Error("the staged copy must land as AGENTS.md.pylons-new")
	}

	res2, err := SelfUpdate(kb, staged, true, false)
	if err != nil {
		t.Fatalf("SelfUpdate force: %v", err)
	}
	if len(res2.Drifted) != 0 {
		t.Fatalf("forced Drifted = %v, want empty", res2.Drifted)
	}
	if readInstalled(t, kb, "AGENTS.md") != "# shipped rules\n" {
		t.Error("force must replace the drifted file")
	}
}

func TestSelfUpdateRemovesDroppedFilesSafely(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	// A product file the new release drops: pylons/OLD.md is on disk
	// and in the old manifest only if the old build carried it.
	write(t, filepath.Join(kb, "pylons", "OLD.md"), "old doc\n")
	// Rebuild the old manifest to include it by staging it first.
	buildStaged(t, staged, "1.0.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "OLD.md"), "old doc\n")
	})
	// Overwrite the installed manifest with one that knows OLD.md.
	m, err := BuildProductManifest(staged, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	installedBytes, err := WriteProductManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(kb, "manifests", "product.json"), string(installedBytes))

	// New release without OLD.md.
	buildStaged(t, staged, "1.1.0", func(dir string) {
		if err := os.Remove(filepath.Join(dir, "pylons", "OLD.md")); err != nil {
			t.Fatal(err)
		}
	})
	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "pylons/OLD.md" {
		t.Fatalf("Removed = %v, want pylons/OLD.md", res.Removed)
	}
	requireNoFile(t, kb, "pylons/OLD.md")

	// A user-modified dropped file is kept.
	write(t, filepath.Join(kb, "pylons", "OLD.md"), "user edited old doc\n")
	write(t, filepath.Join(kb, "manifests", "product.json"), string(installedBytes))
	res2, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.KeptDirty) != 1 || res2.KeptDirty[0] != "pylons/OLD.md" {
		t.Fatalf("KeptDirty = %v", res2.KeptDirty)
	}
	if readInstalled(t, kb, "pylons/OLD.md") != "user edited old doc\n" {
		t.Error("a user-modified dropped file must be kept")
	}
}

func TestSelfUpdateFailsClosedOnStagedMismatch(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	buildStaged(t, staged, "1.1.0", nil)
	// Corrupt one staged file after its manifest was built.
	write(t, filepath.Join(staged, "pylons", "MANIFESTO.md"), "tampered\n")

	if _, err := SelfUpdate(kb, staged, false, false); err == nil {
		t.Fatal("a staged file that contradicts the manifest must fail the update")
	}
	// Nothing was touched.
	if readInstalled(t, kb, "pylons/MANIFESTO.md") != "manifesto\n" {
		t.Error("the installed tree must be untouched after a failed verification")
	}
}

func TestSelfUpdateLegacyInstallNeverSilentlyOverwrites(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	// No manifests/product.json: a pre-manifest install. The user has
	// edited a product file, and the identical rest must be skipped.
	writeProductTree(t, kb)
	write(t, filepath.Join(kb, "AGENTS.md"), "# my own rules\n")
	buildStaged(t, staged, "1.1.0", nil)

	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate legacy: %v", err)
	}
	if len(res.Drifted) != 1 || res.Drifted[0] != "AGENTS.md" {
		t.Fatalf("Drifted = %v; a legacy edit must be treated as drift", res.Drifted)
	}
	if readInstalled(t, kb, "AGENTS.md") != "# my own rules\n" {
		t.Error("legacy drift must keep user content")
	}
}

func TestSelfUpdateDryRunTouchesNothing(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "MANIFESTO.md"), "manifesto v2\n")
	})
	res, err := SelfUpdate(kb, staged, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || len(res.Replaced) == 0 {
		t.Fatalf("dry-run plan = %+v", res)
	}
	if readInstalled(t, kb, "pylons/MANIFESTO.md") != "manifesto\n" {
		t.Error("a dry run must not write")
	}
	requireNoFile(t, kb, "_pylons-backup")
}

func TestSelfUpdateWritesNewProductManifest(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "MANIFESTO.md"), "manifesto v2\n")
	})

	if _, err := SelfUpdate(kb, staged, false, false); err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if !strings.Contains(readInstalled(t, kb, "manifests/product.json"), "1.1.0") {
		t.Error("the installed product manifest must carry the new version")
	}
}

func TestSelfUpdateSecondHopHasNoFalseDrift(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "MANIFESTO.md"), "manifesto v2\n")
	})
	if _, err := SelfUpdate(kb, staged, false, false); err != nil {
		t.Fatalf("first hop: %v", err)
	}

	buildStaged(t, staged, "1.2.0", func(dir string) {
		write(t, filepath.Join(dir, "pylons", "MANIFESTO.md"), "manifesto v3\n")
	})
	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("second hop: %v", err)
	}
	if len(res.Drifted) != 0 {
		t.Fatalf("second hop Drifted = %v; files the first hop replaced must not report drift", res.Drifted)
	}
	if readInstalled(t, kb, "pylons/MANIFESTO.md") != "manifesto v3\n" {
		t.Error("the second hop must apply its staged content")
	}
}

func TestSelfUpdateSkipsIdenticalContent(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	buildStaged(t, staged, "1.1.0", nil)

	res, err := SelfUpdate(kb, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if len(res.Replaced) != 0 {
		t.Fatalf("Replaced = %v; identical content must not be rewritten", res.Replaced)
	}
	// The manifest is the one file that always changes: its backup is
	// the only one the hop needs.
	if _, err := os.Stat(filepath.Join(kb, "_pylons-backup", "1.0.0", "manifests", "product.json")); err != nil {
		t.Errorf("the old manifest must be backed up: %v", err)
	}
	if !strings.Contains(readInstalled(t, kb, "manifests/product.json"), "1.1.0") {
		t.Error("the manifest must still move to the new version")
	}
}

func craftManifest(t *testing.T, dir, version string, files map[string]string) {
	t.Helper()
	data, err := json.Marshal(stagedManifest{Version: version, Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "manifests", "product.json"), string(data))
}

func TestSelfUpdateRejectsUnsafeStagedManifestKeys(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	evil := filepath.Join(filepath.Dir(staged), "evil.txt")
	write(t, evil, "payload")
	data, err := os.ReadFile(evil)
	if err != nil {
		t.Fatal(err)
	}
	craftManifest(t, staged, "9.9.9", map[string]string{"../evil.txt": sha256Hex(data)})
	writeProductTree(t, staged)

	if _, err := SelfUpdate(kb, staged, false, false); err == nil {
		t.Fatal("a ../ manifest key must be rejected")
	}
	requireNoFile(t, kb, "evil.txt")
	if _, err := os.Stat(evil); err != nil {
		t.Errorf("the escape target itself must be untouched: %v", err)
	}
}

func TestSelfUpdateRejectsUnsafeInstalledManifestKeys(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	evil := filepath.Join(filepath.Dir(kb), "outside.txt")
	write(t, evil, "keep me")
	data, err := os.ReadFile(evil)
	if err != nil {
		t.Fatal(err)
	}
	writeProductTree(t, kb)
	craftManifest(t, kb, "1.0.0", map[string]string{"../outside.txt": sha256Hex(data)})
	buildStaged(t, staged, "1.1.0", nil)

	if _, err := SelfUpdate(kb, staged, false, false); err == nil {
		t.Fatal("an installed manifest with a ../ key must be rejected")
	}
	if _, err := os.Stat(evil); err != nil {
		t.Errorf("the removal-list escape target must survive: %v", err)
	}
}

func writeStagedManifest(t *testing.T, dir, version string, files map[string]string, binaries map[string]string) {
	t.Helper()
	data, err := json.Marshal(stagedManifest{Version: version, Files: files, Binary: binaries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "manifests", "product.json"), string(data))
}

func stagedFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	write(t, filepath.Join(dir, filepath.FromSlash(rel)), content)
	return sha256Hex([]byte(content))
}

// A staged release may only write product-layer paths. A hash-matching
// key naming .githooks/ or any user file is a hostile release and must
// be rejected before anything is touched.
func TestSelfUpdateRejectsForeignProductKeys(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	sha := stagedFile(t, staged, ".githooks/pre-commit", "#!/bin/sh\ncurl evil\n")
	writeProductTree(t, staged)
	writeStagedManifest(t, staged, "9.9.9", map[string]string{
		"pylons/MANIFESTO.md":  sha256Hex([]byte("manifesto\n")),
		".githooks/pre-commit": sha,
	}, nil)

	if _, err := SelfUpdate(kb, staged, false, false); err == nil || !strings.Contains(err.Error(), "outside the product layer") {
		t.Fatalf("a foreign key must be rejected, got: %v", err)
	}
	requireNoFile(t, kb, ".githooks/pre-commit")
}

// The staged binary is pinned by its platform hash: a manifest without
// the entry, or with a mismatched one, must fail closed before apply.
func TestSelfUpdatePinsTheStagedBinary(t *testing.T) {
	platform := runtime.GOOS + "-" + runtime.GOARCH
	files := map[string]string{"pylons/MANIFESTO.md": sha256Hex([]byte("manifesto\n"))}

	t.Run("missing entry", func(t *testing.T) {
		kb, staged := t.TempDir(), t.TempDir()
		buildInstalled(t, kb, "1.0.0")
		writeProductTree(t, staged)
		write(t, filepath.Join(staged, "bin", "pylon"), "#!/bin/sh\necho hostile\n")
		writeStagedManifest(t, staged, "1.1.0", files, nil)
		if _, err := SelfUpdate(kb, staged, false, false); err == nil || !strings.Contains(err.Error(), "does not pin a binary") {
			t.Fatalf("a staged binary without a pin must fail closed, got: %v", err)
		}
	})

	t.Run("mismatched hash", func(t *testing.T) {
		kb, staged := t.TempDir(), t.TempDir()
		buildInstalled(t, kb, "1.0.0")
		writeProductTree(t, staged)
		write(t, filepath.Join(staged, "bin", "pylon"), "#!/bin/sh\necho hostile\n")
		writeStagedManifest(t, staged, "1.1.0", files, map[string]string{platform: "deadbeef"})
		if _, err := SelfUpdate(kb, staged, false, false); err == nil || !strings.Contains(err.Error(), "does not match the pinned") {
			t.Fatalf("a mismatched binary pin must fail closed, got: %v", err)
		}
	})

	t.Run("matching hash applies", func(t *testing.T) {
		kb, staged := t.TempDir(), t.TempDir()
		buildInstalled(t, kb, "1.0.0")
		writeProductTree(t, staged)
		sha := stagedFile(t, staged, "bin/pylon", "#!/bin/sh\necho 1.1.0\n")
		writeStagedManifest(t, staged, "1.1.0", files, map[string]string{platform: sha})
		res, err := SelfUpdate(kb, staged, false, false)
		if err != nil {
			t.Fatalf("SelfUpdate: %v", err)
		}
		if !res.BinaryNew {
			t.Fatal("a pinned binary must swap")
		}
		if readInstalled(t, kb, "bin/pylon") != "#!/bin/sh\necho 1.1.0\n" {
			t.Error("the verified binary must be installed")
		}
	})
}

// A same-version re-release must not overwrite the previous run's
// backups: each apply gets its own directory.
func TestSelfUpdateBackupDirDedupesSameVersion(t *testing.T) {
	kb := t.TempDir()
	buildInstalled(t, kb, "1.1.0")
	write(t, filepath.Join(kb, "manifests", "pylons.yaml"), "version: 1\n")

	stagedA := t.TempDir()
	writeProductTree(t, stagedA)
	write(t, filepath.Join(stagedA, "pylons", "MANIFESTO.md"), "manifesto re-release A\n")
	writeProductManifestFor(t, stagedA, "1.1.0")
	resA, err := SelfUpdate(kb, stagedA, false, false)
	if err != nil {
		t.Fatalf("first update: %v", err)
	}

	stagedB := t.TempDir()
	writeProductTree(t, stagedB)
	write(t, filepath.Join(stagedB, "pylons", "MANIFESTO.md"), "manifesto re-release B\n")
	writeProductManifestFor(t, stagedB, "1.1.0")
	resB, err := SelfUpdate(kb, stagedB, false, false)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if resB.BackupDir == resA.BackupDir {
		t.Fatalf("a same-version re-run must get its own backup dir, both %q", resB.BackupDir)
	}
}

// A staged tree may not smuggle a symlink: the hash pins content, but
// the write must land as a real file inside the KB, not through a link.
func TestSelfUpdateRejectsSymlinkedStagedFile(t *testing.T) {
	kb, staged := t.TempDir(), t.TempDir()
	buildInstalled(t, kb, "1.0.0")
	writeProductTree(t, staged)
	outside := filepath.Join(t.TempDir(), "real-manifesto.md")
	if err := os.WriteFile(outside, []byte("manifesto v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(staged, "pylons", "MANIFESTO.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(staged, "pylons", "MANIFESTO.md")); err != nil {
		t.Fatal(err)
	}
	// Hand-crafted manifest: BuildProductManifest refuses to hash a
	// symlinked key, so the hostile case is exactly a release that
	// lists one by hand.
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	writeStagedManifest(t, staged, "1.1.0", map[string]string{
		"pylons/MANIFESTO.md": sha256Hex(data),
	}, nil)

	if _, err := SelfUpdate(kb, staged, false, false); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a symlinked staged file must fail closed, got: %v", err)
	}
	requireNoFile(t, kb, "pylons/MANIFESTO.md.pylons-new")
}
