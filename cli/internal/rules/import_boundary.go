// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// importDirSkip names directories never walked for import extraction.
var importDirSkip = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"target": true, "__pycache__": true, ".venv": true, "venv": true,
	"testdata": true, "build": true, "out": true, "coverage": true,
	".cache": true,
}

// pythonGenericDirs are directory names too generic to serve as import
// identities: `import src` or `import cmd` must never fire an edge.
var pythonGenericDirs = map[string]bool{
	"src": true, "cmd": true, "internal": true, "pkg": true, "bin": true,
	"lib": true, "tests": true, "test": true, "docs": true, "scripts": true,
	"tools": true, "app": true, "apps": true, "examples": true,
	"testdata": true, "build": true, "out": true, "coverage": true,
}

// pkgDoc is one live package document with its parsed frontmatter.
type pkgDoc struct {
	rel string
	fm  resolve.PackageFrontMatter
}

// lintImportBoundaries extracts import edges from enabled-language files
// inside registered projects and compares each cross-project edge against
// the owning package's depends_on. Advisory-first per the roadmap: the
// rule informs until maps are clean, then it can be flipped per platform.
// changedSet scopes the scan to staged package docs under --changed.
func lintImportBoundaries(kbRoot string, cfg config.Config, changedSet map[string]bool) []report.Finding {
	ident := projectIdentities(kbRoot, cfg)
	if len(ident) == 0 {
		return nil
	}

	pkgs := collectPackages(kbRoot, changedSet)
	// owners maps a component alias to every live package claiming it;
	// components of one package are internal to each other.
	owners := map[string][]string{}
	for _, pd := range pkgs {
		for _, alias := range pd.fm.Components {
			if !slices.Contains(owners[alias], pd.fm.Package) {
				owners[alias] = append(owners[alias], pd.fm.Package)
			}
		}
	}

	otherRoots := map[string]string{} // abs project root -> alias
	for _, id := range ident {
		if root := cfg.ProjectPath(kbRoot, id.alias); root != "" {
			otherRoots[root] = id.alias
		}
	}

	// One walk per component alias, regardless of how many packages
	// claim it.
	edgesByAlias := map[string][]importEdge{}
	for _, alias := range slices.Sorted(maps.Keys(owners)) {
		root := cfg.ProjectPath(kbRoot, alias)
		if root == "" {
			continue
		}
		edgesByAlias[alias] = scanImports(root, alias, ident, enabledLanguages(cfg, alias), otherRoots)
	}

	pkgByName := map[string]pkgDoc{}
	for _, pd := range pkgs {
		pkgByName[pd.fm.Package] = pd
	}

	var out []report.Finding
	for _, pkgName := range slices.Sorted(maps.Keys(pkgByName)) {
		pd := pkgByName[pkgName]
		deps := map[string]bool{pkgName: true}
		for _, d := range pd.fm.DependsOn {
			deps[d] = true
		}
		// One finding per undeclared target, naming up to three
		// source files, so the ratchet counts targets not noise.
		byTarget := map[string][]importEdge{}
		for _, alias := range pd.fm.Components {
			for _, edge := range edgesByAlias[alias] {
				// A sibling component of the same package is
				// internal even when it is another project alias.
				if slices.Contains(pd.fm.Components, edge.target) || deps[edge.target] {
					continue
				}
				// depends_on holds package names; an alias clears
				// the edge when any package owning that alias is
				// declared.
				if anyOwnedDeclared(owners[edge.target], deps) {
					continue
				}
				byTarget[edge.target] = append(byTarget[edge.target], edge)
			}
		}
		for _, target := range slices.Sorted(maps.Keys(byTarget)) {
			edges := byTarget[target]
			files := make([]string, 0, 3)
			for i, e := range edges {
				if i == 3 {
					break
				}
				files = append(files, e.file)
			}
			more := ""
			if len(edges) > 3 {
				more = " and " + strconv.Itoa(len(edges)-3) + " more"
			}
			out = append(out, report.Finding{
				RuleID:   "PYLON043",
				Severity: report.SeverityAdvisory,
				Grade:    report.GradeAdvisory,
				File:     pd.rel,
				Line:     1,
				Message: fmt.Sprintf("IMPORT_UNDECLARED: %s imports project %s (%s%s); package %s does not declare it in depends_on",
					pkgName, target, strings.Join(files, ", "), more, pkgName),
			})
		}
	}
	return out
}

