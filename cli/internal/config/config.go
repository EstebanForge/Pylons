// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// Config holds the parsed pylons.yaml registry.
type Config struct {
	Version    int                      `yaml:"version"`
	Extensions []string                 `yaml:"extensions"`
	Protocols  map[string]string        `yaml:"protocols"`
	Projects   map[string]ProjectConfig `yaml:"projects"`
}

// ProjectConfig describes one tracked repository or project.
type ProjectConfig struct {
	Path       string   `yaml:"path"`
	Repo       string   `yaml:"repo"`
	Ref        string   `yaml:"ref"`
	Tier       string   `yaml:"tier"`
	Extensions []string `yaml:"extensions"`
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
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
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
func (c Config) ProjectRef(alias string) string {
	proj, ok := c.Projects[alias]
	if !ok || proj.Ref == "" {
		return "main"
	}
	return proj.Ref
}
