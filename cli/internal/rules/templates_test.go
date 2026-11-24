package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every template ships to users as the schema for new documents; a YAML
// typo here breaks every future copy. AGENTS.md is the folder how-to and
// carries no frontmatter.
func TestTemplateFrontmatterParses(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "pylons", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("templates/ not reachable from this checkout: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "AGENTS.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		parts := strings.SplitN(string(raw), "---", 3)
		if len(parts) < 3 {
			t.Errorf("%s: missing frontmatter block", e.Name())
			continue
		}
		var fm map[string]any
		if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
			t.Errorf("%s: invalid frontmatter YAML: %v", e.Name(), err)
			continue
		}
		for _, key := range []string{"title", "type", "status", "created", "updated", "owners", "tags"} {
			if _, ok := fm[key]; !ok {
				t.Errorf("%s: frontmatter missing required key %q", e.Name(), key)
			}
		}
	}
}