// anyOwnedDeclared reports whether any package that owns the imported
// alias is present in the depending package's declared set.
func anyOwnedDeclared(targetOwners []string, deps map[string]bool) bool {
	for _, p := range targetOwners {
		if deps[p] {
			return true
		}
	}
	return false
}

// collectPackages reads the live package documents, scoped to
// changedSet when the lint is --changed.
func collectPackages(kbRoot string, changedSet map[string]bool) []pkgDoc {
	pkgDir := filepath.Join(kbRoot, "packages")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}
	var out []pkgDoc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" || e.Name() == "AGENTS.md" {
			continue
		}
		rel := "packages/" + e.Name()
		if changedSet != nil && !changedSet[rel] {
			continue
		}
		raw, err := readManifest(filepath.Join(pkgDir, e.Name()))
		if err != nil {
			continue
		}
		var fm resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(string(raw), &fm) || fm.Type != "package" {
			continue
		}
		if fm.Status == "deprecated" || fm.Status == "retired" {
			continue
		}
		out = append(out, pkgDoc{rel: rel, fm: fm})
	}
	return out
}

// readManifest reads a project manifest with the regular-file and
// size bounds shared by the rest of the engine.
func readManifest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return readBounded(path)
}

// projectIdentity holds one alias's import-name candidates: the strings
// an import specifier can name the project by.
type projectIdentity struct {
	alias string
	names []string
}

// projectIdentities reads each registered project's package identity:
// go.mod module, package.json name, Cargo.toml package name, composer
// psr-4 prefixes, and Python top-level package directories.
func projectIdentities(kbRoot string, cfg config.Config) []projectIdentity {
	var out []projectIdentity
	for _, alias := range slices.Sorted(maps.Keys(cfg.Projects)) {
		root := cfg.ProjectPath(kbRoot, alias)
		if root == "" {
			continue
		}
		id := projectIdentity{alias: alias}
		if m := goModulePath(root); m != "" {
			id.names = append(id.names, m)
		}
		if m := jsonName(root, "package.json"); m != "" {
			id.names = append(id.names, m)
		}
		if m := cargoName(root); m != "" {
			id.names = append(id.names, m, strings.ReplaceAll(m, "-", "_"))
		}
		id.names = append(id.names, composerPrefixes(root)...)
		id.names = append(id.names, pythonIdentities(root)...)
		id.names = append(id.names, pomIdentity(root)...)
		id.names = append(id.names, gradleIdentity(root)...)
		if n := pubspecName(root); n != "" {
			id.names = append(id.names, n)
		}
		slices.Sort(id.names)
		id.names = slices.Compact(id.names)
		if len(id.names) > 0 {
			out = append(out, id)
		}
	}
	return out
}

// pomIdentity reads a maven project's own coordinates: only tags that
// are direct children of <project> count, so an inherited parent block
// and dependency coordinates never stand in for the project's own.
// XML comments are skipped: a commented-out coordinate is not an
// identity. Only the qualified group.artifact name is an identity; the
// bare artifact name is not, because artifact words are too generic to
// match import namespaces across languages.
func pomIdentity(root string) []string {
	raw, err := readManifest(filepath.Join(root, "pom.xml"))
	if err != nil {
		return nil
	}
	var group, artifact string
	depth := 0
	inComment := false
	for _, line := range strings.Split(raw, "\n") {
		t := stripXMLComments(line, &inComment)
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		opens := strings.Count(t, "<") - strings.Count(t, "</") - strings.Count(t, "<?") - strings.Count(t, "<!")
		closes := strings.Count(t, "</")
		selfClose := strings.HasSuffix(t, "/>")
		if depth == 1 {
			if group == "" {
				if m := pomGroupIdRe.FindStringSubmatch(t); m != nil {
					group = m[1]
				}
			}
			if artifact == "" {
				if m := pomArtifactIdRe.FindStringSubmatch(t); m != nil {
					artifact = m[1]
				}
			}
		}
		if selfClose {
			continue
		}
		depth += opens - closes
		if depth < 0 {
			depth = 0
		}
	}
	if artifact == "" || group == "" {
		return nil
	}
	return []string{group + "." + artifact}
}

// stripXMLComments removes <!-- --> spans from one line, carrying an
// unterminated comment across lines through inComment.
func stripXMLComments(line string, inComment *bool) string {
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		if *inComment {
			if i+2 < len(line) && line[i:i+3] == "-->" {
				*inComment = false
				i += 2
			}
			continue
		}
		if i+3 < len(line) && line[i:i+4] == "<!--" {
			*inComment = true
			i += 3
			continue
		}
		b.WriteByte(line[i])
	}
	return b.String()
}

