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
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
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
	// ParseURI strips the ::fragment from Path, so the enum name comes
	// from the raw pointer.
	_, enumName, _ := strings.Cut(symbol, "::")
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
		// An empty string is a legitimate valued state; only pure
		// whitespace junk is dropped.
		if s != "" && strings.TrimSpace(s) == "" {
			continue
		}
		declared = append(declared, s)
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
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") {
				return filepath.SkipDir
			}
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
			if importDirSkip[d.Name()] || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
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
	goConstStartRe     = regexp.MustCompile(`(?m)^\s*const\s*\(`)
	goConstLineRe      = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?::?\s*([A-Za-z_][\w.]*))?\s*(?:=\s*(?:(iota)|"((?:[^"\\]|\\.)*)"|` + "`" + `((?:[^` + "`" + `])*)` + "`" + `))?\s*$`)
	declEnumRe         = regexp.MustCompile(`\benum\s+(\w+)`)
	cEnumDeclRe        = regexp.MustCompile(`\benum\s+(?:class\s+|struct\s+)?(\w+)`)
	kotlinEnumDeclRe   = regexp.MustCompile(`\benum\s+class\s+(\w+)(?:\s*:\s*[\w<>. ]+)?`)
	rustMemberRe       = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=\s*[^,}\s]+)?\s*,?\s*$`)
	javaMemberRe       = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:\([^)]*\))?\s*,?\s*$`)
	sqlEnumDeclRe      = regexp.MustCompile(`(?i)\bcreate\s+type\s+(\w+(?:\.\w+)?)\s+as\s+enum\s*\(`)
	sqlAlterAddValueRe = regexp.MustCompile(`(?i)\balter\s+type\s+(\w+(?:\.\w+)?)\s+add\s+value\s+(?:if\s+not\s+exists\s+)?'((?:[^']|'')*)'`)
	phpMemberRe        = regexp.MustCompile(`^\s*case\s+([A-Za-z_]\w*)\s*(?:=\s*['"]((?:[^'\\]|\\.)*)['"]|=\s*(-?\d+))?\s*;`)
	tsMemberRe         = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=\s*'((?:[^'\\]|\\.)*)'|=\s*"((?:[^"\\]|\\.)*)"|=\s*(-?\d+))?\s*,?\s*$`)
)

// builtinProfileOrder fixes the language claim precedence when one
// file extension is claimed by several languages (typescript keeps its
// legacy .js ownership over javascript, and cpp keeps the shared .h
// header over c). Explicit parity specs reject such claims at load;
// the order only governs the implicit path.
var builtinProfileOrder = []string{
	"go", "rust", "php", "typescript", "javascript", "python", "csharp", "ruby",
	"cpp", "c", "java", "kotlin", "dart", "sql",
}

