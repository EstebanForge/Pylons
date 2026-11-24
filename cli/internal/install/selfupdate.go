// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package install

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

var windowsDriveRe = regexp.MustCompile(`^[A-Za-z]:`)

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
	Binary  map[string]string `json:"binary_sha256,omitempty"`
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
	// Error paths walk sorted keys: identical manifests must fail with
	// the same message on every run.
	stagedNames := slices.Sorted(maps.Keys(newM.Files))
	for _, rel := range stagedNames {
		if unsafeManifestPath(rel) {
			return res, fmt.Errorf("staged manifest has unsafe path %q", rel)
		}
		if !productOwnedPath(rel) {
			return res, fmt.Errorf("staged manifest key %q is outside the product layer", rel)
		}
	}

	// Fail closed before touching anything: every staged file must
	// match the release manifest.
	for _, rel := range stagedNames {
		want := newM.Files[rel]
		if st, err := os.Lstat(filepath.Join(stagedDir, filepath.FromSlash(rel))); err != nil || !st.Mode().IsRegular() {
			return res, fmt.Errorf("staged file %s is missing or not a regular file", rel)
		}
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
		for _, rel := range slices.Sorted(maps.Keys(oldM)) {
			if unsafeManifestPath(rel) {
				return res, fmt.Errorf("installed manifest has unsafe path %q", rel)
			}
		}
	}

	diskHash := func(rel string) (string, bool) {
		data, err := os.ReadFile(filepath.Join(kbRoot, filepath.FromSlash(rel)))
		if err != nil {
			return "", false
		}
		return sha256Hex(data), true
	}

	names := stagedNames
	oldNames := slices.Sorted(maps.Keys(oldM))

	for _, rel := range names {
		if rel == "manifests/product.json" {
			continue
		}
		cur, exists := diskHash(rel)
		if exists && cur == newM.Files[rel] {
			continue
		}
		if !exists || cur == oldM[rel] {
			res.Replaced = append(res.Replaced, rel)
			continue
		}
		if force {
			res.Replaced = append(res.Replaced, rel)
			continue
		}
		if newM.Files[rel] == oldM[rel] {
			// The user edited this file and upstream did not change
			// it: the edit is current, leave the file alone.
			continue
		}
		res.Drifted = append(res.Drifted, rel)
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

	binName := "pylon"
	if runtime.GOOS == "windows" {
		binName = "pylon.exe"
	}
	res.BinaryPath = "bin/" + binName
	stagedBin := filepath.Join(stagedDir, "bin", binName)
	if _, err := os.Stat(stagedBin); err == nil {
		// A hostile release cannot swap an unpinned or unverified binary:
		// the manifest must pin this platform's hash before anything is
		// applied.
		platform := runtime.GOOS + "-" + runtime.GOARCH
		pin, ok := newM.Binary[platform]
		if !ok {
			return res, fmt.Errorf("staged release does not pin a binary for %s", platform)
		}
		bin, err := os.ReadFile(stagedBin)
		if err != nil {
			return res, err
		}
		if sha256Hex(bin) != pin {
			return res, fmt.Errorf("staged binary does not match the pinned %s hash", platform)
		}
		// An identical binary is not an update: report it only when
		// the installed bytes differ or the binary is missing.
		if cur, ok := diskHash(res.BinaryPath); !ok || cur != pin {
			res.BinaryNew = true
		}
	}

	if dryRun {
		return res, nil
	}

	backupRoot := ""
	nextBackup := func() string {
		// Same-version re-releases must not overwrite earlier backups.
		label := oldVersionLabel(kbRoot)
		root := filepath.Join(kbRoot, "_pylons-backup", label)
		for n := 2; ; n++ {
			if _, err := os.Stat(root); err == nil {
				root = filepath.Join(kbRoot, "_pylons-backup", fmt.Sprintf("%s-%d", label, n))
				continue
			}
			break
		}
		return root
	}
	for _, rel := range res.Replaced {
		if _, exists := diskHash(rel); !exists {
			continue
		}
		if backupRoot == "" {
			backupRoot = nextBackup()
			res.BackupDir, _ = filepath.Rel(kbRoot, backupRoot)
		}
		if err := backupFile(kbRoot, backupRoot, rel); err != nil {
			return res, err
		}
	}
	for _, rel := range res.Removed {
		if backupRoot == "" {
			backupRoot = nextBackup()
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
		srcPath := filepath.Join(stagedDir, filepath.FromSlash(rel))
		src, err := os.ReadFile(srcPath)
		if err != nil {
			return res, err
		}
		// Keep the staged file's permission bits: the example plugins
		// ship executable and a rebuild at 0644 breaks their dispatch.
		mode := os.FileMode(0o644)
		if info, err := os.Stat(srcPath); err == nil {
			mode = info.Mode().Perm()
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

	// The new manifest is the new truth: write it so the next update
	// compares against the hashes actually installed, not the ones
	// from the release two hops ago.
	if backupRoot == "" {
		backupRoot = nextBackup()
		res.BackupDir, _ = filepath.Rel(kbRoot, backupRoot)
	}
	if _, err := os.Stat(filepath.Join(kbRoot, "manifests", "product.json")); err == nil {
		if err := backupFile(kbRoot, backupRoot, "manifests/product.json"); err != nil {
			return res, err
		}
	}
	if err := atomicWrite(filepath.Join(kbRoot, "manifests", "product.json"), stagedData, 0o644); err != nil {
		return res, err
	}

	// The binary goes last: everything else must be in place before
	// the tool that will run the next update is swapped.
	if res.BinaryNew {
		binName := "pylon"
		if runtime.GOOS == "windows" {
			binName = "pylon.exe"
		}
		bin, err := os.ReadFile(filepath.Join(stagedDir, "bin", binName))
		if err != nil {
			return res, err
		}
		dst := filepath.Join(kbRoot, "bin", binName)
		if cur, err := os.ReadFile(dst); err == nil && sha256Hex(cur) == sha256Hex(bin) {
			return res, nil
		}
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

// unsafeManifestPath reports a release-manifest key that must never
// be joined onto the KB root: absolute paths, parent segments,
// Windows drive forms, and backslashes can all escape the KB.
func unsafeManifestPath(rel string) bool {
	if rel == "" || strings.Contains(rel, "\\") {
		return true
	}
	clean := filepath.FromSlash(rel)
	if filepath.IsAbs(clean) || windowsDriveRe.MatchString(clean) {
		return true
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." || seg == "" {
			return true
		}
	}
	return false
}
