// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// parity rule ids: the set-checks subset of the parity machinery. The
// transition-graph extension lands separately.
const (
	parityNotEvaluatedID = "PYLON049" // advisory: a declared half without its other half, or an unresolvable enum
	parityCountID        = "PYLON050" // error: declared states count differs from the code enum
	parityStateID        = "PYLON051" // error: a state token appears on one side only
	parityRouteID        = "PYLON052" // advisory: a declared route path appears in no project source line
)

// lintParity compares declared tables against extracted code shapes.
// States: domain-entity docs declare a closed state set plus a
// states_symbol pointer at the backed enum in the mapped project.
// Routes: package docs declare route exposes; the mapped project
// source must mention the route path somewhere (a registration can
// carry method or framework specifics, so this check is advisory).
func lintParity(kbRoot string, cfg config.Config, allowExternal bool) []report.Finding {
	findings := parityStates(kbRoot, cfg, allowExternal)
	findings = append(findings, parityRoutes(kbRoot, cfg)...)
	findings = append(findings, machineParityFindings(kbRoot, cfg, allowExternal)...)
	return findings
}

// parityCase is one enum member: its identifier, its literal backing
// value when the language carries one, and whether a value was
// present (an empty string is a value too).
type parityCase struct {
	name   string
	value  string
	valued bool
}

