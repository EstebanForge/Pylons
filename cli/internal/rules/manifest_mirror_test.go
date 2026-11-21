// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
)

// The JSON registries are documentation mirrors of sets the engine
// compiles in. When they drift, docs lie; this test is the drift
// alarm.
func TestVocabularyMirrorMatchesEngine(t *testing.T) {
	data, err := os.ReadFile("../../../manifests/vocabulary.json")
	if err != nil {
		t.Fatalf("read vocabulary.json: %v", err)
	}
	var doc struct {
		Status map[string][]string `json:"status"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse vocabulary.json: %v", err)
	}
	for zone, allowed := range allowedStatuses {
		got, ok := doc.Status[zone]
		if !ok {
			t.Errorf("vocabulary.json status map is missing zone %s", zone)
			continue
		}
		for _, status := range allowed {
			if !contains(got, status) {
				t.Errorf("vocabulary.json zone %s is missing status %s", zone, status)
			}
		}
	}
}

func TestParityMirrorMatchesEngine(t *testing.T) {
	data, err := os.ReadFile("../../../manifests/parity.json")
	if err != nil {
		t.Fatalf("read parity.json: %v", err)
	}
	var doc struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse parity.json: %v", err)
	}
	if len(config.SupportedParityVersions) == 0 {
		t.Fatal("engine declares no parity versions")
	}
	if doc.Version == nil || *doc.Version != config.SupportedParityVersions[0] {
		t.Errorf("parity.json version = %v, want the shipped engine version %v", doc.Version, config.SupportedParityVersions[0])
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