// builtinParityExtractors holds the in-process set-check extractors.
// A built-in language missing here has no shipped extractor yet.
var builtinParityExtractors = map[string]func(source, enumName string) ([]parityCase, error){
	"go": extractGoEnum,
	"rust": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum(declEnumRe, "enum", source, enumName, rustMemberRe, false, false, nil)
	},
	"php": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum(declEnumRe, "enum", source, enumName, phpMemberRe, false, true, nil)
	},
	"typescript": func(source, enumName string) ([]parityCase, error) {
		return extractBracedEnum(declEnumRe, "enum", source, enumName, tsMemberRe, false, false, nil)
	},
	"c":      extractCEnum,
	"cpp":    extractCEnum,
	"java":   extractJavaEnum,
	"kotlin": extractKotlinEnum,
	"dart":   extractDartEnum,
	"sql":    extractSqlEnum,
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
		body, ok := matchDelim(source[m[0]+open:], '(', ')', false)
		if !ok {
			continue
		}
		cases, err := parseGoConstBlock(body, enumName)
		if err != nil {
			return nil, err
		}
		if len(cases) > 0 {
			found = true
			out = append(out, cases...)
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
			if c.value == "" {
				c.value = m[5]
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// extractBracedEnum finds every declaration matching declRe whose name
// group equals enumName, matches the body braces, and parses the
// members one per line. stopAtSemicolon handles enum families whose
// constant list ends at a semicolon with method bodies after it (java,
// dart): everything before the first unquoted semicolon survives, and
// the cut is comment-aware so a comment semicolon never eats members.
// Anything else fails closed.
func extractBracedEnum(declRe *regexp.Regexp, keyword, source, enumName string, memberRe *regexp.Regexp, stopAtSemicolon, hashComments bool, declGuard func(between string) bool) ([]parityCase, error) {
	var found bool
	var out []parityCase
	for _, m := range declRe.FindAllStringSubmatchIndex(source, -1) {
		if source[m[2]:m[3]] != enumName {
			continue
		}
		open := strings.Index(source[m[1]:], "{")
		if open < 0 {
			continue
		}
		// A statement that ends with a semicolon before any brace is a
		// braceless reference (a C field declaration, a C++ opaque
		// forward declaration), not a definition. A language-specific
		// guard can reject more shapes; parsing either against a later
		// scope's braces poisons the real definition.
		between := source[m[1] : m[1]+open]
		if strings.IndexByte(between, ';') >= 0 {
			continue
		}
		if declGuard != nil && declGuard(between) {
			continue
		}
		body, ok := matchDelim(source[m[1]+open:], '{', '}', hashComments)
		if !ok {
			continue
		}
		if stopAtSemicolon {
			body = cutAtSemicolon(body)
		}
		cases, err := parseBracedBlock(body, memberRe, keyword, hashComments)
		if err != nil {
			return nil, err
		}
		if len(cases) == 0 {
			return nil, fmt.Errorf("enum %s parsed to zero members; the canonical shape is one member per line", enumName)
		}
		found = true
		out = append(out, cases...)
	}
	if !found {
		return nil, fmt.Errorf("no enum named %s found in the states_symbol file", enumName)
	}
	return out, nil
}

// cutAtSemicolon drops everything from the first unquoted semicolon
// on: the members of a java or dart enum body all sit before it, and
// the method bodies after it are not member lines. Comments are
// stripped quote-aware first, so a semicolon inside a comment never
// truncates the member list, and a comment marker inside a string
// never eats members.
func cutAtSemicolon(body string) string {
	stripped := stripCCommentsQuoteAware(body)
	var b strings.Builder
	for _, line := range strings.Split(stripped, "\n") {
		b.WriteString(stripLineComment(line))
		b.WriteString("\n")
	}
	stripped = b.String()
	var q byte
	for i := 0; i < len(stripped); i++ {
		c := stripped[i]
		if q != 0 {
			if c == '\\' {
				i++
			} else if c == q {
				q = 0
			}
			continue
		}
		switch {
		case c == '\'', c == '"':
			q = c
		case c == ';':
			return stripped[:i]
		}
	}
	return stripped
}

// stripCCommentsQuoteAware removes block comments that sit outside any
// string or char literal, so a comment marker inside a literal
// survives and a semicolon inside a comment cannot cut a member list.
func stripCCommentsQuoteAware(text string) string {
	var b strings.Builder
	var q byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if q != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(text) {
				i++
				if i < len(text) {
					b.WriteByte(text[i])
				}
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			q = c
			b.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				break
			}
			i += 2 + end + 1
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// extractCEnum reads plain C enumerators and C++ enum class/struct
// declarations (including a backing-type clause) with the same
// one-member-per-line canonical shape as rust. A parenthesis between
// the declaration and the body brace is a function whose signature or
// parameter list mentions the enum, never a definition. An anonymous
// typedef enum has no name at the declaration, so it fails closed.
func extractCEnum(source, enumName string) ([]parityCase, error) {
	return extractBracedEnum(cEnumDeclRe, "enum", source, enumName, rustMemberRe, false, false, cDeclGuard)
}

// cDeclGuard rejects a match whose text to the body brace holds a
// parenthesis: the enum name appears inside a signature or parameter
// list, not as a definition. Kotlin keeps no guard, because a primary
// constructor between the declaration and the body is its main form.
func cDeclGuard(between string) bool {
	return strings.ContainsAny(between, "()")
}

// extractJavaEnum reads an enum constant list with optional constructor
// arguments, stopping at the constant-list semicolon.
func extractJavaEnum(source, enumName string) ([]parityCase, error) {
	return extractBracedEnum(declEnumRe, "enum", source, enumName, javaMemberRe, true, false, nil)
}

// extractKotlinEnum reads enum class entries, primary-constructor
// enums included; entries with bodies or nested call arguments are
// unreadable and fail closed.
func extractKotlinEnum(source, enumName string) ([]parityCase, error) {
	return extractBracedEnum(kotlinEnumDeclRe, "enum class", source, enumName, javaMemberRe, false, false, nil)
}

// extractDartEnum reads plain and enhanced enums; the enhanced form's
// semicolon ends the members exactly like java's.
func extractDartEnum(source, enumName string) ([]parityCase, error) {
	return extractBracedEnum(declEnumRe, "enum", source, enumName, tsMemberRe, true, false, nil)
}

// extractSqlEnum reads Postgres CREATE TYPE ... AS ENUM statements.
// The values are quoted strings and carry the case name verbatim
// (doubled quotes unescape); the type name matches with or without a
// schema prefix and case-insensitively, because unquoted SQL
// identifiers fold. MySQL column ENUM(...) is a column option, not a
// named set, so it has no extractor.
func extractSqlEnum(source, enumName string) ([]parityCase, error) {
	clean := stripSQLLineComments(source)
	clean = stripCCommentsQuoteAware(clean)
	matches := sqlEnumDeclRe.FindAllStringSubmatchIndex(clean, -1)
	// A bare name that two schema-qualified declarations answer is
	// ambiguous; unioning their values would read as parity on a set
	// no single type defines.
	ambiguous := 0
	for _, m := range matches {
		name := clean[m[2]:m[3]]
		if strings.EqualFold(name, enumName) || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(enumName)) {
			ambiguous++
		}
	}
	if ambiguous > 1 {
		return nil, fmt.Errorf("enum %s is ambiguous: several schema-qualified declarations match the bare name", enumName)
	}
	var found bool
	var out []parityCase
	for _, m := range matches {
		name := clean[m[2]:m[3]]
		if !strings.EqualFold(name, enumName) && !strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(enumName)) {
			continue
		}
		body, ok := matchDelim(clean[m[1]-1:], '(', ')', false)
		if !ok {
			continue
		}
		found = true
		for _, seg := range splitSqlValues(body) {
			seg = strings.TrimSpace(seg)
			if seg == "" || seg == "(" || seg == ")" {
				continue
			}
			if len(seg) < 2 || seg[0] != '\'' || seg[len(seg)-1] != '\'' || !sqlQuotedInterior(seg[1:len(seg)-1]) {
				return nil, fmt.Errorf("unreadable sql enum value %q; the canonical shape is comma-separated quoted values", seg)
			}
			val := strings.ReplaceAll(seg[1:len(seg)-1], "''", "'")
			out = append(out, parityCase{name: val, value: val, valued: true})
		}
	}
	if !found {
		return nil, fmt.Errorf("no enum named %s found in the states_symbol file", enumName)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("enum %s declares no values; the canonical shape is comma-separated quoted values", enumName)
	}
	// Postgres migrations extend an enum after creation; ADD VALUE is
	// the only ALTER verb that changes the closed set, so it folds in.
	for _, m := range sqlAlterAddValueRe.FindAllStringSubmatch(clean, -1) {
		if !strings.EqualFold(m[1], enumName) && !strings.HasSuffix(strings.ToLower(m[1]), "."+strings.ToLower(enumName)) {
			continue
		}
		val := strings.ReplaceAll(m[2], "''", "'")
		out = append(out, parityCase{name: val, value: val, valued: true})
	}
	return out, nil
}

// stripSQLLineComments removes -- comments that sit outside any string
// literal, one line at a time.
func stripSQLLineComments(source string) string {
	var b strings.Builder
	for _, line := range strings.Split(source, "\n") {
		inSingle := false
		cut := len(line)
		for i := 0; i < len(line); i++ {
			switch {
			case inSingle && line[i] == '\'':
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			case !inSingle && line[i] == '\'':
				inSingle = true
			case !inSingle && line[i] == '-' && i+1 < len(line) && line[i+1] == '-':
				cut = i
				i = len(line)
			}
		}
		b.WriteString(line[:cut])
		b.WriteString("\n")
	}
	return b.String()
}

// splitSqlValues splits an enum value list on commas that sit outside
// single quotes, so one-line and one-per-line declarations both parse.
func splitSqlValues(body string) []string {
	var segs []string
	var b strings.Builder
	inQ := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inQ {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(body) && body[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				inQ = false
			}
			continue
		}
		if c == '\'' {
			inQ = true
			b.WriteByte(c)
			continue
		}
		if c == ',' {
			segs = append(segs, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	segs = append(segs, b.String())
	return segs
}

// sqlQuotedInterior checks the text inside the outer quotes of one
// value: every quote must be a doubled escape, so a missing comma
// between two values fails closed instead of merging them.
func sqlQuotedInterior(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '\'' {
			continue
		}
		if i+1 >= len(s) || s[i+1] != '\'' {
			return false
		}
		i++
	}
	return true
}

func parseBracedBlock(body string, memberRe *regexp.Regexp, keyword string, hashComments bool) ([]parityCase, error) {
	var out []parityCase
	for _, raw := range strings.Split(body, "\n") {
		line := stripLineComment(raw)
		if hashComments {
			line = stripHashComment(line)
		}
		line, ok := stripAttributes(line)
		if !ok {
			return nil, fmt.Errorf("an attribute in %s does not close on its line; put one member per line", keyword)
		}
		line = strings.TrimSpace(line)
		if line == "" || line == "{" || line == "}" {
			continue
		}
		m := memberRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unreadable %s member line %q; the canonical shape is one member per line", keyword, raw)
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

// stripHashComment removes a # line comment that sits outside any
// string literal, for languages whose comment grammar owns # (php).
func stripHashComment(line string) string {
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
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

// stripAttributes removes leading #[...] attributes from a line,
// string-aware so a bracket inside a literal stays put. The second
// return is false when an attribute never closes on the line: fail
// closed rather than silently eating the member behind it.
func stripAttributes(line string) (string, bool) {
	for {
		t := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(t, "#[") {
			return t, true
		}
		depth, inQ, closed := 0, byte(0), false
		i := 0
		for ; i < len(t); i++ {
			c := t[i]
			if inQ != 0 {
				if c == '\\' && inQ != '`' {
					i++
					continue
				}
				if c == inQ {
					inQ = 0
				}
				continue
			}
			switch {
			case c == '"' || c == '`':
				inQ = c
			case c == '\'':
				if end := indexUnescaped(t, i+1, '\''); end >= 0 {
					i = end
				}
			case c == '[':
				depth++
			case c == ']':
				depth--
				if depth == 0 {
					closed = true
				}
			}
			if closed {
				i++
				break
			}
		}
		if !closed {
			return t, false
		}
		line = t[i:]
	}
}

// matchDelim returns the text between the first opener and its
// matching closer at depth zero.
func matchDelim(s string, open, close byte, hashComments bool) (string, bool) {
	start := strings.IndexByte(s, open)
	if start < 0 {
		return "", false
	}
	depth := 0
	// Quoted spans and comments never count their delimiters: a serde
	// rename holding a brace, a backtick literal, a PHP string, or a
	// comment carrying an apostrophe would otherwise end or corrupt the
	// scan. A single quote counts as a string only when it closes on the
	// same line; an unterminated one is an apostrophe, a rust lifetime,
	// or comment prose and stays inert.
	var q byte
	var lineComment, blockComment bool
	for i := start; i < len(s); i++ {
		c := s[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if q != 0 {
			if c == '\\' && q != '`' && i+1 < len(s) {
				i++
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		switch {
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			lineComment = true
			i++
		case c == '#':
			// Only languages whose comment grammar owns #; elsewhere a
			// bare # is syntax (rust attributes, raw strings) and must
			// not swallow the rest of the line.
			if hashComments {
				lineComment = true
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			blockComment = true
			i++
		case c == '\'':
			if end := indexUnescaped(s, i+1, '\''); end >= 0 {
				i = end
			}
		case c == '"', c == '`':
			q = c
		case c == open:
			depth++
		case c == close:
			depth--
			if depth == 0 {
				return s[start+1 : i], true
			}
		}
	}
	return "", false
}

// indexUnescaped finds the first unescaped needle at or after from,
// bounded to the needle's own line: an escaped quote defers to the next
// one, and -1 means the line carries no closer.
func indexUnescaped(s string, from int, needle byte) int {
	nl := strings.IndexByte(s[from:], '\n')
	end := len(s)
	if nl >= 0 {
		end = from + nl
	}
	for i := from; i < end; i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == needle {
			return i
		}
	}
	return -1
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
