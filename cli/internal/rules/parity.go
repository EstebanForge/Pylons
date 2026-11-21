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
func lintParity(kbRoot string, cfg config.Config) []report.Finding {
	findings := parityStates(kbRoot, cfg)
	findings = append(findings, parityRoutes(kbRoot, cfg)...)
	return findings
}

// parityCase is one enum member: its identifier and, when the language
// carries one, its literal backing value.
type parityCase struct {
	name  string
	value string
}

// parityStates walks domain/ entities and checks the declared state
// sets against the enums the symbol pointers name.
func parityStates(kbRoot string, cfg config.Config) []report.Finding {
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
		out = append(out, checkEntityStates(kbRoot, rel, meta, cfg)...)
		return nil
	})
	return out
}

func checkEntityStates(kbRoot, rel string, meta resolve.DocMeta, cfg config.Config) []report.Finding {
	hasStates := meta.States.NonEmpty()
	hasSymbol := strings.TrimSpace(meta.StatesSymbol) != ""
	if hasStates != hasSymbol {
		missing := "states_symbol"
		if hasSymbol {
			missing = "states"
		}
		return []report.Finding{advisoryFinding(parityNotEvaluatedID, rel, 1,
			fmt.Sprintf("PARITY_NOT_EVALUATED: the entity declares %s but not %s; parity needs both halves", missingOr(hasStates, missing), otherHalf(hasStates, missing)))}
	}
	if !hasStates {
		return nil
	}
	u, ok := resolve.ParseURI(meta.StatesSymbol)
	if !ok || u.Scheme != "repo" {
		return []report.Finding{parityNotEvaluated(rel, "states_symbol is not a repo:// URI")}
	}
	root := cfg.ProjectPath(kbRoot, u.Alias)
	if root == "" {
		return []report.Finding{parityNotEvaluated(rel, fmt.Sprintf("project %q is not registered", u.Alias))}
	}
	filePart := uriSurfacePath(u.Path)
	enumName := ""
	if i := strings.Index(u.Path, "::"); i >= 0 {
		enumName = u.Path[i+2:]
	}
	target, ok := resolve.SafeJoin(root, filePart)
	if !ok {
		return []report.Finding{parityNotEvaluated(rel, fmt.Sprintf("states_symbol path %q is unsafe", filePart))}
	}
	source, err := readBounded(target)
	if err != nil {
		return []report.Finding{parityNotEvaluated(rel, fmt.Sprintf("the states_symbol file %s is unreadable", filePart))}
	}
	cases, err := extractEnum(enabledLanguages(cfg, u.Alias), source, enumName, filePart)
	if err != nil {
		return []report.Finding{parityNotEvaluated(rel, err.Error())}
	}
	if len(cases) == 0 {
		return []report.Finding{parityNotEvaluated(rel, fmt.Sprintf("no enum named %s found in %s; a skipped set must never read as parity", enumName, filePart))}
	}
	return compareStates(rel, meta.States, cases)
}

func missingOr(hasStates bool, missing string) string {
	if hasStates {
		return "states"
	}
	return missing
}

func otherHalf(hasStates bool, missing string) string {
	if hasStates {
		return missing
	}
	return "states"
}

func parityNotEvaluated(rel, msg string) report.Finding {
	return advisoryFinding(parityNotEvaluatedID, rel, 1, "PARITY_NOT_EVALUATED: "+msg)
}

// compareStates: when every case carries a literal backing value the
// comparison set is the values, else the case identifiers. Determinism
// over guessing: a declared token must equal the extracted side
// exactly.
func compareStates(rel string, states resolve.StringOrList, cases []parityCase) []report.Finding {
	allValued := true
	for _, c := range cases {
		if c.value == "" {
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
			fmt.Sprintf("PARITY_STATE_MISMATCH: declared only %v; code only %v", declOnly, codeOnly)))
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
		paths := make([]string, 0, len(routes[alias]))
		for _, r := range routes[alias] {
			paths = append(paths, r.path)
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
// route path literals appear in any source file.
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
		for _, path := range paths {
			if !found[path] && strings.Contains(string(data), path) {
				found[path] = true
			}
		}
		return nil
	})
	return found
}

