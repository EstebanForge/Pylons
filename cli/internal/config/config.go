// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Extension names become pylon-ext-<name> binaries; slashes or dots would
// escape extDir or dodge PATH lookup.
var extensionNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// schemeNameRe is the URI scheme grammar, lowercase. Reserved
// schemes are the engine's own plus the external-link classes the
// link checker handles directly.
var schemeNameRe = regexp.MustCompile(`^[a-z][a-z0-9+.\-]*$`)

var reservedSchemes = []string{"repo", "pylon", "http", "https", "mailto", "file"}

// decisionPrefixRe is the accepted-decision filename prefix grammar:
// one to four uppercase letters.
var decisionPrefixRe = regexp.MustCompile(`^[A-Z]{1,4}$`)

// Config holds the parsed pylons.yaml registry.
type Config struct {
	Version    int               `yaml:"version"`
	Extensions []string          `yaml:"extensions"`
	Protocols  map[string]string `yaml:"protocols"`
	Parity     *ParityConfig     `yaml:"parity"`
	Generate   *[]string         `yaml:"generate"`
	Schemes    []string          `yaml:"schemes"`
	Exclude    []string          `yaml:"exclude"`
	// DecisionPrefix names accepted-decision files: <prefix>NNNN-slug.md.
	// Absent means D.
	DecisionPrefix string                   `yaml:"decision_prefix"`
	Projects       map[string]ProjectConfig `yaml:"projects"`
}

// GeneratedManifests is the closed set of manifests the gate hooks
// regenerate and check.
var GeneratedManifests = []string{"rules", "packages"}

// GeneratesManifest reports whether the gates maintain the named
// manifest. Absent generate means all of them; an explicit list is
// exact, so adopters can hand a manifest over to foreign tooling.
func (c Config) GeneratesManifest(name string) bool {
	if c.Generate == nil {
		return true
	}
	for _, g := range *c.Generate {
		if g == name {
			return true
		}
	}
	return false
}

// DecisionPrefixOrDefault is the accepted-decision filename prefix:
// the configured value, else D.
func (c Config) DecisionPrefixOrDefault() string {
	if c.DecisionPrefix == "" {
		return "D"
	}
	return c.DecisionPrefix
}

// ProjectConfig describes one tracked repository or project.
type ProjectConfig struct {
	Path       string        `yaml:"path"`
	Repo       string        `yaml:"repo"`
	Ref        string        `yaml:"ref"`
	Tier       string        `yaml:"tier"`
	Extensions []string      `yaml:"extensions"`
	Parity     *ParityConfig `yaml:"parity"`
}

// Load reads manifests/pylons.yaml from root. A missing file returns a default config.
func Load(root string) (Config, error) {
	cfg := Config{
		Version:  1,
		Projects: map[string]ProjectConfig{},
	}
	p := filepath.Join(root, "manifests", "pylons.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	// The parity subtree is closed-schema: re-decode it from a generic
	// map so unknown keys error instead of silently disabling checks.
	var generic map[string]any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return cfg, err
	}
	if raw, ok := generic["parity"]; ok {
		m, ok := raw.(map[string]any)
		if !ok {
			return cfg, errors.New("parity must be a map")
		}
		cfg.Parity, err = parityFromMap("parity", m)
		if err != nil {
			return cfg, err
		}
	}
	for alias, proj := range cfg.Projects {
		projRaw, ok := generic["projects"].(map[string]any)
		if !ok {
			continue
		}
		entry, ok := projRaw[alias].(map[string]any)
		if !ok {
			continue
		}
		if raw, ok := entry["parity"]; ok {
			m, ok := raw.(map[string]any)
			if !ok {
				return cfg, errors.New("parity for project " + alias + " must be a map")
			}
			proj.Parity, err = parityFromMap("projects."+alias+".parity", m)
			if err != nil {
				return cfg, err
			}
			cfg.Projects[alias] = proj
		}
	}
	if err := validateParitySpecs(&cfg); err != nil {
		return cfg, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
	}
	for _, pat := range cfg.Exclude {
		if pat == "" || strings.HasPrefix(pat, "/") {
			return cfg, errors.New("exclude: pattern must be a KB-relative glob: " + pat)
		}
		for _, seg := range strings.Split(pat, "/") {
			if seg == ".." {
				return cfg, errors.New("exclude: pattern must not traverse: " + pat)
			}
		}
	}
	for _, sc := range cfg.Schemes {
		if !schemeNameRe.MatchString(sc) {
			return cfg, errors.New("schemes: invalid scheme name " + sc)
		}
		if slices.Contains(reservedSchemes, sc) {
			return cfg, errors.New("schemes: " + sc + " is built in and cannot be redeclared")
		}
	}
	if cfg.DecisionPrefix != "" && !decisionPrefixRe.MatchString(cfg.DecisionPrefix) {
		return cfg, errors.New("decision_prefix: must be one to four uppercase letters, got " + cfg.DecisionPrefix)
	}
	if cfg.Generate != nil {
		for _, g := range *cfg.Generate {
			known := false
			for _, m := range GeneratedManifests {
				if g == m {
					known = true
					break
				}
			}
			if !known {
				return cfg, errors.New("generate: unknown manifest " + g + " (known: rules, packages)")
			}
		}
	}
	for _, ext := range cfg.Extensions {
		if !extensionNameRe.MatchString(ext) {
			return cfg, errors.New("invalid extension name: " + ext)
		}
	}
	for alias, proj := range cfg.Projects {
		for _, ext := range proj.Extensions {
			if !extensionNameRe.MatchString(ext) {
				return cfg, errors.New("invalid extension name for project " + alias + ": " + ext)
			}
		}
	}
	return cfg, nil
}

// HasExtension reports whether ext is enabled globally or for alias.
func (c Config) HasExtension(alias, ext string) bool {
	if slices.Contains(c.Extensions, ext) {
		return true
	}
	if alias == "" {
		return false
	}
	proj, ok := c.Projects[alias]
	if !ok {
		return false
	}
	return slices.Contains(proj.Extensions, ext)
}

// ProjectExtensions returns the deduplicated list of extensions active for alias.
func (c Config) ProjectExtensions(alias string) []string {
	seen := make(map[string]struct{}, len(c.Extensions))
	out := make([]string, 0, len(c.Extensions))
	for _, ext := range c.Extensions {
		if _, ok := seen[ext]; !ok {
			seen[ext] = struct{}{}
			out = append(out, ext)
		}
	}
	if proj, ok := c.Projects[alias]; ok {
		for _, ext := range proj.Extensions {
			if _, ok := seen[ext]; !ok {
				seen[ext] = struct{}{}
				out = append(out, ext)
			}
		}
	}
	return out
}

// ProjectPath resolves the local filesystem path for alias relative to root.
func (c Config) ProjectPath(root, alias string) string {
	proj, ok := c.Projects[alias]
	if !ok || proj.Path == "" {
		return ""
	}
	if filepath.IsAbs(proj.Path) {
		return filepath.Clean(proj.Path)
	}
	return filepath.Clean(filepath.Join(root, proj.Path))
}

// ProjectRef returns the configured base branch for alias, defaulting to main.
// ProjectRef returns the ref contract resolution reads. An unset
// ref means the working tree: the checker judges the code the user
// is about to commit, symmetric with parity and merge-danger. Set
// ref: to pin reads to a committed tree instead.
func (c Config) ProjectRef(alias string) string {
	return c.Projects[alias].Ref
}
