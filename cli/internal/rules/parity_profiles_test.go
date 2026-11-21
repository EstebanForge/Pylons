// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"
)

const phpMachineSource = `<?php

enum ShipmentStatus: string
{
    case Pending = 'pending';
    case Picked = 'picked';
    case Shipped = 'shipped';
}

final class ShipmentTransitions
{
    // pylon:machine shipment
    public const MAP = [
        'pending' => [
            'pick' => 'picked',
        ],
        'picked' => [
            'ship' => 'shipped',
        ],
    ];
}
`

func TestPhpTransitionsExtract(t *testing.T) {
	ts, err := extractPhpTransitions(phpMachineSource, "shipment")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	want := []string{"pending|pick|picked", "picked|ship|shipped"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("triples = %v, want %v", got, want)
	}
}

func TestPhpTransitionsWrongMarker(t *testing.T) {
	if _, err := extractPhpTransitions(phpMachineSource, "returns"); err == nil || !strings.Contains(err.Error(), "shipment") {
		t.Fatalf("a wrong machine id must fail closed naming the file's id: %v", err)
	}
}

func TestPhpTransitionsDuplicateKeySurvives(t *testing.T) {
	src := strings.Replace(phpMachineSource,
		"'picked' => [\n            'ship' => 'shipped',\n        ],",
		"'pending' => [\n            'pick' => 'shipped',\n        ],", 1)
	ts, err := extractPhpTransitions(src, "shipment")
	if err != nil {
		t.Fatalf("a duplicate array key must still extract: %v", err)
	}
	if len(ts) != 2 {
		t.Fatalf("triples = %d, want both kept for the duplicate rule", len(ts))
	}
}

func TestPhpTransitionsNonLiteralFailsClosed(t *testing.T) {
	src := strings.Replace(phpMachineSource, "'pending' => [", "ShipmentStatus::Pending->value => [", 1)
	if _, err := extractPhpTransitions(src, "shipment"); err == nil {
		t.Fatal("a non-literal key must fail closed; the canonical php shape is quoted case values")
	}
}

const rustMachineSource = `pub enum Status {
    Pending,
    Picked,
    Shipped,
}

pub enum Event {
    Pick,
    Ship,
}

// pylon:machine shipment
pub fn transition(s: Status, e: Event) -> Status {
    match (s, e) {
        (Status::Pending, Event::Pick) => Status::Picked,
        (Status::Picked, Event::Ship) => Status::Shipped,
        _ => s,
    }
}
`

func TestRustTransitionsExtract(t *testing.T) {
	ts, err := extractRustTransitions(rustMachineSource, "shipment")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	want := []string{"Pending|Pick|Picked", "Picked|Ship|Shipped"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("triples = %v, want %v", got, want)
	}
}

func TestRustTransitionsWildcardSkipped(t *testing.T) {
	ts, err := extractRustTransitions(rustMachineSource, "shipment")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, tr := range ts {
		if tr.from == "_" || tr.to == "_" {
			t.Fatalf("the wildcard fallthrough arm must not become a triple: %+v", tr)
		}
	}
}

func TestRustTransitionsWrongMarker(t *testing.T) {
	if _, err := extractRustTransitions(rustMachineSource, "returns"); err == nil {
		t.Fatal("a wrong machine id must fail closed")
	}
}

func TestRustTransitionsSecondMachine(t *testing.T) {
	returns := `
// pylon:machine returns
pub fn transition_returns(s: Status, e: Event) -> Status {
    match (s, e) {
        (Status::Shipped, Event::Pick) => Status::Pending,
    }
}
`
	ts, err := extractRustTransitions(rustMachineSource+returns, "returns")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	got := triplesOf(ts)
	if len(got) != 1 || got[0] != "Shipped|Pick|Pending" {
		t.Fatalf("the second machine must extract alone, got %v", got)
	}
}
