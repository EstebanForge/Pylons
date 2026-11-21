// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SelfUpdate applies a staged release to an installed pylon. The
// staged directory holds the new KB skeleton (with its
// manifests/product.json) plus the new binary at bin/pylon.
//
// Three-way drift per product file: unmodified files are replaced,
// user-modified files are never overwritten (the staged copy lands
// beside them as <file>.pylons-new unless force is set), and product
// files dropped from the new release are deleted only when
// unmodified. User-layer files are never touched. Every replaced or
// removed file is backed up under _pylons-backup (the underscore
// prefix keeps backups outside every walker), and the binary is
// replaced last through an atomic rename.
type UpdateResult struct {
	Version    string
	Replaced   []string
	Drifted    []string
	Removed    []string
	KeptDirty  []string
	BackupDir  string
	DryRun     bool
	Force      bool
	BinaryNew  bool
	BinaryPath string
}

type stagedManifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func SelfUpdate(kbRoot, stagedDir string, force, dryRun bool) (UpdateResult, error) {
	var res UpdateResult
	res.Force = force
	res.DryRun = dryRun

	stagedData, err := os.ReadFile(filepath.Join(stagedDir, "manifests", "product.json"))
	if err != nil {
		return res, fmt.Errorf("staged release has no manifests/product.json: %w", err)
	}
	var newM stagedManifest
	if err := json.Unmarshal(stagedData, &newM); err != nil {
		return res, fmt.Errorf("staged manifests/product.json is unreadable: %w", err)
	}
	if newM.Version == "" || len(newM.Files) == 0 {
		return res, fmt.Errorf("staged manifests/product.json is empty")
	}
	res.Version = newM.Version

	// Fail closed before touching anything: every staged file must
	// match the release manifest.
	for rel, want := range newM.Files {
		data, err := os.ReadFile(filepath.Join(stagedDir, filepath.FromSlash(rel)))
		if err != nil {
			return res, fmt.Errorf("staged release missing %s", rel)
		}
		if sha256Hex(data) != want {
			return res, fmt.Errorf("staged file %s does not match the release manifest", rel)
		}
	}

	oldM := map[string]string{}
	if oldData, err := os.ReadFile(filepath.Join(kbRoot, "manifests", "product.json")); err == nil {
		var parsed stagedManifest
		if err := json.Unmarshal(oldData, &parsed); err != nil {
			return res, fmt.Errorf("installed manifests/product.json is unreadable: %w", err)
		}
		oldM = parsed.Files
	}

	diskHash := func(rel string) (string, bool) {
		data, err := os.ReadFile(filepath.Join(kbRoot, filepath.FromSlash(rel)))
		if err != nil {
			return "", false
		}
		return sha256Hex(data), true
	}

	names := make([]string, 0, len(newM.Files))
	for name := range newM.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	oldNames := make([]string, 0, len(oldM))
	for name := range oldM {
		oldNames = append(oldNames, name)
	}
	sort.Strings(oldNames)

	for _, rel := range names {
		if rel == "manifests/product.json" {
			continue
		}
		cur, exists := diskHash(rel)
		if !exists || cur == oldM[rel] {
			res.Replaced = append(res.Replaced, rel)
		} else if force {
			res.Replaced = append(res.Replaced, rel)
		} else {
			res.Drifted = append(res.Drifted, rel)
		}
	}
	for _, rel := range oldNames {
		if _, still := newM.Files[rel]; still {
			continue
		}
		cur, exists := diskHash(rel)
		if exists && cur == oldM[rel] {
			res.Removed = append(res.Removed, rel)
		} else {
			res.KeptDirty = append(res.KeptDirty, rel)
		}
	}

	res.BinaryPath = "bin/pylon"
	if _, err := os.Stat(filepath.Join(stagedDir, "bin", "pylon")); err == nil {
		res.BinaryNew = true
	}

	if dryRun {
		return res, nil
	}

	backupRoot := ""
	for _, rel := range res.Replaced {
		if _, exists := diskHash(rel); !exists {
			continue
		}
		if backupRoot == "" {
			backupRoot = filepath.Join(kbRoot, "_pylons-backup", oldVersionLabel(kbRoot))
			res.BackupDir, _ = filepath.Rel(kbRoot, backupRoot)
		}
		if err := backupFile(kbRoot, backupRoot, rel); err != nil {
			return res, err
		}
	}
	for _, rel := range res.Removed {
		if backupRoot == "" {
			backupRoot = filepath.Join(kbRoot, "_pylons-backup", oldVersionLabel(kbRoot))
			res.BackupDir, _ = filepath.Rel(kbRoot, backupRoot)
		}
		if err := backupFile(kbRoot, backupRoot, rel); err != nil {
			return res, err
		}
	}

	for _, rel := range res.Replaced {
		dst := filepath.Join(kbRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return res, err
		}
		src, err := os.ReadFile(filepath.Join(stagedDir, filepath.FromSlash(rel)))
		if err != nil {
			return res, err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(rel, "bin/THIRD_PARTY_NOTICES") {
			mode = 0o644
		}
		if err := atomicWrite(dst, src, mode); err != nil {
			return res, err
		}
	}
	for _, rel := range res.Drifted {
		dst := filepath.Join(kbRoot, filepath.FromSlash(rel)+".pylons-new")
		src, err := os.ReadFile(filepath.Join(stagedDir, filepath.FromSlash(rel)))
		if err != nil {
			return res, err
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			return res, err
		}
	}
	for _, rel := range res.Removed {
		if err := os.Remove(filepath.Join(kbRoot, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return res, err
		}
	}

	// The binary goes last: everything else must be in place before
	// the tool that will run the next update is swapped.
	if res.BinaryNew {
		bin, err := os.ReadFile(filepath.Join(stagedDir, "bin", "pylon"))
		if err != nil {
			return res, err
		}
		dst := filepath.Join(kbRoot, "bin", "pylon")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return res, err
		}
		if err := atomicWrite(dst+".pylons-new", bin, 0o755); err != nil {
			return res, err
		}
		if err := os.Rename(dst+".pylons-new", dst); err != nil {
			return res, err
		}
	}
	return res, nil
}

func oldVersionLabel(kbRoot string) string {
	data, err := os.ReadFile(filepath.Join(kbRoot, "manifests", "product.json"))
	if err == nil {
		var m stagedManifest
		if json.Unmarshal(data, &m) == nil && m.Version != "" {
			return m.Version
		}
	}
	return "unknown"
}

func backupFile(kbRoot, backupRoot, rel string) error {
	src := filepath.Join(kbRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dst := filepath.Join(backupRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func atomicWrite(dst string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".pylon-update-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, dst)
}
