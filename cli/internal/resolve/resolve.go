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
	if msg := refErr(projectRoot, r.ref); msg != "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: msg}
	}
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
// Trailing /** requires at least one segment under it (a/** never matches bare a).
func MatchGlob(pattern, target string) bool {
	pattern = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(pattern), "/"))
	target = filepath.ToSlash(strings.TrimPrefix(filepath.Clean(target), "/"))
	if pattern == "" || pattern == "." {
		return false
	}
	patSegs := strings.Split(pattern, "/")
	tgtSegs := strings.Split(target, "/")
	memo := make(map[[2]int]bool)
	var m func(pi, ti int) bool
	m = func(pi, ti int) bool {
		key := [2]int{pi, ti}
		if v, ok := memo[key]; ok {
			return v
		}
		var res bool
		if pi == len(patSegs) {
			res = ti == len(tgtSegs)
		} else if patSegs[pi] == "**" {
			litBefore := false
			for k := 0; k < pi; k++ {
				if patSegs[k] != "**" {
					litBefore = true
					break
				}
			}
			if pi == len(patSegs)-1 {
				// a/** covers a's contents only; bare ** covers everything.
				if litBefore {
					res = ti < len(tgtSegs)
				} else {
					res = true
				}
			} else {
				res = false
				for i := ti; i <= len(tgtSegs); i++ {
					if m(pi+1, i) {
						res = true
						break
					}
				}
			}
		} else if ti == len(tgtSegs) {
			res = false
		} else {
			ok, err := filepath.Match(patSegs[pi], tgtSegs[ti])
			if err != nil || !ok {
				res = false
			} else {
				res = m(pi+1, ti+1)
			}
		}
		memo[key] = res
		return res
	}
	return m(0, 0)
}

// gitCallTimeout bounds each git subprocess so a wedged checkout (locked
// index, hung NFS mount) cannot freeze gate hooks or installers.
const gitCallTimeout = 10 * time.Second

const (
	maxGrepFileBytes    = 2 * 1024 * 1024
	maxProjectFileBytes = 4 * 1024 * 1024
)

// GitRun runs git -C <dir> with args and returns stdout.
func GitRun(dir string, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = stdin
	// A grandchild holding the stdout pipe must not block ReadAll forever.
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// refErr reports why a configured ref is unusable, or "" when it resolves.
// A configured-but-missing ref must fail closed, never fall back to the
// working tree silently.
func refErr(root, ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "-") {
		return "invalid ref " + ref
	}
	if !hasGitRepo(root) {
		// Worktree-only project: there is no git to fail closed on.
		return ""
	}
	if _, err := GitRun(root, nil, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return "configured ref " + ref + " not resolvable"
	}
	return ""
}

// ReadProjectFile reads rel from git ref when ref != "" and root is a git repo,
// otherwise safely from the working tree.
func ReadProjectFile(root, ref, rel string) ([]byte, bool) {
	if !isSafeRelPath(rel) {
		return nil, false
	}
	if ref != "" && hasGitRepo(root) {
		if refExists(root, ref) {
			out, err := GitRun(root, nil, "show", ref+":./"+rel)
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
	f, err := os.Open(target)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, maxProjectFileBytes))
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
		out, err := GitRun(root, nil, "cat-file", "-t", ref+":./"+rel)
		return err == nil && strings.TrimSpace(string(out)) == "blob"
	}
	target, ok := SafeJoin(root, rel)
	if !ok {
		return false
	}
	info, err := os.Stat(target)
	return err == nil && info.Mode().IsRegular()
}

// hasGitRepo reports whether root sits inside a git worktree. Git
// resolves upward, so a project registered at a subdirectory (a
// crate inside a workspace repo) is still a git project and keeps
// its configured ref.
func hasGitRepo(root string) bool {
	root = filepath.Clean(root)
	for {
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(root)
		if parent == root {
			return false
		}
		root = parent
	}
}

