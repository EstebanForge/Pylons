// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/report"
)

// assertNoParityFindings fails when any parity rule fires. An extractor
// error surfaces as PYLON049, so asserting only the absence of 050/051
// would pass a broken extractor.
func assertNoParityFindings(t *testing.T, findings []report.Finding) {
	t.Helper()
	for _, f := range findings {
		if f.RuleID == "PYLON049" || f.RuleID == "PYLON050" || f.RuleID == "PYLON051" {
			t.Fatalf("unexpected parity finding: %+v", f)
		}
	}
}

func assertParityRule(t *testing.T, kb, ruleID string) []report.Finding {
	t.Helper()
	findings := lintParity(kb, cfgMust(t, kb), false)
	for _, f := range findings {
		if f.RuleID == ruleID {
			return findings
		}
	}
	t.Fatalf("expected %s, got %+v", ruleID, findings)
	return findings
}

// --- import matcher direction: spec is the extracted import, name is
// --- a project identity; the identity is the namespace root.

func TestImportMatcherNamespaceDirection(t *testing.T) {
	javaMatch := importLangs["java"].match
	if !javaMatch("com.acme.lib.Util", "com.acme.lib") {
		t.Fatal("an import under a project's namespace must match the project")
	}
	if !javaMatch("com.acme.lib", "com.acme.lib") {
		t.Fatal("an import of the namespace root must match exactly")
	}
	if javaMatch("com.acme2.x", "com.acme") {
		t.Fatal("a sibling namespace must not match")
	}

	dartMatch := importLangs["dart"].match
	if !dartMatch("package:foo/bar.dart", "foo") {
		t.Fatal("a package import must match the pubspec name")
	}
	if !dartMatch("package:foo/src/deep.dart", "foo") {
		t.Fatal("a deep package import must match the pubspec name")
	}
	if dartMatch("package:foobar/x.dart", "foo") {
		t.Fatal("a package name that only shares a prefix must not match")
	}

	kotlinMatch := importLangs["kotlin"].match
	if !kotlinMatch("a.b.C", "a.b") {
		t.Fatal("a kotlin import under the identity must match")
	}
}

func TestExtractKotlinImports(t *testing.T) {
	data := []byte("import a.b.C\nimport a.b.D as E // side effect\nimport a.b.F;\n// import gone.F\n")
	got := extractKotlinImports(data)
	if len(got) != 3 || got[0] != "a.b.C" || got[1] != "a.b.D" || got[2] != "a.b.F" {
		t.Fatalf("extractKotlinImports = %v, want [a.b.C a.b.D a.b.F] (trailing semicolon and comments allowed)", got)
	}
}

func TestPomIdentityOneLine(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pom.xml"),
		"<project><groupId>com.acme</groupId><artifactId>lib</artifactId></project>")
	pom := pomIdentity(dir)
	if len(pom) != 1 || pom[0] != "com.acme.lib" {
		t.Fatalf("pomIdentity = %v, want [com.acme.lib] (tag order, not line order)", pom)
	}
}