// parityStates walks domain/ entities and checks the declared state
// sets against the enums the symbol pointers name.
func parityStates(kbRoot string, cfg config.Config, allowExternal bool) []report.Finding {
	var out []report.Finding
	_ = filepath.WalkDir(filepath.Join(kbRoot, "domain"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "README.md" || name == "AGENTS.md" || filepath.Ext(name) != ".md" || strings.HasPrefix(name, "._") {
			return nil
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(string(data), &meta) || meta.Type != "domain-entity" {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		out = append(out, checkEntityStates(kbRoot, rel, meta, cfg, allowExternal)...)
		return nil
	})
	return out
}

func checkEntityStates(kbRoot, rel string, meta resolve.DocMeta, cfg config.Config, allowExternal bool) []report.Finding {
	hasStates := meta.States.NonEmpty()
	hasSymbol := strings.TrimSpace(meta.StatesSymbol) != ""
	if hasStates != hasSymbol {
		missing := "states_symbol"
		if hasSymbol {
			missing = "states"
		}
		return []report.Finding{parityNotEvaluated(rel, fmt.Sprintf("the entity declares %s but not %s; parity needs both halves", missing, missing))}
	}
	if !hasStates {
		return nil
	}
	cases, err := resolveEnumCases(kbRoot, cfg, meta.StatesSymbol, allowExternal)
	if err != nil {
		return []report.Finding{parityNotEvaluated(rel, err.Error())}
	}
	return compareStates(rel, meta.States, cases)
}

// resolveEnumCases resolves a states_symbol repo:// pointer to the
// extracted enum cases through the project's parity profile. Every
// failure is not-evaluated: the caller grades it.
func resolveEnumCases(kbRoot string, cfg config.Config, symbol string, allowExternal bool) ([]parityCase, error) {
	u, ok := resolve.ParseURI(symbol)
	if !ok || u.Scheme != "repo" {
		return nil, fmt.Errorf("states_symbol is not a repo:// URI")
	}
	root := cfg.ProjectPath(kbRoot, u.Alias)
	if root == "" {
		return nil, fmt.Errorf("project %q is not registered", u.Alias)
	}
	filePart := uriSurfacePath(u.Path)
	enumName := ""
	if i := strings.Index(u.Path, "::"); i >= 0 {
		enumName = u.Path[i+2:]
	}
	target, safe := resolve.SafeJoin(root, filePart)
	if !safe {
		return nil, fmt.Errorf("states_symbol path %q is unsafe", filePart)
	}
	source, err := readBounded(target)
	if err != nil {
		return nil, fmt.Errorf("the states_symbol file %s is unreadable", filePart)
	}
	cases, err := extractWithProfile(cfg, u.Alias, filePart, source, enumName, kbRoot, allowExternal)
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no enum named %s found in %s; a skipped set must never read as parity", enumName, filePart)
	}
	return cases, nil
}

// compareStates: when every case carries a literal backing value the
// comparison set is the values, else the case identifiers. Determinism
// over guessing: a declared token must equal the extracted side
// exactly.
func compareStates(rel string, states resolve.StringOrList, cases []parityCase) []report.Finding {
	allValued := true
	for _, c := range cases {
		if !c.valued {
			allValued = false
			break
		}
	}
	extracted := make([]string, 0, len(cases))
	for _, c := range cases {
		if allValued {
			extracted = append(extracted, c.value)
		} else {
			extracted = append(extracted, c.name)
		}
	}
	declared := make([]string, 0, len(states))
	for _, s := range states {
		if strings.TrimSpace(s) != "" {
			declared = append(declared, s)
		}
	}
	var out []report.Finding
	if len(declared) != len(extracted) {
		out = append(out, errFinding(parityCountID, rel, 1,
			fmt.Sprintf("PARITY_COUNT_MISMATCH: %d declared states against %d enum cases", len(declared), len(extracted))))
	}
	var declOnly, codeOnly []string
	for _, s := range declared {
		if !slices.Contains(extracted, s) {
			declOnly = append(declOnly, s)
		}
	}
	for _, e := range extracted {
		if !slices.Contains(declared, e) {
			codeOnly = append(codeOnly, e)
		}
	}
	if len(declOnly) > 0 || len(codeOnly) > 0 {
		out = append(out, errFinding(parityStateID, rel, 1,
			fmt.Sprintf("PARITY_STATE_MISMATCH: declared only %q; code only %q", declOnly, codeOnly)))
	}
	return out
}

// parityRoutes checks each active package's route exposes against the
// project source: the declared path literal must appear somewhere.
func parityRoutes(kbRoot string, cfg config.Config) []report.Finding {
	routes := map[string][]routeDecl{} // alias -> declared routes with their anchor doc
	_ = filepath.WalkDir(filepath.Join(kbRoot, "packages"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "README.md" || name == "AGENTS.md" || filepath.Ext(name) != ".md" || strings.HasPrefix(name, "._") {
			return nil
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		content := string(data)
		var meta resolve.DocMeta
		if !resolve.ParseDocMeta(content, &meta) || meta.Type != "package" || meta.Status == "deprecated" || meta.Status == "retired" {
			return nil
		}
		var pkg resolve.PackageFrontMatter
		if !resolve.ParsePackageFrontMatter(content, &pkg) {
			return nil
		}
		rel, err := filepath.Rel(kbRoot, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, entry := range pkg.Exposes {
			kindStr, _ := entry["kind"].(string)
			if kindOr(kindStr) != "route" {
				continue
			}
			idStr, _ := entry["id"].(string)
			u, ok := resolve.ParseURI(idStr)
			if !ok || u.Scheme != "repo" || u.Path == "" {
				continue
			}
			routes[u.Alias] = append(routes[u.Alias], routeDecl{path: uriSurfacePath(u.Path), doc: rel})
		}
		return nil
	})
	var out []report.Finding
	for _, alias := range slices.Sorted(maps.Keys(routes)) {
		root := cfg.ProjectPath(kbRoot, alias)
		if root == "" {
			for _, r := range routes[alias] {
				out = append(out, parityNotEvaluated(r.doc, fmt.Sprintf("project %q is not registered", alias)))
			}
			continue
		}
		// One walk over deduped paths; a path shared by two packages
		// costs one scan, not two.
		paths := make([]string, 0, len(routes[alias]))
		for _, r := range routes[alias] {
			if !slices.Contains(paths, r.path) {
				paths = append(paths, r.path)
			}
		}
		found := scanRoutePaths(root, paths)
		for _, r := range routes[alias] {
			if !found[r.path] {
				out = append(out, advisoryFinding(parityRouteID, r.doc, 1,
					fmt.Sprintf("PARITY_ROUTE_MISMATCH: no source line in %s mentions the declared route path %s (dynamic registration is out of the extractor's reach)", alias, r.path)))
			}
		}
	}
	return out
}

type routeDecl struct {
	path, doc string
}

// scanRoutePaths walks the project once and records which declared
// route path literals appear in any source file. The mention can be a
// comment or a test; the heuristic is advisory by design.
func scanRoutePaths(root string, paths []string) map[string]bool {
	found := map[string]bool{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if importDirSkip[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if len(found) == len(paths) {
			return filepath.SkipAll
		}
		data, err := readBounded(p)
		if err != nil {
			return nil
		}
		content := string(data)
		for _, path := range paths {
			if !found[path] && strings.Contains(content, path) {
				found[path] = true
			}
		}
		return nil
	})
	return found
}

var (
	goConstStartRe = regexp.MustCompile(`(?m)^\s*const\s*\(`)
	goConstLineRe  = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?::?\s*([A-Za-z_][\w.]*))?\s*(?:=\s*(?:(iota)|"((?:[^"\\]|\\.)*)"))?\s*$`)
	declEnumRe     = regexp.MustCompile(`\benum\s+(\w+)`)
	rustMemberRe   = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=\s*[^,}\s]+)?\s*,?\s*$`)
	phpMemberRe    = regexp.MustCompile(`^\s*case\s+([A-Za-z_]\w*)\s*(?:=\s*['"]((?:[^'\\]|\\.)*)['"])?\s*;`)
	tsMemberRe     = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=\s*'((?:[^'\\]|\\.)*)'|=\s*"((?:[^"\\]|\\.)*)")?\s*,?\s*$`)
)

