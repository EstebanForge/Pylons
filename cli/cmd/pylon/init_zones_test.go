package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Every top-level doc zone ships an AGENTS.md entry point, so a fresh KB does
// not advise PYLON034 the moment a user drops a document in a zone.
func TestInitSeedsZoneAgentGuides(t *testing.T) {
	dir := t.TempDir()
	if code := cmdInit(dir); code != 0 {
		t.Fatalf("cmdInit exit = %d", code)
	}
	for _, zone := range []string{
		"packages", "proposals", "plans", "decisions", "quirks",
		"domain", "investigations", "audits", "references", "tickets",
		"prompts", "skills", "conventions",
	} {
		if _, err := os.Stat(filepath.Join(dir, zone, "AGENTS.md")); err != nil {
			t.Errorf("%s/AGENTS.md missing: %v", zone, err)
		}
	}
}
