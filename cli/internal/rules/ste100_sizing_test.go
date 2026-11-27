// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

// STE100 governs prose. Symbol-only tokens and the structured sizing
// tails ledgers append to task bullets ("- dev 4 + qa 0 + review 1 =
// **5**") are data, not sentences, and must not count.
func TestCheckSTE100IgnoresSizingTailsAndSymbols(t *testing.T) {
	rel := "plans/x.md"

	cases := []struct {
		name string
		body string
		flag bool
	}{
		// 16 prose words + an 11-token sizing tail: flags while the
		// tail counts, passes once the tail is treated as data.
		{"sizing tail is not prose", "- Draft the full page map after the spike findings land in the conventions zone review - dev 4 + qa 0 + review 1 = **5**\n", false},
		// 19 words plus "= done": flags while symbols count (21).
		{"symbol-only tokens do not count", "- " + repWords(19) + " = done\n", false},
		{"tail with dense body still fits", "- T16 Agree target folder and delivery - dev 1 + qa 0 + review 2 = **3**\n", false},
		{"long prose still flags", "The system will never under any circumstances allow a request to bypass the configured permission boundary checks during startup, even when the request itself carries a valid internal token.\n", true},
		{"long bullet without tail still flags", "- " + repWords(21) + "\n", true},
		{"tail does not rescue long prose", "- T1 " + repWords(20) + " dev 4 + qa 0 + review 1 = **5**\n", true},
	}
	for _, tc := range cases {
		got := checkSTE100(rel, tc.body)
		if tc.flag && len(got) == 0 {
			t.Fatalf("%s: expected a finding, got none", tc.name)
		}
		if !tc.flag && len(got) != 0 {
			t.Fatalf("%s: expected no finding, got %v", tc.name, got)
		}
	}

	// The preview keeps the raw leading tokens (a bullet dash counts
	// in the head even though it is dropped from the word count), so
	// baseline keys recorded by older engines still match.
	flagged := checkSTE100(rel, "- "+repWords(20)+" = done\n")
	if len(flagged) != 1 {
		t.Fatalf("expected one finding for the 21-word bullet, got %d", len(flagged))
	}
	if want := "sentence exceeds 20 words under ASD-STE100: - word word word word word word word"; flagged[0].Message != want {
		t.Fatalf("preview drifted from the stable format:\n got: %s\nwant: %s", flagged[0].Message, want)
	}
}

func repWords(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "word "
	}
	return s[:len(s)-1]
}