// gradleIdentity reads a gradle project's root name and group from the
// groovy and kotlin dialects alike, in both quote styles; commented
// lines never count.
func gradleIdentity(root string) []string {
	var name, group string
	for _, f := range []string{"settings.gradle.kts", "settings.gradle"} {
		raw, err := readManifest(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if m := gradleRootNameRe.FindStringSubmatch(line); m != nil {
				name = m[1]
				break
			}
		}
		if name != "" {
			break
		}
	}
	for _, f := range []string{"build.gradle.kts", "build.gradle"} {
		raw, err := readManifest(filepath.Join(root, f))
		if err != nil {
			continue
		}
		// Only a top-level group counts: a group inside a subprojects
		// block names that submodule, not this project.
		depth := 0
		for _, line := range strings.Split(raw, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			code := stripQuotedSpans(trimmed)
			if depth == 0 {
				if m := gradleGroupRe.FindStringSubmatch(line); m != nil {
					group = m[1]
					break
				}
			}
			depth += strings.Count(code, "{") - strings.Count(code, "}")
			if depth < 0 {
				depth = 0
			}
		}
		if group != "" {
			break
		}
	}
	if name == "" || group == "" {
		return nil
	}
	return []string{group + "." + name}
}

// stripQuotedSpans removes single- and double-quoted spans so brace
// counting never sees a brace inside a string.
func stripQuotedSpans(line string) string {
	var b strings.Builder
	var q byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if q != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			q = c
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// pubspecName reads a dart project's pubspec name.
func pubspecName(root string) string {
	raw, err := readManifest(filepath.Join(root, "pubspec.yaml"))
	if err != nil {
		return ""
	}
	m := pubspecNameRe.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	return m[1]
}

// enabledLanguages returns the enabled extension names for a project:
// global extensions plus the project's own list.
func enabledLanguages(cfg config.Config, alias string) map[string]bool {
	langs := make(map[string]bool)
	for _, e := range cfg.Extensions {
		langs[e] = true
	}
	for _, e := range cfg.ProjectExtensions(alias) {
		langs[e] = true
	}
	return langs
}

// goModulePath reads the module line of a project's go.mod.
func goModulePath(root string) string {
	raw, err := readManifest(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(raw, "\n") {
		if m := goModuleRe.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

var goModuleRe = regexp.MustCompile(`^module\s+(\S+)`)

// jsonName reads the "name" field of a JSON manifest.
func jsonName(root, file string) string {
	raw, err := readManifest(filepath.Join(root, file))
	if err != nil {
		return ""
	}
	name := jsonNameRe.FindStringSubmatch(raw)
	if name == nil {
		return ""
	}
	return name[1]
}

// jsonNameRe reads the "name" field of a JSON manifest without pulling
// a JSON parser into the rules package. The key must open a value or
// follow { or , so dependency-map keys never match.
var jsonNameRe = regexp.MustCompile(`[{,\s]"name"\s*:\s*"([^"]+)"`)

// cargoName reads the [package] name of a Cargo.toml.
func cargoName(root string) string {
	raw, err := readManifest(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return ""
	}
	inPackage := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inPackage = trimmed == "[package]"
			continue
		}
		if !inPackage {
			continue
		}
		if m := cargoNameRe.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

var cargoNameRe = regexp.MustCompile(`^name\s*=\s*"([^"]+)"`)

// pyprojectName reads the [project] name of a pyproject.toml.
func pyprojectName(root string) string {
	raw, err := readManifest(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return ""
	}
	inProject := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inProject = trimmed == "[project]"
			continue
		}
		if !inProject {
			continue
		}
		if m := cargoNameRe.FindStringSubmatch(trimmed); m != nil {
			return m[1]
		}
	}
	return ""
}

// composerPrefixes reads psr-4 namespace prefixes from composer.json.
func composerPrefixes(root string) []string {
	raw, err := readManifest(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range psr4Re.FindAllStringSubmatch(raw, -1) {
		prefix := strings.TrimSuffix(strings.TrimPrefix(m[1], "\""), "\\")
		if prefix != "" {
			out = append(out, prefix)
		}
	}
	return out
}

// psr4Re reads the psr-4 map entries; composer namespaces carry a
// trailing backslash.
var psr4Re = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*:\s*"(?:[^"]*)"\s*,?\s*$`)

// pythonIdentities names the import roots of a Python project: the
// [project] name plus top-level directories that are real packages
// (carry __init__.py). Generic directory names never count.
func pythonIdentities(root string) []string {
	var out []string
	if n := pyprojectName(root); n != "" {
		out = append(out, n)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || pythonGenericDirs[e.Name()] {
			continue
		}
		init := filepath.Join(root, e.Name(), "__init__.py")
		info, err := os.Lstat(init)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// importEdge is one cross-project import occurrence.
type importEdge struct {
	file   string // project-relative source file
	spec   string // raw import specifier
	target string // project alias the specifier resolves to
}

// per-language import extractors. Each returns specifiers; the caller
// matches them against project identities.
var (
	tsImportRe       = regexp.MustCompile(`(?:\bfrom|\bimport\s*\(?\s*|\brequire\s*\(\s*)\s*['"]([^'"]+)['"]`)
	pyImportRe       = regexp.MustCompile(`(?:^|\n)\s*(?:from\s+([\w.]+)\s+import|import\s+([\w.]+(?:[ \t]*,[ \t]*[\w.]+)*))`)
	rsImportRe       = regexp.MustCompile(`(?:^|\n)\s*(?:pub\s+)?use\s+([\w:]+)`)
	cIncludeRe       = regexp.MustCompile(`^\s*#\s*include\s*(?:<([^>]+)>|"([^"]+)")`)
	javaImportRe     = regexp.MustCompile(`^\s*import\s+(?:static\s+)?([\w.]+)\s*;`)
	kotlinImportRe   = regexp.MustCompile(`^\s*import\s+([\w.]+)(?:\s+as\s+\w+)?\s*(?://.*)?$`)
	dartImportRe     = regexp.MustCompile(`^\s*(?:export\s+)?import\s+(?:"([^"]+)"|'([^']+)')`)
	pomGroupIdRe     = regexp.MustCompile(`<groupId>([^<]+)</groupId>`)
	pomArtifactIdRe  = regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	gradleRootNameRe = regexp.MustCompile(`rootProject\.name\s*=\s*["']([^"']+)['"]`)
	gradleGroupRe    = regexp.MustCompile(`^\s*group\s*(?:=\s*)?["']([^"']+)['"]`)
	pubspecNameRe    = regexp.MustCompile(`(?m)^name:\s*(\S+)`)
	rsCrateRe        = regexp.MustCompile(`extern\s+crate\s+([\w]+)`)
	phpImportRe      = regexp.MustCompile(`(?:^|\n)\s*use\s+(?:(?:function|const)\s+)?\\?([\w\\]+)`)
)

// importLang binds one extension family to its extractor and matcher.
type importLang struct {
	exts    map[string]bool
	extract func(data []byte) []string
	match   func(spec, name string) bool
}

var importLangs = map[string]*importLang{
	"go": {exts: setOf(".go"), extract: extractGoImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+"/") }},
	"typescript": {exts: setOf(".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"), extract: extractTsImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+"/") }},
	"python": {exts: setOf(".py"), extract: extractPyImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+".") }},
	"rust": {exts: setOf(".rs"), extract: extractRsImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+"::") }},
	"php": {exts: setOf(".php"), extract: extractPhpImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+"\\") }},
	"java": {exts: setOf(".java"), extract: extractJavaImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+".") }},
	"kotlin": {exts: setOf(".kt", ".kts"), extract: extractKotlinImports,
		match: func(spec, name string) bool { return spec == name || strings.HasPrefix(spec, name+".") }},
	"dart": {exts: setOf(".dart"), extract: extractDartImports,
		match: func(spec, name string) bool {
			return spec == name || dartNamespace(spec) == name || strings.HasPrefix(dartNamespace(spec), name+"/")
		}},
}

