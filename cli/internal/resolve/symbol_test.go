// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

// A symbol must not resolve when it only appears as a substring of a
// longer identifier; bare substring matching produced false contracts.
func TestResolveSymbolWordBoundary(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "svc.go")
	if err := os.WriteFile(f, []byte("package svc\n\nfunc GetString() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistryWithOptions(RegistryOptions{})
	if v := reg.Resolve(root, KindSymbol, "svc.go::Get"); v.Resolved {
		t.Fatal("Get must not resolve while only GetString exists")
	}
	if err := os.WriteFile(f, []byte("package svc\n\nfunc Get() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := reg.Resolve(root, KindSymbol, "svc.go::Get"); !v.Resolved {
		t.Fatalf("Get should resolve: %s", v.Detail)
	}
}
