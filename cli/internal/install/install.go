// Package install wires the Pylons gate hooks into the surrounding git
// repository: hook scripts in .githooks/ and core.hooksPath pointing there.
package install

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/resolve"
)

//go:embed assets/pre-commit assets/pre-push
var assets embed.FS

var hookNames = []string{"pre-commit", "pre-push"}

// Result reports what EnsureHooks changed.
type Result struct {
	WroteHooks   []string
	SkippedHooks []string
	HooksPathSet bool
	BinaryOK     bool
}

// EnsureHooks writes missing .githooks scripts into the git toplevel of
// kbRoot, enables core.hooksPath, and checks for bin/pylon. Existing hook
// files are never overwritten. The error is non-nil only when kbRoot is not
// inside a git repository.
func EnsureHooks(kbRoot string) (Result, error) {
	var res Result

	top, err := gitTopLevel(kbRoot)
	if err != nil {
		return res, err
	}

	if err := os.MkdirAll(filepath.Join(top, ".githooks"), 0o755); err != nil {
		return res, err
	}
	for _, name := range hookNames {
		dst := filepath.Join(top, ".githooks", name)
		if _, err := os.Stat(dst); err == nil {
			res.SkippedHooks = append(res.SkippedHooks, name)
			continue
		}
		body, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return res, err
		}
		if err := os.WriteFile(dst, body, 0o755); err != nil {
			return res, err
		}
		res.WroteHooks = append(res.WroteHooks, name)
	}

	want := ".githooks"
	out, err := resolve.GitRun(top, nil, "config", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	if err != nil || current != want {
		if _, err := resolve.GitRun(top, nil, "config", "core.hooksPath", want); err != nil {
			return res, err
		}
		res.HooksPathSet = true
	}

	if st, err := os.Stat(filepath.Join(top, "bin", "pylon")); err == nil && st.Mode()&0o111 != 0 {
		res.BinaryOK = true
	}
	return res, nil
}

func gitTopLevel(kbRoot string) (string, error) {
	out, err := resolve.GitRun(kbRoot, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository", kbRoot)
	}
	return strings.TrimSpace(string(out)), nil
}