// builtinProfileOrder fixes the language claim precedence when one
// file extension is claimed by several languages (typescript keeps its
// legacy .js ownership over javascript). Explicit parity specs reject
// such claims at load; the order only governs the implicit path.
var builtinProfileOrder = []string{
	"go", "rust", "php", "typescript", "javascript", "python", "csharp", "ruby",
}

// builtinParityExtractors holds the in-process set-check extractors.
// A built-in language missing here has no shipped extractor yet.
var builtinParityExtractors = map[string]func(source, enumName string) ([]parityCase, error){
	"go": extractGoEnum,
	"rust": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum("enum", source, enumName, rustMemberRe)
	},
	"php": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum("enum", source, enumName, phpMemberRe)
	},
	"typescript": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum("enum", source, enumName, tsMemberRe)
	},
}

// parityProfileFor resolves the binding for a file: the first language
// in builtin claim order that both has a binding for the project and
// claims the file extension (canonical list plus profile additions).
func parityProfileFor(cfg config.Config, alias, file string) (string, config.ParityProfile, bool) {
	ext := strings.ToLower(filepath.Ext(file))
	bound := map[string]struct{}{}
	for _, lang := range append(slices.Clone(builtinProfileOrder), cfg.ParityLanguages(alias)...) {
		prof, ok := cfg.ParityProfileFor(alias, lang)
		if !ok {
			continue
		}
		bound[lang] = struct{}{}
		if slices.Contains(config.BuiltinProfileExtensions[lang], ext) || slices.Contains(prof.Extensions, ext) {
			return lang, prof, true
		}
	}
	return "", config.ParityProfile{}, false
}

// extractWithProfile pulls the case list for enumName through the
// resolved parity profile. A builtin without a shipped extractor and
// an unbound language are errors, never an empty parity pass.
func extractWithProfile(cfg config.Config, alias, file, source, enumName, kbRoot string, allowExternal bool) ([]parityCase, error) {
	lang, prof, bound := parityProfileFor(cfg, alias, file)
	if !bound {
		ext := strings.ToLower(filepath.Ext(file))
		var claimants []string
		for _, candidate := range builtinProfileOrder {
			if slices.Contains(config.BuiltinProfileExtensions[candidate], ext) {
				claimants = append(claimants, candidate)
			}
		}
		return nil, fmt.Errorf("no parity profile binds %s, which claims %s", strings.Join(claimants, " or "), file)
	}
	use, name, _ := strings.Cut(prof.Use, ":")
	switch use {
	case "builtin":
		extract, ok := builtinParityExtractors[name]
		if !ok {
			return nil, fmt.Errorf("the builtin parity profile for %s has no in-process extractor yet", name)
		}
		return extract(source, enumName)
	case "external":
		return extractExternalStates(cfg, alias, lang, enumName, source, prof, kbRoot, allowExternal)
	default:
		return nil, fmt.Errorf("unknown profile use %q", prof.Use)
	}
}

