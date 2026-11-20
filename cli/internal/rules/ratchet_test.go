package rules

import "testing"

func TestAdvisoryRatchetExcludesImportEdges(t *testing.T) {
	// PYLON043 moves with sibling repo state, not with the corpus:
	// it must stay out of the budget so a sibling edit cannot fail
	// the gate with zero corpus edits.
	if !clockDrivenRules["PYLON043"] {
		t.Fatal("PYLON043 must be excluded from the ratchet budget")
	}
}