var (
	goConstBlockRe   = regexp.MustCompile(`(?s)const\s*\(([^)]*)\)`)
	goConstLineRe    = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?::?\s*([A-Za-z_][\w.]*))?\s*(?:=\s*(?:(iota)|"((?:[^"\\]|\\.)*)"))?\s*$`)
	declBlockRe      = regexp.MustCompile(`(?s)\benum\s+(\w+)[^{]*\{([^}]*)\}`)
	goCaseValueRe    = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	rustMemberRe     = regexp.MustCompile(`(?m)^\s*([A-Za-z_]\w*)\s*(?:=\s*"((?:[^"\\]|\\.)*)")?\s*[,{]`)
	phpMemberRe      = regexp.MustCompile(`(?m)^\s*case\s+([A-Za-z_]\w*)\s*(?:=\s*'((?:[^'\\]|\\.)*)')?\s*;`)
	tsMemberRe       = regexp.MustCompile(`(?m)^\s*([A-Za-z_]\w*)\s*(?:=\s*"((?:[^"\\]|\\.)*)")?\s*,?\s*$`)
	phpEnumKeywordRe = regexp.MustCompile(`(?s)\benum\s+(\w+)\b[^{]*\{([^}]*)\}`)
)

// extractEnum pulls the case list for enumName out of the source file
// using the language extractors enabled for the project. An enum that
// cannot be resolved is an error, never an empty parity pass.
func extractEnum(langs map[string]bool, source, enumName, file string) ([]parityCase, error) {
	ext := strings.ToLower(filepath.Ext(file))
	if langs["go"] && ext == ".go" {
		return extractGoEnum(source, enumName)
	}
	if langs["rust"] && ext == ".rs" {
		return extractBracedEnum(declBlockRe, source, enumName, rustMemberRe)
	}
	if langs["php"] && ext == ".php" {
		return extractBracedEnum(phpEnumKeywordRe, source, enumName, phpMemberRe)
	}
	if langs["typescript"] && (ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx") {
		return extractBracedEnum(declBlockRe, source, enumName, tsMemberRe)
	}
	return nil, fmt.Errorf("no enabled language extractor can read %s", file)
}

// extractGoEnum reads typed const blocks: `const ( Name Type = ... )`.
// A case with a quoted value carries it; otherwise the identifier is
// the value.
func extractGoEnum(source, enumName string) ([]parityCase, error) {
	block := goConstBlockRe.FindString(source)
	if block == "" {
		return nil, fmt.Errorf("no const block found in the states_symbol file")
	}
	var out []parityCase
	current := ""
	for _, line := range strings.Split(block, "\n") {
		m := goConstLineRe.FindStringSubmatch(line)
		if m == nil || strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if m[2] != "" {
			current = m[2]
		}
		if current != enumName {
			continue
		}
		if m[4] != "" {
			out = append(out, parityCase{name: m[1], value: m[4]})
			continue
		}
		out = append(out, parityCase{name: m[1]})
	}
	return out, nil
}

// extractBracedEnum reads `enum Name { ... }` blocks for rust, php, and
// typescript via per-language member regexes.
func extractBracedEnum(blockRe *regexp.Regexp, source, enumName string, memberRe *regexp.Regexp) ([]parityCase, error) {
	m := blockRe.FindStringSubmatch(source)
	if m == nil || m[1] != enumName {
		return nil, fmt.Errorf("no enum named %s found in the states_symbol file", enumName)
	}
	var out []parityCase
	for _, line := range strings.Split(m[2], "\n") {
		mm := memberRe.FindStringSubmatch(line)
		if mm == nil {
			continue
		}
		out = append(out, parityCase{name: mm[1], value: mm[2]})
	}
	return out, nil
}

// advisoryFinding mirrors errFinding at advisory grade.
func advisoryFinding(ruleID, file string, line int, msg string) report.Finding {
	return report.Finding{
		RuleID:   ruleID,
		Severity: report.SeverityAdvisory,
		Grade:    report.GradeAdvisory,
		File:     file,
		Line:     line,
		Message:  msg,
	}
}
