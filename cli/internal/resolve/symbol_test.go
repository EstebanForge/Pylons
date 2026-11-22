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

// PHP contract ids name namespaced symbols (Vendor\Class::method), but
// the file declares the namespace on its own line and the class bare.
// The class regex must compare the namespace against the namespace
// declaration and the short name against the declaration, or every
// namespaced id fails with "class or method not found".
func TestResolvePHPSymbolNamespacedClass(t *testing.T) {
	root := t.TempDir()
	src := "<?php\nnamespace AcmeOrg;\n\nclass OrgMan {\n\tpublic static function getInstance() { return null; }\n}\n"
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "src", "OrgMan.php"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry([]string{"php"}, "")
	v := reg.Resolve(root, Kind("php-symbol"), "src/OrgMan.php::AcmeOrg\\OrgMan::getInstance")
	if !v.Resolved {
		t.Fatalf("namespaced id must resolve: %s", v.Detail)
	}
	// A namespace the file does not declare must not resolve.
	if err := os.WriteFile(filepath.Join(root, "src", "Other.php"), []byte("<?php\nnamespace Other;\nclass OrgMan {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "src/Other.php::AcmeOrg\\OrgMan::getInstance"); v.Resolved {
		t.Fatal("a foreign namespace must not resolve")
	}
	// Multi-level namespaces split at the last separator.
	os.MkdirAll(filepath.Join(root, "deep"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "deep", "Gate.php"), []byte("<?php\nnamespace A\\B;\nclass Gate { public function open() {} }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "deep/Gate.php::A\\B\\Gate::open"); !v.Resolved {
		t.Fatalf("multi-level namespace must resolve: %s", v.Detail)
	}
	// Bare class ids keep working.
	if v := reg.Resolve(root, Kind("php-symbol"), "src/OrgMan.php::OrgMan::getInstance"); !v.Resolved {
		t.Fatalf("bare class id must keep resolving: %s", v.Detail)
	}
}
