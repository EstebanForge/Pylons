package resolve

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A ref-pinned route match must honor the same skip list as the worktree
// walk: a vendored decoy sorts before the real source, so without excludes
// git grep resolves the contract against vendor/ instead of the code.
func TestRouteRefExcludesVendoredDecoy(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+t.TempDir()+"/gitconfig", "GIT_CONFIG_SYSTEM="+t.TempDir()+"/empty")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "zz"), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := "<?php\n// vendor copy of the route\n$route = 'GET /legacy-migrate';\n"
	if err := os.WriteFile(filepath.Join(root, "vendor", "decoy.php"), []byte(decoy), 0o644); err != nil {
		t.Fatal(err)
	}
	real := "<?php\nfunction handle_migrate() {\n\t// GET /legacy-migrate\n}\n"
	if err := os.WriteFile(filepath.Join(root, "zz", "real.php"), []byte(real), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-f", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "seed")

	reg := NewRegistryWithOptions(RegistryOptions{Extensions: []string{"php"}, Ref: "HEAD"})
	v := reg.Resolve(root, "route", "GET /legacy-migrate")
	if !v.Resolved {
		t.Fatalf("route did not resolve under ref: %+v", v)
	}
	if v.Detail != "zz/real.php" {
		t.Fatalf("route resolved against %q, want zz/real.php (vendored decoy must be excluded)", v.Detail)
	}
}