// dartNamespace strips the package: scheme so a pubspec name matches
// its imports; dart: URIs name the SDK and never a project.
func dartNamespace(spec string) string {
	return strings.TrimPrefix(spec, "package:")
}

// extractJavaImports reads java import statements, static forms
// included; on-demand wildcard imports (a.b.*) yield no edge because
// they name no single project.
func extractJavaImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(stripBlockComments(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, m := range javaImportRe.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// extractKotlinImports reads kotlin import statements; kotlin needs
// its own shape because it carries no trailing semicolon, and an alias
// keeps the imported name, not the alias. A trailing line comment is
// allowed.
func extractKotlinImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(stripBlockComments(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, m := range kotlinImportRe.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// extractDartImports reads import and export URIs. Relative URIs carry
// no package identity and yield nothing; only dart: and package: forms
// count.
func extractDartImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(stripBlockComments(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, m := range dartImportRe.FindAllStringSubmatch(line, -1) {
			uri := m[1]
			if m[2] != "" {
				uri = m[2]
			}
			if strings.HasPrefix(uri, "dart:") || strings.HasPrefix(uri, "package:") {
				out = append(out, uri)
			}
		}
	}
	return out
}

func setOf(exts ...string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// extractGoImports parses import declarations only: quoted strings in
// comments, errors, and test fixtures never become specifiers.
func extractGoImports(data []byte) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", data, parser.ImportsOnly|parser.SkipObjectResolution)
	if err != nil || f == nil {
		return nil
	}
	var out []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		out = append(out, path)
	}
	return out
}

// stripBlockComments removes /* */ spans so commented-out code stops
// yielding specifiers. Line comments are left alone: stripping `//`
// would eat URLs inside string imports.
func stripBlockComments(data []byte) string {
	re := regexp.MustCompile(`/\*.*?\*/`)
	return re.ReplaceAllLiteralString(string(data), "")
}

// extractTsImports scans line by line so a leading // line comment
// never yields a specifier; block comments are stripped first.
func extractTsImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(stripBlockComments(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		for _, m := range tsImportRe.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// extractPyImports handles single-line multi-imports (`import a, b`)
// and from-imports; the specifier class uses [ \t] so a capture never
// runs past the line.
func extractPyImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, m := range pyImportRe.FindAllStringSubmatch("\n"+line, -1) {
			if m[1] != "" {
				out = append(out, m[1])
			}
			if m[2] == "" {
				continue
			}
			for _, part := range strings.Split(m[2], ",") {
				if spec := strings.TrimSpace(part); spec != "" {
					out = append(out, strings.SplitN(spec, " as ", 2)[0])
				}
			}
		}
	}
	return out
}

func extractRsImports(data []byte) []string {
	var out []string
	for _, m := range rsImportRe.FindAllStringSubmatch(string(data), -1) {
		out = append(out, m[1])
	}
	for _, m := range rsCrateRe.FindAllStringSubmatch(string(data), -1) {
		out = append(out, m[1])
	}
	return out
}

func extractPhpImports(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, m := range phpImportRe.FindAllStringSubmatch("\n"+line, -1) {
			out = append(out, strings.TrimLeft(m[1], "\\"))
		}
	}
	return out
}

// scanImports walks the project root and resolves import specifiers
// against the other projects' identities. The longest matching name
// wins, so a nested module never also fires its parent; an identity
// name shared by several aliases is ambiguous and never fires.
func scanImports(root, self string, ident []projectIdentity, langs map[string]bool, otherRoots map[string]string) []importEdge {
	var active []*importLang
	for _, name := range slices.Sorted(maps.Keys(importLangs)) {
		if langs[name] {
			active = append(active, importLangs[name])
		}
	}
	if len(active) == 0 {
		return nil
	}
	// nameIndex maps one identity name to its owning aliases.
	nameIndex := map[string][]string{}
	for _, id := range ident {
		for _, n := range id.names {
			nameIndex[n] = append(nameIndex[n], id.alias)
		}
	}
	var out []importEdge
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if importDirSkip[d.Name()] || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
			// A nested registered project owns its own files.
			if otherRoots[filepath.Clean(p)] != "" && filepath.Clean(p) != filepath.Clean(root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		ext := filepath.Ext(p)
		for _, l := range active {
			if !l.exts[ext] {
				continue
			}
			data, err := readBounded(p)
			if err != nil {
				return nil
			}
			for _, spec := range l.extract([]byte(data)) {
				target := resolveSpecifier(spec, self, l.match, nameIndex)
				if target == "" {
					continue
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					rel = p
				}
				out = append(out, importEdge{file: filepath.ToSlash(rel), spec: spec, target: target})
			}
			break
		}
		return nil
	})
	return out
}

// resolveSpecifier picks the project an import specifier names.
func resolveSpecifier(spec, self string, match func(spec, name string) bool, nameIndex map[string][]string) string {
	best := ""
	var bestAliases []string
	for name, aliases := range nameIndex {
		if !match(spec, name) {
			continue
		}
		if len(name) > len(best) {
			best = name
			bestAliases = aliases
		}
	}
	if best == "" {
		return ""
	}
	// A self-import of the module prefix is never an edge.
	if slices.Contains(bestAliases, self) {
		return ""
	}
	// The same name registered under several aliases is ambiguous.
	if len(bestAliases) != 1 {
		return ""
	}
	return bestAliases[0]
}
