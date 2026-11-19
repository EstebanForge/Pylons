// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import (
	"context"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Kind identifies a contract entry type.
type Kind string

const (
	KindFile   Kind = "file"
	KindGlob   Kind = "glob"
	KindSymbol Kind = "symbol"
	KindRoute  Kind = "route"
	KindCLI    Kind = "cli"
	KindEvent  Kind = "event"
	KindSchema Kind = "schema"
)

// Verdict is the deterministic resolution result for one contract entry.
type Verdict struct {
	Kind     Kind   `json:"kind"`
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
	Detail   string `json:"detail,omitempty"`
}

// Resolver resolves a single contract kind against a project root.
type Resolver func(projectRoot, ref string, kind Kind, id string) Verdict

// RegistryOptions configures a resolver Registry.
type RegistryOptions struct {
	Extensions    []string
	ExtDir        string
	Ref           string
	AllowExternal bool
}

// Registry dispatches core kinds and opt-in drop-in extensions.
type Registry struct {
	enabledExtensions []string
	extDir            string
	ref               string
	allowExternal     bool
	kinds             map[Kind]Resolver
}

// NewRegistry builds a resolver registry with external execution disabled by default.
func NewRegistry(enabledExtensions []string, extDir string) *Registry {
	return NewRegistryWithOptions(RegistryOptions{
		Extensions: enabledExtensions,
		ExtDir:     extDir,
	})
}

// NewRegistryWithOptions builds a resolver registry from opts.
func NewRegistryWithOptions(opts RegistryOptions) *Registry {
	r := &Registry{
		enabledExtensions: slices.Clone(opts.Extensions),
		extDir:            opts.ExtDir,
		ref:               opts.Ref,
		allowExternal:     opts.AllowExternal,
		kinds: map[Kind]Resolver{
			KindFile:   resolveFile,
			KindGlob:   resolveGlob,
			KindSymbol: resolveSymbol,
			KindRoute:  resolveRoute,
			KindCLI:    resolveCLI,
			KindEvent:  resolveEvent,
			KindSchema: resolveSchema,
		},
	}
	for _, name := range opts.Extensions {
		if ext, ok := builtinExtensions[name]; ok {
			maps.Copy(r.kinds, ext)
		}
	}
	return r
}

// Resolve resolves kind+id against projectRoot.
func (r *Registry) Resolve(projectRoot string, kind Kind, id string) Verdict {
	if fn, ok := r.kinds[kind]; ok {
		return fn(projectRoot, r.ref, kind, id)
	}
	if v, handled := r.tryExternalExtension(projectRoot, kind, id); handled {
		return v
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "unknown or disabled contract kind"}
}

// SupportsKind reports whether kind is handled by core or an enabled built-in extension.
func (r *Registry) SupportsKind(kind Kind) bool {
	_, ok := r.kinds[kind]
	return ok
}

// SafeJoin resolves rel inside root, rejecting traversal and escaping symlinks.
func SafeJoin(root, rel string) (string, bool) {
	if root == "" || !isSafeRelPath(rel) {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	candidate := filepath.Clean(filepath.Join(absRoot, filepath.FromSlash(rel)))
	relCheck, err := filepath.Rel(absRoot, candidate)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return candidate, true
	}
	if realCand, err := filepath.EvalSymlinks(candidate); err == nil {
		r, err := filepath.Rel(realRoot, realCand)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", false
		}
	}
	return candidate, true
}

// MatchGlob matches target against pattern, supporting ** across path segments.
func MatchGlob(pattern, target string) bool {
	pattern = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(pattern), "/"))
	target = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(target), "/"))
	if pattern == "" || pattern == "." {
		return false
	}
	patSegs := strings.Split(pattern, "/")
	tgtSegs := strings.Split(target, "/")
	return matchSegments(patSegs, tgtSegs)
}

func matchSegments(pat, tgt []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(tgt); i++ {
				if matchSegments(pat[1:], tgt[i:]) {
					return true
				}
			}
			return false
		}
		if len(tgt) == 0 {
			return false
		}
		ok, err := filepath.Match(pat[0], tgt[0])
		if err != nil || !ok {
			return false
		}
		pat = pat[1:]
		tgt = tgt[1:]
	}
	return len(tgt) == 0
}

// gitCallTimeout bounds each git subprocess so a wedged checkout (locked
// index, hung NFS mount) cannot freeze gate hooks or installers.
const gitCallTimeout = 10 * time.Second

// GitRun runs git -C <dir> with args and returns stdout.
func GitRun(dir string, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = stdin
	return cmd.Output()
}

