// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"
)

// goMachineSource is the canonical Go machine shape: typed const
// identifier keys and values in a package-level nested map, marked by
// a pylon:machine directive.
const goMachineSource = `package svc

type Status string
type Event string

const (
	StatusPending Status = "pending"
	StatusPicked  Status = "picked"
	StatusShipped Status = "shipped"
)

const (
	EventPick Event = "pick"
	EventShip Event = "ship"
)

// pylon:machine shipment
var shipmentTransitions = map[Status]map[Event]Status{
	StatusPending: {
		EventPick: StatusPicked,
	},
	StatusPicked: {
		EventShip: StatusShipped,
	},
}
`

func triplesOf(ts []parityTransition) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.from+"|"+t.event+"|"+t.to)
	}
	return out
}

func TestGoTransitionsExtract(t *testing.T) {
	ts, err := extractGoTransitions(goMachineSource, "shipment")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	want := []string{"pending|pick|picked", "picked|ship|shipped"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("triples = %v, want %v", got, want)
	}
}

func TestGoTransitionsMarkerIdentity(t *testing.T) {
	if _, err := extractGoTransitions(goMachineSource, "other"); err == nil || !strings.Contains(err.Error(), "shipment") {
		t.Fatalf("a wrong machine id must fail closed naming the id: %v", err)
	}
	noMarker := "package svc\n\nvar shipmentTransitions = map[string]map[string]string{}\n"
	if _, err := extractGoTransitions(noMarker, "shipment"); err == nil {
		t.Fatal("a map without the directive must fail closed")
	}
}

func TestGoTransitionsUnknownIdentFailsClosed(t *testing.T) {
	src := strings.Replace(goMachineSource, "StatusPicked: {\n\t\tEventShip: StatusShipped,",
		"StatusArchived: {\n\t\tEventShip: StatusShipped,", 1)
	if _, err := extractGoTransitions(src, "shipment"); err == nil || !strings.Contains(err.Error(), "StatusArchived") {
		t.Fatalf("an ident outside the const tables must fail closed: %v", err)
	}
}

func TestGoTransitionsNonIdentKeyFailsClosed(t *testing.T) {
	src := strings.Replace(goMachineSource, "StatusPending: {", "\"pending\": {", 1)
	if _, err := extractGoTransitions(src, "shipment"); err == nil {
		t.Fatal("a string-literal key must fail closed; the canonical shape is const identifiers only")
	}
}

func TestGoTransitionsDuplicateTripleSurvives(t *testing.T) {
	// Duplicate map keys parse but do not compile. The extractor must
	// return both triples so PYLON058 can judge them.
	src := strings.Replace(goMachineSource,
		"StatusPicked: {\n\t\tEventShip: StatusShipped,\n\t},",
		"StatusPending: {\n\t\tEventPick: StatusShipped,\n\t},", 1)
	ts, err := extractGoTransitions(src, "shipment")
	if err != nil {
		t.Fatalf("a non-compiling duplicate must still extract: %v", err)
	}
	if len(ts) != 2 {
		t.Fatalf("triples = %d, want both duplicates kept", len(ts))
	}
}

func TestGoTransitionsUnvaluedConstsCompareNames(t *testing.T) {
	src := strings.Replace(goMachineSource, ` Status = "pending"`, " Status = iota", 1)
	src = strings.Replace(src, ` Status = "picked"`, " Status", 1)
	src = strings.Replace(src, ` Status = "shipped"`, " Status", 1)
	src = strings.Replace(src, ` Event = "pick"`, " Event = iota", 1)
	src = strings.Replace(src, ` Event = "ship"`, " Event", 1)
	ts, err := extractGoTransitions(src, "shipment")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	want := []string{"StatusPending|EventPick|StatusPicked", "StatusPicked|EventShip|StatusShipped"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("an unvalued machine must compare case names, got %v", got)
	}
}

func TestGoTransitionsSecondMachineInFile(t *testing.T) {
	returns := `
const (
	EventReceive Event = "receive"
)

// pylon:machine returns
var returnsTransitions = map[Status]map[Event]Status{
	StatusPending: {
		EventReceive: StatusPicked,
	},
}
`
	ts, err := extractGoTransitions(goMachineSource+returns, "returns")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	if len(got) != 1 || got[0] != "pending|receive|picked" {
		t.Fatalf("the second machine must extract alone, got %v", got)
	}
}
