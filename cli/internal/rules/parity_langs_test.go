// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParityCEnum(t *testing.T) {
	src := "/* status codes */\nenum Status {\n  DRAFT,\n  SENT = 5,\n};\n"
	kb, _ := newParityFixture(t, "c", src, "[DRAFT, SENT]", "repo://svc/state.c::Status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParityCppEnumClass(t *testing.T) {
	src := "enum class Status : uint8_t {\n  Draft,\n  Sent,\n};\n"
	kb, _ := newParityFixture(t, "cpp", src, "[Draft, Sent]", "repo://svc/state.cpp::Status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParityJavaEnum(t *testing.T) {
	src := "public enum Status {\n  DRAFT,\n  SENT(1);\n\n  private final int code;\n\n  Status(int code) {\n    this.code = code;\n  }\n}\n"
	kb, _ := newParityFixture(t, "java", src, "[DRAFT, SENT]", "repo://svc/State.java::Status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParityKotlinEnum(t *testing.T) {
	src := "enum class Status {\n  DRAFT,\n  SENT,\n}\n"
	kb, _ := newParityFixture(t, "kotlin", src, "[DRAFT, SENT]", "repo://svc/State.kt::Status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParityDartEnum(t *testing.T) {
	src := "enum Status {\n  draft,\n  sent;\n\n  bool get isLive => this == Status.sent;\n}\n"
	kb, _ := newParityFixture(t, "dart", src, "[draft, sent]", "repo://svc/state.dart::Status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParitySqlEnum(t *testing.T) {
	src := "-- lifecycle\nCREATE TYPE shipment_status AS ENUM (\n  'draft',\n  'sent'\n);\n"
	kb, _ := newParityFixture(t, "sql", src, "[draft, sent]", "repo://svc/schema.sql::shipment_status")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParitySqlEnumSchemaQualified(t *testing.T) {
	src := "CREATE TYPE public.mood AS ENUM ('sad', 'ok');\n"
	kb, _ := newParityFixture(t, "sql", src, "[sad, ok]", "repo://svc/schema.sql::mood")
	assertNoParityFindings(t, lintParity(kb, cfgMust(t, kb), false))
}

func TestParityDotHClaimFollowsBindingOrder(t *testing.T) {
	kbBoth, root1 := newParityFixture(t, "cpp", "", "[Draft, Sent]", "repo://svc/state.h::Status")
	mustWrite(t, filepath.Join(kbBoth, "manifests/pylons.yaml"),
		"version: 1\nextensions: [cpp, c]\nprojects:\n  svc:\n    path: "+root1+"\n    ref: main\n")
	lang, _, bound := parityProfileFor(cfgMust(t, kbBoth), "svc", filepath.Join(root1, "state.h"))
	if !bound || lang != "cpp" {
		t.Fatalf("with both c and cpp bound, .h must claim cpp, got %q bound=%v", lang, bound)
	}

	kbCOnly, root2 := newParityFixture(t, "c", "", "[DRAFT, SENT]", "repo://svc/state.h::Status")
	lang, _, bound = parityProfileFor(cfgMust(t, kbCOnly), "svc", filepath.Join(root2, "state.h"))
	if !bound || lang != "c" {
		t.Fatalf("with only c bound, .h must claim c, got %q bound=%v", lang, bound)
	}
}

func TestExtractJavaImports(t *testing.T) {
	data := []byte("import java.util.List;\nimport static java.util.Map.of;\n// import gone.Foo;\n")
	got := extractJavaImports(data)
	want := []string{"java.util.List", "java.util.Map.of"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("extractJavaImports = %v, want %v", got, want)
	}
}

func TestExtractDartImports(t *testing.T) {
	data := []byte("import 'package:foo/bar.dart';\nimport \"dart:async\";\nimport 'src/relative.dart';\n// import 'package:gone/x.dart';\n")
	got := extractDartImports(data)
	want := []string{"package:foo/bar.dart", "dart:async"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("extractDartImports = %v, want %v", got, want)
	}
}

func TestParityBracelessEnumRefDoesNotPoison(t *testing.T) {
	// A C header declares the enum type on fields long before the
	// definition; the braceless reference must be skipped, not parsed
	// against some later scope's braces.
	src := "struct Config {\n  enum Status status;\n  int n;\n};\n\nenum Status {\n  DRAFT,\n  SENT\n};\n"
	cases, err := extractCEnum(src, "Status")
	if err != nil {
		t.Fatalf("braceless enum reference poisoned the definition: %v", err)
	}
	if len(cases) != 2 || cases[0].name != "DRAFT" || cases[1].name != "SENT" {
		t.Fatalf("extracted %+#v, want DRAFT and SENT", cases)
	}
}

func TestParityLangsFailClosedOnUnreadableMember(t *testing.T) {
	src := "enum Status {\n  DRAFT,\n  if (x) { y }\n};\n"
	_, err := extractCEnum(src, "Status")
	if err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("an unreadable C member must fail closed, got %v", err)
	}
}
