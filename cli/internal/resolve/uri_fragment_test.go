// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import "testing"

// Contract ids carry symbol fragments after a double colon. The fragment
// may name PHP-namespaced symbols, so backslashes are legal there even
// though the path portion keeps rejecting them.
func TestParseURISymbolFragmentAllowsNamespaces(t *testing.T) {
	u, ok := ParseURI("repo://svc/src/OrgMan.php::AcmeOrg\\OrgMan::getInstance")
	if !ok {
		t.Fatal("namespaced fragment must parse")
	}
	if u.Path != "src/OrgMan.php" {
		t.Fatalf("Path must drop the fragment, got %q", u.Path)
	}
	if u.Alias != "svc" {
		t.Fatalf("alias must survive the fragment, got %q", u.Alias)
	}
}

func TestParseURIBranchWithFragment(t *testing.T) {
	u, ok := ParseURI("repo://svc@fix/src/x.php::Vendor\\Thing::do")
	if !ok {
		t.Fatal("branch plus fragment must parse")
	}
	if u.Branch != "fix" || u.Path != "src/x.php" {
		t.Fatalf("branch=%q path=%q", u.Branch, u.Path)
	}
}

func TestParseURIPlainPathsUnchanged(t *testing.T) {
	if u, ok := ParseURI("repo://svc/src/a.php"); !ok || u.Path != "src/a.php" {
		t.Fatalf("plain repo uri broken: ok=%v path=%q", ok, u.Path)
	}
	if u, ok := ParseURI("pylon://domain/x.md"); !ok || u.Path != "domain/x.md" {
		t.Fatalf("plain pylon uri broken: ok=%v path=%q", ok, u.Path)
	}
	// Path traversal in the path portion stays rejected even when a
	// fragment follows.
	if _, ok := ParseURI("repo://svc/../evil.php::Sym"); ok {
		t.Fatal("traversal under a fragment must stay rejected")
	}
	// A backslash in the PATH portion (not the fragment) stays rejected.
	if _, ok := ParseURI("repo://svc/src\\x.php"); ok {
		t.Fatal("backslash in the path portion must stay rejected")
	}
}
