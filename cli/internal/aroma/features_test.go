// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package aroma_test

import (
	"github.com/estebanforge/pylons/cli/internal/aroma"
	"slices"
	"strings"
	"testing"
)

// The paper is "Aroma: Code Recommendation via Structural Code Search"
// (arXiv:1812.01158). Section, figure, table, and listing numbers are its.

// listing4 is Figure 4: the simplified parse tree of Listing 4, in the toy
// language's bracket notation with $ marking local variables.
const listing4 = `
[ if [ ( [ $view instanceof ViewGroup ] ) ]
  [ { [ for ( [ [ int [ $i = 0 ] ] ; [ $i < [ [ ( [ ( ViewGroup ) $view ] ) ] . [ getChildCount ( ) ] ] ] ; [ $i ++ ] ] )
        [ { [ [ View [ $innerView = [ [ ( [ ( ViewGroup ) $view ] ) ] . [ getChildAt ( $i ) ] ] ] ] ; ] } ] ] } ] ]
`

func snippet(t *testing.T, source string) aroma.Node {
	t.Helper()
	node, ok := Language.ParseSnippet(source, aroma.SnippetOptions{})
	if !ok || node == nil {
		t.Fatalf("parseSnippet(%q) returned nothing", source)
	}
	return node
}

func labelOf(t *testing.T, source string) string {
	t.Helper()
	switch tree := snippet(t, source).(type) {
	case *aroma.Tree:
		return tree.Label()
	default:
		t.Fatalf("label needs a tree, got a token")
		return ""
	}
}

func featuresOf(t *testing.T, source string) []string {
	t.Helper()
	var out []string
	for _, entry := range aroma.Featurize(snippet(t, source), aroma.FeaturizeOptions{}) {
		out = append(out, entry.Features...)
	}
	slices.Sort(out)
	return out
}

func TestLabelKeepsKeywordsWritesHashes(t *testing.T) {
	if got := labelOf(t, "[ x > [ y . f ] ]"); got != "#>#" {
		t.Fatalf("label = %q, want #>#", got)
	}
	if got := labelOf(t, listing4); got != "if##" {
		t.Fatalf("label(Listing 4) = %q, want if##", got)
	}
}

func TestFeaturizeExtractsTable2(t *testing.T) {
	tokens := aroma.Featurize(snippet(t, listing4), aroma.FeaturizeOptions{})
	var texts []string
	for _, entry := range tokens {
		texts = append(texts, entry.Token.Text)
	}
	wantTexts := []string{"view", "ViewGroup", "i", "0", "i", "ViewGroup", "view",
		"getChildCount", "i", "View", "innerView", "ViewGroup", "view", "getChildAt", "i"}
	if !slices.Equal(texts, wantTexts) {
		t.Fatalf("token texts = %v, want %v", texts, wantTexts)
	}

	// Table 2 prints the for-control label as "#;#;#;"; Figure 4 draws it as
	// "#;#;#".
	want0 := []string{
		aroma.TokenFeature("0"),
		aroma.ParentFeature("0", 2, "#=#"),
		aroma.ParentFeature("0", 1, "int#"),
		aroma.ParentFeature("0", 1, "#;#;#"),
		aroma.SiblingFeature(aroma.VAR, "0"),
		aroma.SiblingFeature("0", aroma.VAR),
	}
	slices.Sort(want0)
	got0 := slices.Clone(tokens[3].Features)
	slices.Sort(got0)
	if !slices.Equal(got0, want0) {
		t.Fatalf("features of 0_20:\n got %v\nwant %v", got0, want0)
	}

	wantView := []string{
		aroma.TokenFeature(aroma.VAR),
		aroma.ParentFeature(aroma.VAR, 2, "(#)#"),
		aroma.ParentFeature(aroma.VAR, 1, "(#)"),
		aroma.ParentFeature(aroma.VAR, 1, "#.#"),
		aroma.SiblingFeature("ViewGroup", aroma.VAR),
		aroma.SiblingFeature(aroma.VAR, "getChildCount"),
		aroma.UsageFeature(aroma.PositionContext(1, "#instanceof#"), aroma.PositionContext(2, "(#)#")),
		aroma.UsageFeature(aroma.PositionContext(2, "(#)#"), aroma.PositionContext(2, "(#)#")),
	}
	slices.Sort(wantView)
	gotView := slices.Clone(tokens[6].Features)
	slices.Sort(gotView)
	if !slices.Equal(gotView, wantView) {
		t.Fatalf("features of view_30:\n got %v\nwant %v", gotView, wantView)
	}
}

