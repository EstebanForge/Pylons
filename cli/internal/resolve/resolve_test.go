// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		raw    string
		scheme string
		alias  string
		branch string
		path   string
		ok     bool
	}{
		{"repo://core-api/cmd/main.go", "repo", "core-api", "", "cmd/main.go", true},
		{"repo://core-api@feat-auth/cmd/main.go", "repo", "core-api", "feat-auth", "cmd/main.go", true},
		{"pylon://plans/my-plan.md", "pylon", "", "", "plans/my-plan.md", true},
		{"repo://core-api/", "", "", "", "", false},
		{"repo://core-api/../secret.go", "", "", "", "", false},
		{"repo://core-api/cmd//main.go", "", "", "", "", false},
		{"/local/abs/path.go", "", "", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			u, ok := ParseURI(tc.raw)
			if ok != tc.ok {
				t.Errorf("ParseURI(%q) ok=%v, want %v", tc.raw, ok, tc.ok)
				return
			}
			if !ok {
				return
			}
			if u.Scheme != tc.scheme || u.Alias != tc.alias || u.Branch != tc.branch || u.Path != tc.path {
				t.Errorf("ParseURI(%q) = %+v, want scheme=%q alias=%q branch=%q path=%q", tc.raw, u, tc.scheme, tc.alias, tc.branch, tc.path)
			}
		})
	}
}

func TestCoreContractKinds(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "internal/api"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package api

func HandleHealth() {}
// router.GET("/v1/health", HandleHealth)
// cobra.Command{Use: "sync"}
// bus.Emit("vault.synced")
type VaultRecord struct{}
`
	if err := os.WriteFile(filepath.Join(proj, "internal/api/health.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry(nil, "")
	cases := []struct {
		kind Kind
		id   string
		want bool
	}{
		{KindFile, "internal/api/health.go", true},
		{KindFile, "internal/api/missing.go", false},
		{KindGlob, "internal/api/*.go", true},
		{KindGlob, "internal/api/*.rs", false},
		{KindSymbol, "internal/api/health.go::HandleHealth", true},
		{KindSymbol, "internal/api/health.go::MissingFunc", false},
		{KindRoute, "GET /v1/health", true},
		{KindRoute, "POST /v1/missing", false},
		{KindCLI, "sync", true},
		{KindEvent, "vault.synced", true},
		{KindSchema, "VaultRecord", true},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind)+"/"+tc.id, func(t *testing.T) {
			v := reg.Resolve(proj, tc.kind, tc.id)
			if v.Resolved != tc.want {
				t.Errorf("Resolve(%s, %q) resolved=%v (%s), want %v", tc.kind, tc.id, v.Resolved, v.Detail, tc.want)
			}
		})
	}
}

func TestDropInExtensionsOptIn(t *testing.T) {
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "lib.go"), []byte("package lib\nfunc Exported() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "mod.ts"), []byte("export function createSession() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Disabled by default: go-symbol and ts-export must not resolve.
	regOff := NewRegistry(nil, "")
	if v := regOff.Resolve(proj, "go-symbol", "lib.go::Exported"); v.Resolved {
		t.Fatal("expected go-symbol to fail when go extension is disabled")
	}

	// Enabled via extensions list: both resolve.
	regOn := NewRegistry([]string{"go", "typescript"}, "")
	if v := regOn.Resolve(proj, "go-symbol", "lib.go::Exported"); !v.Resolved {
		t.Fatalf("expected go-symbol to resolve when go extension enabled: %s", v.Detail)
	}
	if v := regOn.Resolve(proj, "ts-export", "mod.ts::createSession"); !v.Resolved {
		t.Fatalf("expected ts-export to resolve when typescript extension enabled: %s", v.Detail)
	}
}

func TestExternalExecutableExtension(t *testing.T) {
	proj := t.TempDir()
	extDir := t.TempDir()
	script := filepath.Join(extDir, "pylon-ext-custom")
	body := `#!/usr/bin/env sh
read -r line
case "$line" in
  *"\"id\":\"ok\""*) echo '{"resolved":true,"detail":"matched custom"}' ;;
  *) echo '{"resolved":false,"detail":"not found"}' ;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistryWithOptions(RegistryOptions{
		Extensions:    []string{"custom"},
		ExtDir:        extDir,
		AllowExternal: true,
	})
	if v := reg.Resolve(proj, "custom-kind", "ok"); !v.Resolved {
		t.Fatalf("expected external extension to resolve ok: %+v", v)
	}
	if v := reg.Resolve(proj, "custom-kind", "nope"); v.Resolved {
		t.Fatalf("expected external extension to reject nope: %+v", v)
	}

	// External extension must be blocked when AllowExternal is false.
	regBlocked := NewRegistry([]string{"custom"}, extDir)
	if v := regBlocked.Resolve(proj, "custom-kind", "ok"); v.Resolved {
		t.Fatalf("expected external extension to be blocked without AllowExternal: %+v", v)
	}
}

func TestMatchGlobAndTraversal(t *testing.T) {
	if !MatchGlob("src/**/*.go", "src/a/b/c.go") {
		t.Fatal("expected src/**/*.go to match src/a/b/c.go")
	}
	if !MatchGlob("src/**/*.go", "src/main.go") {
		t.Fatal("expected src/**/*.go to match src/main.go")
	}
	if MatchGlob("src/**/*.go", "other/main.go") {
		t.Fatal("did not expect src/**/*.go to match other/main.go")
	}
	reg := NewRegistry(nil, "")
	if v := reg.Resolve(t.TempDir(), KindFile, "../etc/passwd"); v.Resolved {
		t.Fatal("expected traversal path to fail")
	}
}

func TestListExtensionsBuiltinAndExternal(t *testing.T) {
	extDir := t.TempDir()
	script := filepath.Join(extDir, "pylon-ext-proto")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env sh\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	list := ListExtensions([]string{"go", "proto"}, extDir)
	byName := map[string]ExtensionInfo{}
	for _, item := range list {
		byName[item.Name] = item
	}

	if goExt, ok := byName["go"]; !ok || !goExt.Enabled || goExt.Source != "builtin" || len(goExt.Kinds) == 0 {
		t.Fatalf("unexpected go extension info: %+v", goExt)
	}
	if rustExt, ok := byName["rust"]; !ok || rustExt.Enabled {
		t.Fatalf("expected rust to be listed and disabled: %+v", rustExt)
	}
	if protoExt, ok := byName["proto"]; !ok || !protoExt.Enabled || protoExt.Source != "external" {
		t.Fatalf("unexpected external proto extension info: %+v", protoExt)
	}
}
