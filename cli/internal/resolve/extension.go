// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"bytes"
	"context"
	"encoding/json"
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
	},
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
		classRe := regexp.MustCompile(`(?m)\b(?:class|interface|trait|enum)\s+` + regexp.QuoteMeta(classPart) + `\b`)
		methodRe := regexp.MustCompile(`(?m)\b(?:function|const)\s+` + regexp.QuoteMeta(method) + `\b`)
		if classRe.Match(data) && methodRe.Match(data) {
			return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
		}
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "class or method not found"}
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, bin)
		cmd.Env = extEnv()
		cmd.Stdin = bytes.NewReader(append(reqBytes, '\n'))
		// A grandchild holding the stdout pipe must not block ReadAll forever.
		cmd.WaitDelay = time.Second
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: err.Error()}, true
		}
		if err := cmd.Start(); err != nil {
			cancel()
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "extension start failed: " + err.Error()}, true
		}
		out, readErr := io.ReadAll(io.LimitReader(stdout, maxExtStdoutBytes))
		waitErr := cmd.Wait()
		cancel()
		if readErr != nil || waitErr != nil {
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "extension failed"}, true
		}
		var resp extResponse
		if err := json.Unmarshal(bytes.TrimSpace(out), &resp); err != nil {
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "invalid extension JSON"}, true
		}
		return Verdict{Kind: kind, ID: id, Resolved: resp.Resolved, Detail: resp.Detail}, true
	}
	return Verdict{}, false
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