// extractGoEnum reads every const block: `const ( ... )` with manual
// paren matching, so comments and expressions containing ) cannot
// truncate capture. A case with a quoted value carries it; otherwise
// the identifier is the value. A non-blank, non-comment line inside a
// block that parses as nothing is an error, so a case the regex cannot
// read fails closed instead of silently dropping.
func extractGoEnum(source, enumName string) ([]parityCase, error) {
	var found bool
	var out []parityCase
	for _, m := range goConstStartRe.FindAllStringIndex(source, -1) {
		open := strings.Index(source[m[0]:], "(")
		body, ok := matchDelim(source[m[0]+open:], '(', ')')
		if !ok {
			continue
		}
		cases, err := parseGoConstBlock(body, enumName)
		if err != nil {
			return nil, err
		}
		if len(cases) > 0 {
			found = true
			out = cases
		}
	}
	if !found {
		return nil, fmt.Errorf("no const block declares cases of type %s", enumName)
	}
	return out, nil
}

func parseGoConstBlock(body, enumName string) ([]parityCase, error) {
	var out []parityCase
	current := ""
	for _, raw := range strings.Split(body, "\n") {
		line := stripLineComment(raw)
		line = strings.TrimSpace(line)
		if line == "" || line == "const (" || line == ")" {
			continue
		}
		m := goConstLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unreadable const case line %q", raw)
		}
		if m[1] == "_" {
			continue
		}
		if m[2] != "" {
			current = m[2]
		}
		if current != enumName {
			continue
		}
		valued := m[3] != "" || strings.Contains(line, "=")
		c := parityCase{name: m[1], valued: valued}
		if valued && m[3] == "" {
			c.value = m[4]
		}
		out = append(out, c)
	}
	return out, nil
}

// extractBracedEnum reads `enum Name { ... }` blocks for rust, php,
// and typescript via manual brace matching over every enum occurrence,
// so a later enum in the same file is reachable and serde attributes
// with braces cannot truncate the body. Only comment, attribute, and
// member lines parse; anything else fails closed.
func extractBracedEnum(keyword, source, enumName string, memberRe *regexp.Regexp) ([]parityCase, error) {
	var found bool
	var out []parityCase
	for _, m := range declEnumRe.FindAllStringSubmatchIndex(source, -1) {
		if source[m[2]:m[3]] != enumName {
			continue
		}
		open := strings.Index(source[m[1]:], "{")
		if open < 0 {
			continue
		}
		body, ok := matchDelim(source[m[1]+open:], '{', '}')
		if !ok {
			continue
		}
		cases, err := parseBracedBlock(body, memberRe, keyword)
		if err != nil {
			return nil, err
		}
		found = true
		out = cases
	}
	if !found {
		return nil, fmt.Errorf("no enum named %s found in the states_symbol file", enumName)
	}
	return out, nil
}

func parseBracedBlock(body string, memberRe *regexp.Regexp, keyword string) ([]parityCase, error) {
	var out []parityCase
	for _, raw := range strings.Split(body, "\n") {
		line := stripLineComment(raw)
		line = strings.TrimSpace(line)
		if line == "" || line == "{" || line == "}" || strings.HasPrefix(line, "#[") {
			continue
		}
		m := memberRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unreadable %s member line %q", keyword, raw)
		}
		c := parityCase{name: m[1]}
		// The value group position differs per member regex; the last
		// non-empty capture after the name carries the literal.
		for i := len(m) - 1; i > 1; i-- {
			if m[i] != "" {
				c.value = m[i]
				c.valued = true
				break
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// matchDelim returns the text between the first opener and its
// matching closer at depth zero.
func matchDelim(s string, open, close byte) (string, bool) {
	start := strings.IndexByte(s, open)
	if start < 0 {
		return "", false
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[start+1 : i], true
			}
		}
	}
	return "", false
}

// stripLineComment removes a trailing line comment that sits outside
// any string literal.
func stripLineComment(line string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '/':
			if i+1 < len(line) && line[i+1] == '/' && !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

// parityNotEvaluated is the fail-closed advisory: an entity whose
// parity cannot be judged must never read as parity.
func parityNotEvaluated(rel, msg string) report.Finding {
	return advisoryFinding(parityNotEvaluatedID, rel, 1, "PARITY_NOT_EVALUATED: "+msg)
}
