// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// a/** covers a's contents but never the bare directory itself.
func TestMatchGlobTrailingDoubleStar(t *testing.T) {
	cases := []struct {
		pat, tgt string
		want     bool
	}{
		{"a/**", "a", false},
		{"a/**", "a/b", true},
		{"a/**", "a/b/c", true},
		{"**", "x/y", true},
		{"**/x", "x", true},
		{"x/**/y", "x/y", true},
		{"x/**/y", "x/m/y", true},
		{"a/b/**", "a/b", false},
		{"cmd/**", "cmd/app.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.pat+"|"+tc.tgt, func(t *testing.T) {
			if got := MatchGlob(tc.pat, tc.tgt); got != tc.want {
				t.Fatalf("MatchGlob(%q, %q) = %v, want %v", tc.pat, tc.tgt, got, tc.want)
			}
		})
	}
}

// A symlink inside the project that points at a device must never be read.
func TestGrepProjectSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.go"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(root, "evil.go")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if got := grepProject(root, "needle"); got != "real.go" {
		t.Fatalf("grepProject = %q, want real.go", got)
	}
}

// Frontmatter fences must survive a BOM and CRLF line endings.
func TestFrontMatterCRLFAndBOM(t *testing.T) {
	content := "\ufeff---\r\ntitle: Auth\r\ntype: plan\r\nstatus: accepted\r\n---\r\n\r\n# Auth\r\n"
	var meta DocMeta
	if !ParseDocMeta(content, &meta) {
		t.Fatal("CRLF+BOM frontmatter did not parse")
	}
	if meta.Title != "Auth" || meta.Type != "plan" {
		t.Fatalf("meta = %+v", meta)
	}
	body := WithoutFrontMatter(content)
	if body == content {
		t.Fatal("frontmatter was not stripped")
	}
}

// A configured ref that does not resolve must fail closed, while a
// worktree-only project (no .git) keeps working.
func TestRefFailClosed(t *testing.T) {
	gitRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(gitRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if msg := refErr(gitRoot, "nope"); msg == "" {
		t.Fatal("missing ref in a git repo must fail closed")
	}
	if msg := refErr(gitRoot, "-dangerous"); msg == "" {
		t.Fatal("dash ref must always be rejected")
	}
	plain := t.TempDir()
	if msg := refErr(plain, "main"); msg != "" {
		t.Fatalf("non-git root must stay on the worktree: %v", msg)
	}
}

// URI safety: dot segments, trailing slashes, control chars, drive letters.
func TestIsSafeRelPathHardened(t *testing.T) {
	bad := []string{"a/./b", "a/", "a\x00b", "a/b\x1b", "C:/x", "./x", ".."}
	good := []string{"a/b.go", "cmd/main.go", "a_b/c-d.txt"}
	for _, p := range bad {
		if isSafeRelPath(p) {
			t.Fatalf("isSafeRelPath(%q) = true, want false", p)
		}
	}
	for _, p := range good {
		if !isSafeRelPath(p) {
			t.Fatalf("isSafeRelPath(%q) = false, want true", p)
		}
	}
}

// A contract kind with no extension namespace must never pick a binary.
func TestExternalExtensionRequiresNamespace(t *testing.T) {
	reg := NewRegistryWithOptions(RegistryOptions{Extensions: []string{"php-symbol"}})
	v, handled := reg.tryExternalExtension(t.TempDir(), Kind("nospace"), "whatever")
	if !handled {
		t.Fatal("kind without a dash must be handled (refused), not fall through")
	}
	if v.Resolved {
		t.Fatal("kind without a dash must not resolve")
	}
}

// wp-hook ships natively under the php language extension: the id is
// the hook tag, resolved when a bounded project PHP file registers,
// fires, removes, or queries the tag through the standard hook
// functions. Without the php extension the kind does not exist.
func TestWPHookNativeThroughPhpExtension(t *testing.T) {
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, "src"), 0o755)
	src := "<?php\nadd_action('acme/module/ready', 'boot');\ndo_action( \"acme/other\" );\n"
	if err := os.WriteFile(filepath.Join(proj, "src", "boot.php"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("add_action('acme/in-txt')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry([]string{"php"}, "")
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/module/ready"); !v.Resolved {
		t.Fatalf("registered hook must resolve: %s", v.Detail)
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/other"); !v.Resolved {
		t.Fatalf("double-quoted spaced hook must resolve: %s", v.Detail)
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/never-wired"); v.Resolved {
		t.Fatal("unregistered hook must not resolve")
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/in-txt"); v.Resolved {
		t.Fatal("a non-PHP file must never satisfy a wp-hook contract")
	}
	plain := NewRegistry([]string{}, "")
	if v := plain.Resolve(proj, Kind("wp-hook"), "acme/module/ready"); v.Resolved {
		t.Fatal("wp-hook must not exist without the php extension")
	}
}

// The git-grep ref path must speak POSIX ERE: the review found the
// shared regex carried RE2-only syntax, so every hook died as an
// unmatched grep under a configured ref. The ref path must also keep
// the worktree path's file scoping, and the worktree path must follow
// a symlinked project root.
func TestWPHookRefPathResolves(t *testing.T) {
	proj := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = proj
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(proj, "boot.php"), []byte("<?php\nadd_action('acme/ready', 'b');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A markdown file mentioning the tag must never satisfy the
	// contract on the ref path, mirroring the worktree walk's .php
	// scoping.
	if err := os.WriteFile(filepath.Join(proj, "notes.md"), []byte("add_action('acme/mdtag')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "vendor", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "vendor", "x", "v.php"), []byte("<?php\nadd_filter('acme/vendortag', 'v');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "seed")
	reg := NewRegistryWithOptions(RegistryOptions{Extensions: []string{"php"}, Ref: "HEAD"})
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/ready"); !v.Resolved {
		t.Fatalf("hook must resolve through the configured ref: %s", v.Detail)
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/mdtag"); v.Resolved {
		t.Fatal("a non-PHP file must never satisfy a wp-hook contract on the ref path")
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/vendortag"); v.Resolved {
		t.Fatal("vendor must be skipped on the ref path like the worktree walk")
	}
	if v := reg.Resolve(proj, Kind("wp-hook"), "acme/ref-array"); v.Resolved {
		t.Fatal("unset fixture leak")
	}
}

// A symlinked project root must resolve: WalkDir does not follow a
// root symlink, so the whole tree went dark.
func TestWPHookSymlinkedRootResolves(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.WriteFile(filepath.Join(real, "boot.php"), []byte("<?php\nadd_action('acme/linktag', 'b');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry([]string{"php"}, "")
	if v := reg.Resolve(link, Kind("wp-hook"), "acme/linktag"); !v.Resolved {
		t.Fatalf("symlinked root must resolve: %s", v.Detail)
	}
}

// New family forms ship with the kind: ref-array, deprecated, and the
// remove_all pair.
func TestWPHookFamilyForms(t *testing.T) {
	proj := t.TempDir()
	src := "<?php\ndo_action_ref_array('acme/a', [$x]);\napply_filters_deprecated('acme/b', [], '1.0');\nremove_all_actions('acme/c');\n"
	if err := os.WriteFile(filepath.Join(proj, "boot.php"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry([]string{"php"}, "")
	for _, tag := range []string{"acme/a", "acme/b", "acme/c"} {
		if v := reg.Resolve(proj, Kind("wp-hook"), tag); !v.Resolved {
			t.Fatalf("%s must resolve: %s", tag, v.Detail)
		}
	}
}
