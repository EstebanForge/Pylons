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

// PHP namespaces and class names are case-insensitive, a namespace may
// share its line with the opening tag, and the braced form is legal.
func TestPHPSymbolNamespaceForms(t *testing.T) {
	cases := []struct{ name, src string }{
		{"standalone", "<?php\nnamespace Acme\\Org;\n\nclass Thing { public function ping() {} }\n"},
		{"same line as open tag", "<?php namespace Acme\\Org;\n\nclass Thing { public function ping() {} }\n"},
		{"braced", "<?php\nnamespace Acme\\Org {\n\n  class Thing { public function ping() {} }\n}\n"},
		{"case fold", "<?php\nnamespace acme\\org;\n\nclass THING { public function ping() {} }\n"},
	}
	for _, tc := range cases {
		root := t.TempDir()
		p := filepath.Join(root, "src", "Thing.php")
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(tc.src), 0o644); err != nil {
			t.Fatal(err)
		}
		reg := NewRegistry([]string{"php"}, "")
		v := reg.Resolve(root, Kind("php-symbol"), "src/Thing.php::Acme\\Org\\Thing::ping")
		if !v.Resolved {
			t.Fatalf("%s: namespaced symbol must resolve: %s", tc.name, v.Detail)
		}
	}
}

// Class-only namespaced ids (file.php::Vendor\Class) carry no method
// part: the resolver must still compare the namespace against the
// declaration instead of feeding the backslashed name to the bare
// keyword regex, which can never match it.
func TestResolvePHPSymbolClassOnlyNamespaced(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.MkdirAll(filepath.Join(root, "deep"), 0o755)
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/Schema.php", "<?php\nnamespace Acme\\Mdp;\n\nclass Schema {}\n")
	write("src/Other.php", "<?php\nnamespace Other;\nclass Schema {}\n")
	write("src/Missing.php", "<?php\nnamespace Acme\\Mdp;\nclass SomethingElse {}\n")
	write("deep/Gate.php", "<?php\nnamespace A\\B;\ntrait Gate {}\n")
	write("deep/fmt.php", "<?php\nnamespace A\\B;\nfunction fmt_date($d) { return $d; }\n")
	reg := NewRegistry([]string{"php"}, "")
	if v := reg.Resolve(root, Kind("php-symbol"), "src/Schema.php::Acme\\Mdp\\Schema"); !v.Resolved {
		t.Fatalf("class-only namespaced id must resolve: %s", v.Detail)
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "src/Other.php::Acme\\Mdp\\Schema"); v.Resolved {
		t.Fatal("a foreign namespace must not resolve for class-only ids")
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "src/Missing.php::Acme\\Mdp\\Schema"); v.Resolved {
		t.Fatal("a missing class must not resolve even under a matching namespace")
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "deep/Gate.php::A\\B\\Gate"); !v.Resolved {
		t.Fatalf("class-only trait in a multi-level namespace must resolve: %s", v.Detail)
	}
	if v := reg.Resolve(root, Kind("php-symbol"), "deep/fmt.php::A\\B\\fmt_date"); !v.Resolved {
		t.Fatalf("namespaced function id must resolve: %s", v.Detail)
	}
	// Bare class-only ids keep the plain keyword path.
	if v := reg.Resolve(root, Kind("php-symbol"), "src/Schema.php::Schema"); !v.Resolved {
		t.Fatalf("bare class-only id must keep resolving: %s", v.Detail)
	}
}
