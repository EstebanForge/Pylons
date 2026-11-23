// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"
)

const tsMachineFixture = `// pylon:machine order
export function next(s: State, e: Event): State {
  switch (s) {
    case "pending":
      switch (e) {
        case "pay":
          return "paid";
        case "cancel":
          return "cancelled";
        default:
          throw new Error("unknown event");
      }
    case "paid":
      switch (e) {
        case "refund":
          return "refunded";
      }
    case "refunded":
      return s;
  }
}
`

func tsTriples(t *testing.T, source, id string) []parityTransition {
	t.Helper()
	out, err := extractTSTransitions(source, id)
	if err != nil {
		t.Fatalf("extractTSTransitions: %v", err)
	}
	return out
}

func TestTSTransitionsNestedSwitch(t *testing.T) {
	got := tsTriples(t, tsMachineFixture, "order")
	want := []parityTransition{
		{from: "pending", event: "pay", to: "paid"},
		{from: "pending", event: "cancel", to: "cancelled"},
		{from: "paid", event: "refund", to: "refunded"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d triples, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("triple %d: want %+v, got %+v", i, w, got[i])
		}
	}
}

func TestTSTransitionsSingleQuotes(t *testing.T) {
	src := strings.ReplaceAll(tsMachineFixture, `"`, `'`)
	got := tsTriples(t, src, "order")
	if len(got) != 3 || got[0] != (parityTransition{from: "pending", event: "pay", to: "paid"}) {
		t.Fatalf("single-quoted triples diverged: %+v", got)
	}
}

func TestTSTransitionsAbsorbAndDefaultSkipped(t *testing.T) {
	got := tsTriples(t, tsMachineFixture, "order")
	for _, tr := range got {
		if tr.from == "refunded" || tr.event == "" {
			t.Fatalf("absorb or default leaked a triple: %+v", tr)
		}
	}
}

func TestTSTransitionsArrowReducer(t *testing.T) {
	src := `// pylon:machine sync
const next = (s: SyncState, e: SyncEvent): SyncState => {
  switch (s) {
    case 'idle':
      switch (e) {
        case 'pull':
          return "pulling";
      }
  }
};
`
	got := tsTriples(t, src, "sync")
	if len(got) != 1 || got[0] != (parityTransition{from: "idle", event: "pull", to: "pulling"}) {
		t.Fatalf("arrow reducer triples: %+v", got)
	}
}

func TestTSTransitionsSecondMachineIsolated(t *testing.T) {
	src := tsMachineFixture + "\n// pylon:machine other\nfunction n(s: S, e: E): S {\n  switch (s) {\n    case 'a':\n      switch (e) {\n        case 'go':\n          return \"b\";\n      }\n  }\n}\n"
	got := tsTriples(t, src, "other")
	if len(got) != 1 || got[0].to != "b" {
		t.Fatalf("second machine triples: %+v", got)
	}
}

func TestTSTransitionsWrongMarker(t *testing.T) {
	_, err := extractTSTransitions(tsMachineFixture, "shipping")
	if err == nil || !strings.Contains(err.Error(), "carries order") {
		t.Fatalf("wrong marker must name the carried id: %v", err)
	}
}

func TestTSTransitionsEmptySwitchFails(t *testing.T) {
	src := "// pylon:machine empty\nfunction next(s: S, e: E): S {\n  switch (s) {\n  }\n}\n"
	if _, err := extractTSTransitions(src, "empty"); err == nil {
		t.Fatal("an empty reducer must fail closed")
	}
}

func TestTSTransitionsNonLiteralReturnFails(t *testing.T) {
	src := strings.Replace(tsMachineFixture, `return "paid";`, `return route(e);`, 1)
	_, err := extractTSTransitions(src, "order")
	if err == nil || !strings.Contains(err.Error(), "computed") {
		t.Fatalf("a computed return inside an event case must fail closed: %v", err)
	}
}

func TestTSTransitionsGarbageFailsClosed(t *testing.T) {
	src := strings.Replace(tsMachineFixture, `    case "paid":`, "    if (s === S.Paid) {", 1)
	if _, err := extractTSTransitions(src, "order"); err == nil {
		t.Fatal("unrecognized reducer logic must fail closed")
	}
}

func TestTSTransitionsCastReturnFails(t *testing.T) {
	src := strings.Replace(tsMachineFixture, `return "paid";`, `return "paid" as State;`, 1)
	if _, err := extractTSTransitions(src, "order"); err == nil {
		t.Fatal("a cast return must fail closed; declare the literal only")
	}
}
