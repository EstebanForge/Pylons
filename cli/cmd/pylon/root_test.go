package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The installed pylon always lives inside its ledger (pylons/bin/pylon),
// so when the cwd walk finds nothing the binary's own home names the
// ledger. This makes `pylons/bin/pylon brief` work from the parent
// project root, which is how the README tells users to run it.
func TestRootFromBinaryResolvesLedgerBesideBinary(t *testing.T) {
	ledger := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ledger, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "manifests", "pylons.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(ledger, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rootFromBinary(filepath.Join(binDir, "pylon")); got != ledger {
		t.Fatalf("rootFromBinary = %q, want %q", got, ledger)
	}
}

func TestRootFromBinaryRejectsNonLedgerHome(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "pylon"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rootFromBinary(filepath.Join(binDir, "pylon")); got != "" {
		t.Fatalf("rootFromBinary = %q, want empty", got)
	}
}
