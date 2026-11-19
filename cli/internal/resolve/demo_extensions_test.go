// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDemoExtensionsModern2025Syntax(t *testing.T) {
	extDir := filepath.Clean(filepath.Join("..", "..", "..", "extensions"))
	if _, err := os.Stat(extDir); err != nil {
		t.Fatalf("missing demo extensions dir %s: %v", extDir, err)
	}

	proj := t.TempDir()
	files := map[string]string{
		"src/Session.php": `<?php
readonly class SessionManager {
    public string $id { get => $this->id; }
    public function revoke(string $id): void {}
}
enum TokenState: string { case Active = 'active'; }
`,
		"src/client.mjs": `export async function fetchToken(url) { return url; }
export class TokenClient {}
export const DEFAULT_TTL = 300;
`,
		"lib/vault.rb": `module Vault
  Entry = Data.define(:id, :secret)
  class Store
    def self.open(path) = new
    def rotate_key(id) = true
  end
end
`,
		"src/lib.rs": `pub struct KeyRing;
pub async fn unlock_vault() {}
pub macro_rules! define_slot { () => {}; }
`,
		"pkg/cache.go": `package pkg
type Cache[K comparable, V any] struct{}
func (c *Cache[K, V]) Get(k K) (V, bool) { var zero V; return zero, false }
func NewCache[K comparable, V any]() *Cache[K, V] { return &Cache[K, V]{} }
`,
	}
	for rel, body := range files {
		full := filepath.Join(proj, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	reg := NewRegistryWithOptions(RegistryOptions{
		Extensions:    []string{"php", "js", "ruby", "rust", "golang"},
		ExtDir:        extDir,
		AllowExternal: true,
	})
	cases := []struct {
		kind Kind
		id   string
		want bool
	}{
		{"php-symbol", "src/Session.php::SessionManager", true},
		{"php-symbol", "src/Session.php::TokenState", true},
		{"php-symbol", "src/Session.php::MissingClass", false},
		{"js-symbol", "src/client.mjs::fetchToken", true},
		{"js-symbol", "src/client.mjs::TokenClient", true},
		{"js-symbol", "src/client.mjs::MissingFn", false},
		{"ruby-symbol", "lib/vault.rb::Store", true},
		{"ruby-symbol", "lib/vault.rb::Entry", true},
		{"ruby-symbol", "lib/vault.rb::rotate_key", true},
		{"ruby-symbol", "lib/vault.rb::missing_method", false},
		{"rust-symbol", "src/lib.rs::KeyRing", true},
		{"rust-symbol", "src/lib.rs::unlock_vault", true},
		{"rust-symbol", "src/lib.rs::MissingStruct", false},
		{"golang-symbol", "pkg/cache.go::Cache", true},
		{"golang-symbol", "pkg/cache.go::NewCache", true},
		{"golang-symbol", "pkg/cache.go::Get", true},
		{"golang-symbol", "pkg/cache.go::MissingFunc", false},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind)+"/"+tc.id, func(t *testing.T) {
			v := reg.Resolve(proj, tc.kind, tc.id)
			if v.Resolved != tc.want {
				t.Errorf("Resolve(%s, %q) = %+v, want resolved=%v", tc.kind, tc.id, v, tc.want)
			}
		})
	}
}