// ReadProjectFile reads rel from git ref when ref != "" and root is a git repo,
// otherwise safely from the working tree.
func ReadProjectFile(root, ref, rel string) ([]byte, bool) {
	if !isSafeRelPath(rel) {
		return nil, false
	}
	if ref != "" && hasGitRepo(root) {
		if refExists(root, ref) {
			out, err := GitRun(root, nil, "show", ref+":"+rel)
			if err != nil {
				return nil, false
			}
			return out, true
		}
	}
	target, ok := SafeJoin(root, rel)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, false
	}
	return data, true
}

// StatProjectFile reports whether rel exists in git ref (when applicable) or working tree.
func StatProjectFile(root, ref, rel string) bool {
	if !isSafeRelPath(rel) {
		return false
	}
	if ref != "" && hasGitRepo(root) && refExists(root, ref) {
		_, err := GitRun(root, nil, "cat-file", "-e", ref+":"+rel)
		return err == nil
	}
	target, ok := SafeJoin(root, rel)
	if !ok {
		return false
	}
	info, err := os.Stat(target)
	return err == nil && !info.IsDir()
}

func hasGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

func refExists(root, ref string) bool {
	_, err := GitRun(root, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func resolveFile(projectRoot, ref string, kind Kind, id string) Verdict {
	rel, ok := stripRepoPrefix(id)
	if !ok || !StatProjectFile(projectRoot, ref, rel) {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "file not found"}
	}
	return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
}

func resolveGlob(projectRoot, ref string, kind Kind, id string) Verdict {
	relPat, ok := stripRepoPrefix(id)
	if !ok {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "invalid glob path"}
	}
	if ref != "" && hasGitRepo(projectRoot) && refExists(projectRoot, ref) {
		out, err := GitRun(projectRoot, nil, "ls-tree", "-r", "--name-only", ref)
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if line != "" && MatchGlob(relPat, line) {
					return Verdict{Kind: kind, ID: id, Resolved: true, Detail: line}
				}
			}
			return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "glob matched zero files"}
		}
	}

	found := ""
	_ = filepath.WalkDir(projectRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "target" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(projectRoot, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if MatchGlob(relPat, rel) {
			found = rel
			return fs.SkipAll
		}
		return nil
	})
	if found != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: found}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "glob matched zero files"}
}

func resolveSymbol(projectRoot, ref string, kind Kind, id string) Verdict {
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
	if containsSymbol(data, sym) {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: rel}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "symbol not found in file"}
}

// containsSymbol matches sym on word boundaries so Get does not resolve
// inside GetString; boundaries apply only when both ends are word chars.
func containsSymbol(data []byte, sym string) bool {
	pattern := regexp.QuoteMeta(sym)
	if isWordByte(sym[0]) && isWordByte(sym[len(sym)-1]) {
		pattern = `\b` + pattern + `\b`
	}
	re, err := regexp.Compile(pattern)
	return err == nil && re.Match(data)
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func resolveRoute(projectRoot, _ string, kind Kind, id string) Verdict {
	parts := strings.Fields(id)
	if len(parts) < 2 {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "expected METHOD /path"}
	}
	if match := grepProject(projectRoot, parts[1]); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "route path not found"}
}

func resolveCLI(projectRoot, _ string, kind Kind, id string) Verdict {
	fields := strings.Fields(id)
	if len(fields) == 0 {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty cli command"}
	}
	if match := grepProject(projectRoot, fields[len(fields)-1]); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "cli command not found"}
}

func resolveEvent(projectRoot, _ string, kind Kind, id string) Verdict {
	if id == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty event name"}
	}
	if match := grepProject(projectRoot, id); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "event not found"}
}

func resolveSchema(projectRoot, _ string, kind Kind, id string) Verdict {
	if id == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty schema name"}
	}
	if match := grepProject(projectRoot, id); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "schema not found"}
}

func stripRepoPrefix(raw string) (string, bool) {
	if strings.Contains(raw, "://") {
		u, ok := ParseURI(raw)
		if !ok || u.Scheme != "repo" {
			return "", false
		}
		return u.Path, true
	}
	clean := strings.TrimPrefix(raw, "/")
	if !isSafeRelPath(clean) {
		return "", false
	}
	return clean, true
}

func grepProject(root, needle string) string {
	if root == "" || needle == "" {
		return ""
	}
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "target" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), needle) {
			rel, relErr := filepath.Rel(root, p)
			if relErr != nil {
				found = p
			} else {
				found = filepath.ToSlash(rel)
			}
			return fs.SkipAll
		}
		return nil
	})
	return found
}
