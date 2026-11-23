package install

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the working directory until it finds the
// checkout root: the directory holding manifests/ and cli/. Fixed
// .. counting breaks when go test runs anywhere but the package dir.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "manifests", "pylons.yaml")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no ledger root above the working directory")
		}
		dir = parent
	}
}