func TestFeaturizeRelatesConsecutiveUses(t *testing.T) {
	// `if (y < 0) x = -x;`: x is used as (1, #=#) and then as (1, -#); each
	// use records the pair, and y, used once, records none.
	source := "[ if [ ( [ $y < 0 ] ) ] [ [ $x = [ - $x ] ] ; ] ]"
	pair := aroma.UsageFeature(aroma.PositionContext(1, "#=#"), aroma.PositionContext(1, "-#"))
	var count int
	for _, feature := range featuresOf(t, source) {
		if feature == pair {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("usage pair occurs %d times, want 2", count)
	}
	tokens := aroma.Featurize(snippet(t, source), aroma.FeaturizeOptions{})
	want := []string{
		aroma.TokenFeature(aroma.VAR),
		aroma.ParentFeature(aroma.VAR, 1, "#<#"),
		aroma.ParentFeature(aroma.VAR, 1, "(#)"),
		aroma.ParentFeature(aroma.VAR, 1, "if##"),
		aroma.SiblingFeature(aroma.VAR, "0"),
	}
	slices.Sort(want)
	got := slices.Clone(tokens[0].Features)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("features of first use of y:\n got %v\nwant %v", got, want)
	}
}

func TestFeaturizeUsesMemberNameAsContext(t *testing.T) {
	// `x.foo(); x.bar();`: the context of x is foo, then bar, not (1, #.#).
	features := featuresOf(t, "[ [ $x . [ foo ( ) ] ] ; ] [ [ $x . [ bar ( ) ] ] ; ]")
	want := aroma.UsageFeature(aroma.TokenContext("foo"), aroma.TokenContext("bar"))
	if !slices.Contains(features, want) {
		t.Fatalf("features lack the member-context usage pair %q", want)
	}
	bad := aroma.UsageFeature(aroma.PositionContext(1, "#.#"), aroma.PositionContext(1, "#.#"))
	if slices.Contains(features, bad) {
		t.Fatalf("features carry the positional pair for a member access")
	}
}

func TestFeaturizeIgnoresLocalRenames(t *testing.T) {
	original := featuresOf(t, "[ [ $a = [ load ( $b ) ] ] ; ] [ [ save ( $a ) ] ; ]")
	renamed := featuresOf(t, "[ [ $out = [ load ( $src ) ] ] ; ] [ [ save ( $out ) ] ; ]")
	otherCall := featuresOf(t, "[ [ $a = [ fetch ( $b ) ] ] ; ] [ [ save ( $a ) ] ; ]")
	otherFlow := featuresOf(t, "[ [ $a = [ load ( $b ) ] ] ; ] [ [ save ( $b ) ] ; ]")
	if !slices.Equal(renamed, original) {
		t.Fatalf("renaming locals changed the features")
	}
	if slices.Equal(otherCall, original) {
		t.Fatalf("renaming a called method did not change the features")
	}
	if slices.Equal(otherFlow, original) {
		t.Fatalf("renaming a data flow did not change the features")
	}
}

func TestFeaturizeTellsSwappedConditionsApart(t *testing.T) {
	first := featuresOf(t, "[ if [ ( [ $x > 0 ] ) ] [ [ $z = 3 ] ; ] ]")
	second := featuresOf(t, "[ if [ ( [ $z > 3 ] ) ] [ [ $x = 0 ] ; ] ]")
	if !slices.Contains(first, aroma.ParentFeature("3", 2, "if##")) {
		t.Fatalf("first snippet lacks the if-body parent feature")
	}
	if slices.Contains(second, aroma.ParentFeature("3", 2, "if##")) {
		t.Fatalf("second snippet carries the other's if-body parent feature")
	}
	if slices.Equal(first, second) {
		t.Fatalf("swapped snippets featurize the same")
	}
}

func coreClip(t *testing.T, text string) string {
	t.Helper()
	// clip is unexported; reach it through a featurize of a long literal.
	tokens := aroma.Featurize(snippet(t, "[ [ save ( "+text+" ) ] ; ]"), aroma.FeaturizeOptions{})
	for _, entry := range tokens {
		for _, feature := range entry.Features {
			if name := strings.TrimPrefix(feature, "T\x1f"); name != feature && name != "save" {
				return name
			}
		}
	}
	t.Fatalf("no token feature found")
	return ""
}

func TestClipKeepsStartAndLength(t *testing.T) {
	long := make([]byte, 60)
	for i := range long {
		long[i] = byte('a' + i%26)
	}
	got := coreClip(t, string(long))
	if got[:48] != string(long[:48]) {
		t.Fatalf("clip start wrong")
	}
	if got[48:49] != "\x1f" {
		t.Fatalf("clip separator wrong")
	}
	if got[49:] != "60" {
		t.Fatalf("clip length = %q, want 60", got[49:])
	}
}
