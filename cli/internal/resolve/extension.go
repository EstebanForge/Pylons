// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"

	"strings"
	"time"
)

const maxExtStdoutBytes = 64 * 1024

// ExtensionInfo describes one built-in or external drop-in extension.
type ExtensionInfo struct {
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Enabled bool     `json:"enabled"`
	Kinds   []string `json:"kinds,omitempty"`
	Path    string   `json:"path,omitempty"`
}

// ListExtensions returns all built-in extensions plus external pylon-ext-* binaries in extDir.
func ListExtensions(enabled []string, extDir string) []ExtensionInfo {
	var out []ExtensionInfo
	for name, kindsMap := range builtinExtensions {
		kinds := make([]string, 0, len(kindsMap))
		for k := range kindsMap {
			kinds = append(kinds, string(k))
		}
		slices.Sort(kinds)
		out = append(out, ExtensionInfo{
			Name:    name,
			Source:  "builtin",
			Enabled: slices.Contains(enabled, name),
			Kinds:   kinds,
		})
	}
	if extDir != "" {
		if entries, err := os.ReadDir(extDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasPrefix(e.Name(), "pylon-ext-") {
					continue
				}
				name := strings.TrimPrefix(e.Name(), "pylon-ext-")
				if name == "" {
					continue
				}
				out = append(out, ExtensionInfo{
					Name:    name,
					Source:  "external",
					Enabled: slices.Contains(enabled, name),
					Kinds:   []string{name + "-*"},
					Path:    filepath.Join(extDir, e.Name()),
				})
			}
		}
	}
	slices.SortFunc(out, func(a, b ExtensionInfo) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Source, b.Source)
	})
	return out
}

// builtinExtensions maps opt-in extension names to their registered contract kinds.
var builtinExtensions = map[string]map[Kind]Resolver{
	"go": {
		"go-symbol": resolveGoSymbol,
	},
	"rust": {
		"rust-item": resolveRustItem,
	},
	"typescript": {
		"ts-export": resolveTSExport,
		"js-export": resolveTSExport,
	},
	"python": {
		"py-symbol": resolvePySymbol,
	},
	"php": {
		"php-symbol": resolvePHPSymbol,
		"wp-hook":    resolveWPHook,
	},
}

// wpHookFamily is every standard hook entry point: registration,
// firing, removal, queries, the ref-array and deprecated variants, and
// the remove_all pair. Variable and concatenated tags are out of scope
// by design: a contract names a literal tag.
var wpHookFamily = "add_action|add_filter|do_action|do_action_ref_array|do_action_deprecated|apply_filters|apply_filters_ref_array|apply_filters_deprecated|remove_action|remove_filter|remove_all_actions|remove_all_filters|has_action|has_filter|doing_action|doing_filter|did_action|did_filter"

// wpHookPattern is one pattern serving both engines: the worktree walk
// matches it line by line as RE2, the ref path hands the identical
// string to git grep -E as POSIX ERE. It is deliberately line-shaped
// ([[:space:]] classes, no \b, no (?:), no cross-line \s*), so a
// hook call wrapped across lines is out of scope on BOTH paths and the
// two engines can never disagree.
var wpHookPattern = `(^|[^A-Za-z0-9_])(` + wpHookFamily + `)[[:space:]]*\([[:space:]]*['"]`

// resolveWPHook resolves a hook-name contract: the id is the hook tag,
// and the hook exists when a bounded project PHP file registers, fires,
// removes, or queries the tag through the standard hook functions.
func resolveWPHook(projectRoot, ref string, kind Kind, id string) Verdict {
	tag := strings.TrimSpace(id)
	if tag == "" || strings.ContainsAny(tag, "\"'\\") {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "expected the hook tag as the id"}
	}
	pattern := wpHookPattern + regexp.QuoteMeta(tag) + `['"]`
	if match := grepTargetRegex(projectRoot, ref, map[string]bool{".php": true}, pattern); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "hook " + tag + " never registered, fired, or queried"}
}

func resolveGoSymbol(projectRoot, ref string, kind Kind, id string) Verdict {
	return resolveFileRegex(projectRoot, ref, kind, id, func(sym string) *regexp.Regexp {
		q := regexp.QuoteMeta(sym)
		return regexp.MustCompile(`(?m)(?:\b(?:func|type|const|var)\s+(?:\([^)]+\)\s*)?` + q + `\b|^\s*` + q + `\s*=)`)
	})
}

