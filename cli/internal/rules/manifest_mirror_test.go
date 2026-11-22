// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
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

func TestFrontmatterKeysMirrorMatchesEngine(t *testing.T) {
	// vocabulary.json carries the per-type frontmatter key registry;
	// it must mirror resolve.TemplateKeys or the advisory lies.
	data, err := os.ReadFile("../../../manifests/vocabulary.json")
	if err != nil {
		t.Fatalf("read vocabulary.json: %v", err)
	}
	var doc struct {
		FrontmatterKeys map[string][]string `json:"frontmatter_keys"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse vocabulary.json: %v", err)
	}
	if len(doc.FrontmatterKeys) != len(resolve.TemplateKeys) {
		t.Fatalf("doc-type count drifted: vocabulary has %d types, engine has %d",
			len(doc.FrontmatterKeys), len(resolve.TemplateKeys))
	}
	for docType, engineKeys := range resolve.TemplateKeys {
		got, ok := doc.FrontmatterKeys[docType]
		if !ok {
			t.Errorf("vocabulary.json is missing doc type %s", docType)
			continue
		}
		want := append([]string(nil), engineKeys...)
		slices.Sort(want)
		have := append([]string(nil), got...)
		slices.Sort(have)
		if !slices.Equal(want, have) {
			t.Errorf("doc type %s keys drifted: engine %v, vocabulary %v", docType, engineKeys, got)
		}
	}
}