// TestSqlDollarMaskStaysInLiveSQL pins the masking state machine: a
// $ pair inside a comment or a quoted string is text, and a $ that
// follows an identifier character belongs to that identifier, so only
// a token-boundary opener may mask.
func TestSqlDollarMaskStaysInLiveSQL(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"$$ inside a line comment is text",
			"-- uses $$ quoting\nCREATE TYPE s AS ENUM ('a');\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
		{"$$ inside a block comment is text",
			"/* $tag$ */\nCREATE TYPE s AS ENUM ('a');\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
		{"$$ inside a quoted string is text",
			"SELECT '$$';\nCREATE TYPE s AS ENUM ('a');\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
		{"a quoted value may contain $$",
			"CREATE TYPE s AS ENUM ('a$$');\nALTER TYPE s ADD VALUE 'b';\nSELECT '$$';\n",
			[]string{"a$$", "b"}},
		{"a $ after an identifier character never opens a mask",
			"CREATE TYPE s AS ENUM ('a');\nSELECT price$$ FROM t;\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
		{"a tag opener after digits is an identifier tail",
			"CREATE TYPE s AS ENUM ('a');\nSELECT 5$x$;\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
		{"a real dollar quote still masks comment syntax",
			"CREATE TYPE s AS ENUM ('a');\nDO $$ SELECT '--x' AS /*y*/; $$;\nALTER TYPE s ADD VALUE 'b';\n",
			[]string{"a", "b"}},
	}
	for _, tc := range cases {
		got, err := extractSqlEnum(tc.src, "s")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: extracted %v, want %v", tc.name, got, tc.want)
		}
		for i, w := range tc.want {
			if got[i].name != w {
				t.Fatalf("%s: extracted %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestSqlDollarQuotedCommentSurvives(t *testing.T) {
	src := "SELECT $$ keep /* markers $$;\nCREATE TYPE s AS ENUM ('a');\n-- */\nALTER TYPE s ADD VALUE 'b';\n"
	cases, err := extractSqlEnum(src, "s")
	if err != nil {
		t.Fatalf("comment markers inside a dollar-quoted string must stay inside it: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "a" || cases[1].name != "b" {
		t.Fatalf("extracted %+v, want a and b", cases)
	}
}

func TestJavaDartGradleIdentities(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pom.xml"),
		"<?xml version=\"1.0\"?>\n<project>\n  <parent>\n    <groupId>com.parent</groupId>\n    <artifactId>parent</artifactId>\n  </parent>\n  <groupId>com.acme</groupId>\n  <artifactId>lib</artifactId>\n  <dependencies>\n    <dependency>\n      <groupId>com.dep</groupId>\n      <artifactId>dep</artifactId>\n    </dependency>\n  </dependencies>\n</project>\n")
	pom := pomIdentity(dir)
	if len(pom) != 1 || pom[0] != "com.acme.lib" {
		t.Fatalf("pomIdentity = %v, want [com.acme.lib] (dependency and parent coordinates ignored, no bare artifact)", pom)
	}

	dir = t.TempDir()
	mustWrite(t, filepath.Join(dir, "settings.gradle"), "// rootProject.name = 'old'\nrootProject.name = 'svc'\n")
	mustWrite(t, filepath.Join(dir, "build.gradle"), "subprojects { group = 'sub.x' }\ngroup = 'com.acme'\n")
	gradle := gradleIdentity(dir)
	if len(gradle) != 1 || gradle[0] != "com.acme.svc" {
		t.Fatalf("gradleIdentity = %v, want [com.acme.svc] (subprojects block ignored)", gradle)
	}

	dir = t.TempDir()
	mustWrite(t, filepath.Join(dir, "pom.xml"),
		"<project>\n  <!-- <groupId>old</groupId> -->\n  <groupId>com.acme</groupId>\n  <artifactId>lib</artifactId>\n</project>\n")
	pom = pomIdentity(dir)
	if len(pom) != 1 || pom[0] != "com.acme.lib" {
		t.Fatalf("pomIdentity = %v, want [com.acme.lib] (xml comments ignored)", pom)
	}

	dir = t.TempDir()
	mustWrite(t, filepath.Join(dir, "pubspec.yaml"), "name: foo\n")
	dart := pubspecName(dir)
	if dart != "foo" {
		t.Fatalf("pubspecName = %q, want foo", dart)
	}
}

func TestGraphJavaAndDartEdgesFire(t *testing.T) {
	kb := t.TempDir()
	app := t.TempDir()
	lib := t.TempDir()
	dartlib := t.TempDir()
	mustWrite(t, filepath.Join(app, "pom.xml"),
		"<project>\n  <groupId>com.acme</groupId>\n  <artifactId>app</artifactId>\n</project>\n")
	mustWrite(t, filepath.Join(app, "App.java"), "import com.acme.lib.Tool;\nimport my.dart.lib.Thing;\n")
	mustWrite(t, filepath.Join(lib, "pom.xml"),
		"<project>\n  <groupId>com.acme</groupId>\n  <artifactId>lib</artifactId>\n</project>\n")
	mustWrite(t, filepath.Join(dartlib, "pubspec.yaml"), "name: my.dart.lib\n")

	pkg := func(alias, dir, deps string) string {
		return "  " + alias + ":\n    path: " + dir + "\n    ref: main\n"
	}
	manifest := "version: 1\nextensions: [java, dart]\nprojects:\n" +
		pkg("app", app, "") + pkg("lib", lib, "") + pkg("dartlib", dartlib, "")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), manifest)
	mustWrite(t, filepath.Join(kb, "packages/app.md"), pkgDocFrontmatter("app", "[app]", "[lib, dartlib]"))
	mustWrite(t, filepath.Join(kb, "packages/lib.md"), pkgDocFrontmatter("lib", "[lib]", "[]"))
	mustWrite(t, filepath.Join(kb, "packages/dartlib.md"), pkgDocFrontmatter("dartlib", "[dartlib]", "[]"))

	findings := lintImportBoundaries(kb, cfgMust(t, kb), nil)
	assertNoImportFindings(t, findings)
}

func assertNoImportFindings(t *testing.T, findings []report.Finding) {
	t.Helper()
	for _, f := range findings {
		if f.RuleID == "PYLON043" {
			t.Fatalf("unexpected import finding: %+v", f)
		}
	}
}

func pkgDocFrontmatter(name, components, deps string) string {
	return "---\ntitle: \"" + name + "\"\ntype: package\nstatus: accepted\ncreated: 2026-11-24\nupdated: 2026-11-24\npackage: " + name + "\ncomponents: " + components + "\ndepends_on: " + deps + "\ntags: [pkg]\n---\n\n# " + name + "\n"
}

// --- fail-closed coverage per language and shape.

func TestParityJavaMissingSemicolonFailsClosed(t *testing.T) {
	src := "public enum Status {\n  DRAFT,\n  SENT\n\n  boolean live() { return true; }\n}\n"
	kb, _ := newParityFixture(t, "java", src, "[DRAFT, SENT]", "repo://svc/State.java::Status")
	assertParityRule(t, kb, "PYLON049")
}

func TestParityKotlinEnhancedFailsClosed(t *testing.T) {
	src := "enum class Status {\n  DRAFT,\n  SENT;\n\n  fun live() = true\n}\n"
	kb, _ := newParityFixture(t, "kotlin", src, "[DRAFT, SENT]", "repo://svc/State.kt::Status")
	assertParityRule(t, kb, "PYLON049")
}

func TestParityDartConstructorArgsFailClosed(t *testing.T) {
	src := "enum Status {\n  draft(0),\n  sent(1);\n}\n"
	kb, _ := newParityFixture(t, "dart", src, "[draft, sent]", "repo://svc/state.dart::Status")
	assertParityRule(t, kb, "PYLON049")
}

func TestParitySqlUnreadableValueFailsClosed(t *testing.T) {
	src := "CREATE TYPE s AS ENUM (\n  'a',\n  draft\n);\n"
	kb, _ := newParityFixture(t, "sql", src, "[a, draft]", "repo://svc/schema.sql::s")
	assertParityRule(t, kb, "PYLON049")
}

func TestParityEmptyEnumBodyFailsClosed(t *testing.T) {
	// The first brace here is a lambda in the primary constructor, not
	// the body; an empty member parse must not read as parity.
	src := "enum class S(val f: () -> Unit = { }) {\n  A\n}\n"
	_, err := extractKotlinEnum(src, "S")
	if err == nil {
		t.Fatal("an empty member parse must fail closed")
	}
}

func TestCutAtSemicolonCommentSafe(t *testing.T) {
	body := "  A, // first; legacy\n  B,\n  /* tail; doc */\n  C;\n\n  void m() { ; }\n"
	cases, err := parseBracedBlock(cutAtSemicolon(body), javaMemberRe, "enum", false)
	if err != nil {
		t.Fatalf("comment-safe cut failed: %v", err)
	}
	if len(cases) != 3 || cases[0].name != "A" || cases[1].name != "B" || cases[2].name != "C" {
		t.Fatalf("comment semicolons dropped members: %+v", cases)
	}
}

func TestParityCFunctionDeclDoesNotPoison(t *testing.T) {
	src := "enum S {\n  A,\n  B\n};\nenum S parse(const char *s) {\n  return A;\n}\n"
	cases, err := extractCEnum(src, "S")
	if err != nil {
		t.Fatalf("function definition mentioning the enum poisoned extraction: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "A" || cases[1].name != "B" {
		t.Fatalf("extracted %+v, want A and B", cases)
	}
}

func TestParityCParameterDeclDoesNotPoison(t *testing.T) {
	src := "enum S {\n  A,\n  B\n};\nstatic int f(enum S s) {\n  return 1;\n}\n"
	cases, err := extractCEnum(src, "S")
	if err != nil {
		t.Fatalf("parameter mentioning the enum poisoned extraction: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("extracted %+v, want A and B", cases)
	}
}

func TestParityKotlinPrimaryConstructor(t *testing.T) {
	src := "enum class Status(val code: Int) {\n  DRAFT(1),\n  SENT(2)\n}\n"
	cases, err := extractKotlinEnum(src, "Status")
	if err != nil {
		t.Fatalf("a primary-constructor enum must extract: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "DRAFT" || cases[1].name != "SENT" {
		t.Fatalf("extracted %+v, want DRAFT and SENT", cases)
	}
}

func TestParityJavaQuotedCommentMarkersSurvive(t *testing.T) {
	src := "enum S {\n  A(\"/*\"),\n  B(\"*/\");\n\n  S(String s) { }\n}\n"
	cases, err := extractJavaEnum(src, "S")
	if err != nil {
		t.Fatalf("comment markers inside string arguments must survive: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "A" || cases[1].name != "B" {
		t.Fatalf("extracted %+v, want A and B", cases)
	}
}

// --- sql hardening.

func TestParitySqlBlockCommentedTypeIgnored(t *testing.T) {
	src := "/* CREATE TYPE s AS ENUM ('ghost'); */\nCREATE TYPE s AS ENUM ('a');\n"
	cases, err := extractSqlEnum(src, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].name != "a" {
		t.Fatalf("block-commented type contributed cases: %+v", cases)
	}
}

func TestParitySqlAmbiguousBareNameFailsClosed(t *testing.T) {
	src := "CREATE TYPE a.s AS ENUM ('x');\nCREATE TYPE b.s AS ENUM ('y');\n"
	if _, err := extractSqlEnum(src, "s"); err == nil {
		t.Fatal("two schema-qualified types under one bare name must fail closed")
	}
}

func TestParitySqlAlterAddValueFoldsIn(t *testing.T) {
	src := "CREATE TYPE s AS ENUM ('a');\nALTER TYPE s ADD VALUE 'b';\nALTER TYPE s ADD VALUE IF NOT EXISTS 'c';\n"
	cases, err := extractSqlEnum(src, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 || cases[0].name != "a" || cases[1].name != "b" || cases[2].name != "c" {
		t.Fatalf("ADD VALUE values missing: %+v", cases)
	}
}

func TestParitySqlMissingCommaFailsClosed(t *testing.T) {
	if _, err := extractSqlEnum("CREATE TYPE s AS ENUM ('a' 'b');", "s"); err == nil {
		t.Fatal("two values without a comma must fail closed, not merge")
	}
}

func TestParitySqlQuotedCommentMarkersSurvive(t *testing.T) {
	cases, err := extractSqlEnum("CREATE TYPE s AS ENUM ('a/*', 'b*/');", "s")
	if err != nil {
		t.Fatalf("comment markers inside values must survive: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "a/*" || cases[1].name != "b*/" {
		t.Fatalf("extracted %+v, want a/* and b*/", cases)
	}
}

func TestParitySqlEmptyValueListFailsClosed(t *testing.T) {
	if _, err := extractSqlEnum("CREATE TYPE s AS ENUM ();", "s"); err == nil {
		t.Fatal("an empty value list must fail closed")
	}
}

// --- deterministic validation error.

func TestParityExplicitDotHConflictErrorIsDeterministic(t *testing.T) {
	root := t.TempDir()
	manifest := "version: 1\nextensions: [c, cpp]\nparity:\n  version: 1\n  profiles:\n    c:\n      use: builtin:c\n    cpp:\n      use: builtin:cpp\n"
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifests", "pylons.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	first := ""
	for i := 0; i < 10; i++ {
		_, err := config.Load(root)
		if err == nil {
			t.Fatal("two profiles claiming .h must fail to load")
		}
		msg := err.Error()
		if !strings.Contains(msg, "both claim the .h extension") {
			t.Fatalf("wrong conflict message: %v", msg)
		}
		if strings.Index(msg, "\"cpp\"") < strings.Index(msg, "\"c\"") {
			t.Fatalf("conflict message must name keys in sorted order: %v", msg)
		}
		if i == 0 {
			first = msg
		} else if msg != first {
			t.Fatalf("conflict message is nondeterministic: %q vs %q", first, msg)
		}
	}
}