// gitToplevelPrefix returns the project root's path relative to the
// repo toplevel with a trailing slash ("" at the toplevel), so
// toplevel-relative ls-tree lines can be compared with
// project-relative globs.
func gitToplevelPrefix(root string) string {
	out, err := GitRun(root, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(strings.TrimSpace(string(out)), root)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel) + "/"
}

func refExists(root, ref string) bool {
	return ref != "" && refErr(root, ref) == ""
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
		out, err := GitRun(projectRoot, nil, "ls-tree", "-r", "--name-only", "-z", ref)
		if err == nil {
			prefix := gitToplevelPrefix(projectRoot)
			for _, line := range strings.Split(string(out), "\x00") {
				line = strings.TrimSpace(line)
				if line != "" && MatchGlob(relPat, strings.TrimPrefix(line, prefix)) {
					return Verdict{Kind: kind, ID: id, Resolved: true, Detail: strings.TrimPrefix(line, prefix)}
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

func resolveRoute(projectRoot, ref string, kind Kind, id string) Verdict {
	parts := strings.Fields(id)
	if len(parts) < 2 {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "expected METHOD /path"}
	}
	if match := grepTarget(projectRoot, ref, parts[1]); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "route path not found"}
}

func resolveCLI(projectRoot, ref string, kind Kind, id string) Verdict {
	fields := strings.Fields(id)
	if len(fields) == 0 {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty cli command"}
	}
	if match := grepTarget(projectRoot, ref, fields[len(fields)-1]); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "cli command not found"}
}

func resolveEvent(projectRoot, ref string, kind Kind, id string) Verdict {
	if id == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty event name"}
	}
	if match := grepTarget(projectRoot, ref, id); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "event not found"}
}

func resolveSchema(projectRoot, ref string, kind Kind, id string) Verdict {
	if id == "" {
		return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "empty schema name"}
	}
	if match := grepTarget(projectRoot, ref, id); match != "" {
		return Verdict{Kind: kind, ID: id, Resolved: true, Detail: match}
	}
	return Verdict{Kind: kind, ID: id, Resolved: false, Detail: "schema not found"}
}

// grepTarget searches the configured ref via git grep so route/cli/event/schema
// contracts honor repo:// base refs instead of silently reading the worktree.
// grepTargetRegex is grepTarget with a regex and a filename-extension
// allowlist on the worktree path; the ref path runs git grep -E.
func grepTargetRegex(root, ref string, exts map[string]bool, re *regexp.Regexp) string {
	if ref != "" && hasGitRepo(root) {
		out, err := GitRun(root, nil, "grep", "-I", "-m", "1", "-E", "-e", re.String(), ref)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(out))
		if idx := strings.Index(line, ref+":"); idx >= 0 {
			rest := line[idx+len(ref)+1:]
			if end := strings.IndexByte(rest, ':'); end > 0 {
				return rest[:end]
			}
		}
		return ""
	}
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "target" || base == "vendor" || strings.HasPrefix(base, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if exts != nil && !exts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxGrepFileBytes {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(f, maxGrepFileBytes+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return nil
		}
		if re.Match(data) {
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

func grepTarget(root, ref, needle string) string {
	if ref != "" && hasGitRepo(root) {
		out, err := GitRun(root, nil, "grep", "-I", "--fixed-strings", "-m", "1", "-e", needle, ref)
		if err != nil {
			return ""
		}
		// Output lines look like <ref>:<path>:<line>:<text>.
		line := strings.TrimSpace(string(out))
		if idx := strings.Index(line, ref+":"); idx >= 0 {
			rest := line[idx+len(ref)+1:]
			if end := strings.IndexByte(rest, ':'); end > 0 {
				return rest[:end]
			}
		}
		return ""
	}
	return grepProject(root, needle)
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
		// lstat type: a symlink or FIFO must never be opened for reading.
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxGrepFileBytes {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(f, maxGrepFileBytes+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
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
