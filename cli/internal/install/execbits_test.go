package install

import (
	"os"
	"path/filepath"
	"testing"
)

// Replaced product files keep the staged file's permission bits: the
// example parity plugins ship executable and a 0644 rebuild breaks
// their dispatch.
func TestSelfUpdatePreservesExecBits(t *testing.T) {
	installed := t.TempDir()
	staged := t.TempDir()
	buildInstalled(t, installed, "1.0.0")
	buildStaged(t, staged, "1.1.0", func(dir string) {
		p := filepath.Join(dir, "extensions", "_examples", "pylon-ext-js")
		// Change the content too: skip-identical must not hide the mode
		// the replacement would have written. WriteFile applies perm only
		// at creation, so the chmod is explicit.
		if err := os.WriteFile(p, []byte("plugin v2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	})
	res, err := SelfUpdate(installed, staged, false, false)
	if err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	dst := filepath.Join(installed, "extensions", "_examples", "pylon-ext-js")
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("replaced plugin mode %v, want 0755", info.Mode().Perm())
	}
	if len(res.Replaced) == 0 {
		t.Fatal("nothing replaced; the test proves nothing")
	}
}