func resolveRustItem(projectRoot, ref string, kind Kind, id string) Verdict {
	return resolveFileRegex(projectRoot, ref, kind, id, func(sym string) *regexp.Regexp {
		q := regexp.QuoteMeta(sym)
		return regexp.MustCompile(`(?m)\b(?:pub(?:\([^)]*\))?\s+)?(?:(?:async|unsafe|const)\s+)*(?:fn|struct|enum|trait|type|const|static|mod|macro_rules!)\s+` + q + `\b`)
	})
}

func resolveTSExport(projectRoot, ref string, kind Kind, id string) Verdict {
	return resolveFileRegex(projectRoot, ref, kind, id, func(sym string) *regexp.Regexp {
		q := regexp.QuoteMeta(sym)
		return regexp.MustCompile(`(?m)\bexport\s+(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?(?:function\*?|const|let|var|class|interface|type|enum|namespace)\s+` + q + `\b|\bexport\s*\{[^}]*\b` + q + `\b`)
	})
}

func resolvePySymbol(projectRoot, ref string, kind Kind, id string) Verdict {
	return resolveFileRegex(projectRoot, ref, kind, id, func(sym string) *regexp.Regexp {
		if _, method, ok := strings.Cut(sym, "."); ok && method != "" {
			sym = method
		}
		q := regexp.QuoteMeta(sym)
		return regexp.MustCompile(`(?m)^\s*(?:async\s+)?(?:def|class)\s+` + q + `\b`)
	})
}

// matchNamespacedClass reports whether data declares the namespace of
// a namespaced id (Vendor\Class) and declares the short name bare: the
// file declares the namespace on its own line and the class bare. PHP
// folds case on namespaces, classes, and functions; the namespace may
// share the opening tag's line and the braced form is legal. decl
// selects the declaration keywords matched against the short name.
func matchNamespacedClass(data []byte, namespaced, decl string) bool {
	i := strings.LastIndexByte(namespaced, '\\')
	if i <= 0 {
		return false
	}
	ns, short := namespaced[:i], namespaced[i+1:]
	nsRe := regexp.MustCompile(`(?mi)^\s*(?:<\?php\s+)?namespace\s+` + regexp.QuoteMeta(ns) + `\s*[;{]`)
	shortRe := regexp.MustCompile(`(?mi)\b(?:` + decl + `)\s+` + regexp.QuoteMeta(short) + `\b`)
	return nsRe.Match(data) && shortRe.Match(data)
}

func resolvePHPSymbol(projectRoot, ref string, kind Kind, id string) Verdict {
	filePart, sym, ok := strings.Cut(id, "::")
	if !ok || filePart == "" || sym == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "expected <file>::<symbol>"}
	}
	rel, ok := stripRepoPrefix(filePart)
	if !ok {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "invalid file path"}
	}
	data, ok := ReadProjectFile(projectRoot, ref, rel)
	if !ok {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "file not found"}
	}
	if classPart, method, hasClass := strings.Cut(sym, "::"); hasClass && classPart != "" && method != "" {
		classPart = strings.Trim(classPart, "\\")
		methodRe := regexp.MustCompile(`(?m)\b(?:function|const)\s+` + regexp.QuoteMeta(method) + `\b`)
		if i := strings.LastIndexByte(classPart, '\\'); i > 0 {
			if matchNamespacedClass(data, classPart, "class|interface|trait|enum") && methodRe.Match(data) {
				return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
			}
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "class or method not found"}
		}
		classRe := regexp.MustCompile(`(?m)\b(?:class|interface|trait|enum)\s+` + regexp.QuoteMeta(classPart) + `\b`)
		if classRe.Match(data) && methodRe.Match(data) {
			return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
		}
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "class or method not found"}
	}
	// Class-only ids keep the bare keyword path unless the symbol is
	// namespaced: a backslashed name can never follow a declaration
	// keyword, so resolve through the namespace declaration. The short
	// name may be a function or const, not only a class-like decl.
	if strings.LastIndexByte(sym, '\\') > 0 {
		if matchNamespacedClass(data, strings.Trim(sym, "\\"), "function|class|interface|trait|enum|const") {
			return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
		}
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "declaration not found"}
	}
	q := regexp.QuoteMeta(sym)
	re := regexp.MustCompile(`(?m)\b(?:function|class|interface|trait|enum|const)\s+` + q + `\b`)
	if re.Match(data) {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "declaration not found"}
}

func resolveFileRegex(projectRoot, ref string, kind Kind, id string, buildRe func(string) *regexp.Regexp) Verdict {
	filePart, sym, ok := strings.Cut(id, "::")
	if !ok || filePart == "" || sym == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "expected <file>::<symbol>"}
	}
	rel, ok := stripRepoPrefix(filePart)
	if !ok {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "invalid file path"}
	}
	data, ok := ReadProjectFile(projectRoot, ref, rel)
	if !ok {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "file not found"}
	}
	if buildRe(sym).Match(data) {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "declaration not found"}
}

type extRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Root          string `json:"root"`
}

type extResponse struct {
	Resolved bool   `json:"resolved"`
	Detail   string `json:"detail"`
	Error    string `json:"error,omitempty"`
}

func (r *Registry) tryExternalExtension(projectRoot string, kind Kind, id string) (Verdict, bool) {
	prefix, _, hasDash := strings.Cut(string(kind), "-")
	if !hasDash || prefix == "" {
		// No extension namespace in the kind: never pick an arbitrary binary.
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "invalid contract kind"}, true
	}
	for _, name := range r.enabledExtensions {
		if name != prefix && name != string(kind) {
			continue
		}
		bin := r.findExternalBinary(name)
		if bin == "" {
			continue
		}
		if !r.allowExternal {
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "external extensions disabled (pass --allow-external-extensions)"}, true
		}
		reqBytes, err := json.Marshal(extRequest{SchemaVersion: 1, Kind: string(kind), ID: id, Root: projectRoot})
		if err != nil {
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: err.Error()}, true
		}
		var resp extResponse
		if err := runExternalJSON(bin, reqBytes, &resp); err != nil {
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: err.Error()}, true
		}
		return Verdict{Kind: kind, ID: id, Resolved: resp.Resolved, Detail: resp.Detail}, true
	}
	return Verdict{}, false
}

// runExternalJSON runs one plugin binary with one JSON request line
// on stdin and decodes one JSON response from stdout. The guards are
// the shared extension protocol guards: 5s timeout, 64KB stdout cap,
// and a WaitDelay so a grandchild holding the pipe cannot hang the
// read forever.
func runExternalJSON(bin string, reqBytes []byte, response any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = extEnv()
	cmd.Stdin = bytes.NewReader(append(reqBytes, '\n'))
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("extension pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("extension start failed: %w", err)
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, maxExtStdoutBytes))
	waitErr := cmd.Wait()
	if readErr != nil || waitErr != nil {
		return errors.New("extension failed")
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), response); err != nil {
		return errors.New("invalid extension JSON")
	}
	return nil
}

// CallExternal runs the pylon-ext-<name> plugin for one non-contract
// request (parity ops and future protocol users). The plugin must be
// an enabled extension and --allow-external-extensions must be on.
func (r *Registry) CallExternal(name string, request, response any) error {
	if !slices.Contains(r.enabledExtensions, name) {
		return fmt.Errorf("extension %s is not enabled", name)
	}
	if !r.allowExternal {
		return errors.New("external extensions disabled (pass --allow-external-extensions)")
	}
	bin := r.findExternalBinary(name)
	if bin == "" {
		return fmt.Errorf("extension binary pylon-ext-%s not found", name)
	}
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return runExternalJSON(bin, reqBytes, response)
}

func extEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	// Windows child processes fail without these.
	for _, key := range []string{"SystemRoot", "COMSPEC", "windir"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}

func (r *Registry) findExternalBinary(name string) string {
	if strings.ContainsAny(name, "/\\") {
		return ""
	}
	exeName := "pylon-ext-" + name
	for _, candidate := range []string{exeName, exeName + ".exe"} {
		if r.extDir != "" {
			p := filepath.Join(r.extDir, candidate)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return p
			}
		}
		if p, err := exec.LookPath(candidate); err == nil {
			return p
		}
	}
	return ""
}
