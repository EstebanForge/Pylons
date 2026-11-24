// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// SupportedParityVersions is the closed set of parity spec versions this
// CLI loads. A higher spec version errors and names this set, so an
// installed KB can never silently miss new spec semantics.
var SupportedParityVersions = []int{1}

// BuiltinProfileExtensions maps every built-in parity language id to
// its canonical file extensions. Shared truth for binding validation;
// the extraction side lives in the rules package.
var BuiltinProfileExtensions = map[string][]string{
	"go":         {".go"},
	"rust":       {".rs"},
	"php":        {".php"},
	"typescript": {".ts", ".tsx", ".js", ".jsx"},
	"javascript": {".js", ".jsx", ".mjs"},
	"python":     {".py"},
	"csharp":     {".cs"},
	"ruby":       {".rb"},
	"c":          {".c", ".h"},
	"cpp":        {".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx", ".h"},
	"java":       {".java"},
	"kotlin":     {".kt", ".kts"},
	"dart":       {".dart"},
	"sql":        {".sql"},
}

// ParityProfile is one language binding: an in-process builtin or an
// external plugin, with an options bag and optional extra extensions.
type ParityProfile struct {
	Use        string         `yaml:"use"`
	Options    map[string]any `yaml:"options,omitempty"`
	Extensions []string       `yaml:"extensions,omitempty"`
}

// ParityConfig is the closed-schema parity spec. Unlike the lenient
// rest of pylons.yaml, unknown keys here are hard errors: a typo can
// never silently disable a check.
type ParityConfig struct {
	Version  int                      `yaml:"version"`
	Profiles map[string]ParityProfile `yaml:"profiles"`
}

// parityFromMap decodes one parity spec subtree from its generic map
// form, rejecting unknown keys and validating the binding surface.
// path names the owner ("parity" or "projects.<alias>.parity") in
// errors.
func parityFromMap(path string, m map[string]any) (*ParityConfig, error) {
	spec := &ParityConfig{}
	for _, key := range slices.Sorted(maps.Keys(m)) {
		val := m[key]
		switch key {
		case "version":
			n, ok := val.(int)
			if !ok {
				return nil, fmt.Errorf("%s.version must be an integer", path)
			}
			spec.Version = n
		case "profiles":
			raw, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.profiles must be a map of language bindings", path)
			}
			profs := make(map[string]ParityProfile, len(raw))
			for _, lang := range slices.Sorted(maps.Keys(raw)) {
				pv := raw[lang]
				if !extensionNameRe.MatchString(lang) {
					return nil, fmt.Errorf("%s.profiles: invalid language id %q", path, lang)
				}
				pm, ok := pv.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%s.profiles.%s must be a map", path, lang)
				}
				prof, err := parityProfileFromMap(path, lang, pm)
				if err != nil {
					return nil, err
				}
				profs[lang] = prof
			}
			spec.Profiles = profs
		default:
			return nil, fmt.Errorf("%s: unknown key %q; the parity spec is a closed schema", path, key)
		}
	}
	if spec.Version == 0 {
		return nil, fmt.Errorf("%s.version is required", path)
	}
	if !slices.Contains(SupportedParityVersions, spec.Version) {
		return nil, fmt.Errorf("%s.version %d unsupported; supported versions: %v", path, spec.Version, SupportedParityVersions)
	}
	return spec, nil
}

func parityProfileFromMap(path, lang string, m map[string]any) (ParityProfile, error) {
	prof := ParityProfile{}
	for _, key := range slices.Sorted(maps.Keys(m)) {
		val := m[key]
		switch key {
		case "use":
			s, ok := val.(string)
			if !ok {
				return prof, fmt.Errorf("%s.profiles.%s.use must be a string", path, lang)
			}
			prof.Use = s
		case "options":
			om, ok := val.(map[string]any)
			if !ok {
				return prof, fmt.Errorf("%s.profiles.%s.options must be a map", path, lang)
			}
			prof.Options = om
		case "extensions":
			lst, ok := val.([]any)
			if !ok {
				return prof, fmt.Errorf("%s.profiles.%s.extensions must be a list", path, lang)
			}
			exts := make([]string, 0, len(lst))
			for _, e := range lst {
				s, ok := e.(string)
				if !ok || !strings.HasPrefix(s, ".") {
					return prof, fmt.Errorf("%s.profiles.%s.extensions: %v is not a dotted extension", path, lang, e)
				}
				exts = append(exts, s)
			}
			prof.Extensions = exts
		default:
			return prof, fmt.Errorf("%s.profiles.%s: unknown key %q; the parity spec is a closed schema", path, lang, key)
		}
	}
	if prof.Use == "" {
		return prof, fmt.Errorf("%s.profiles.%s.use is required", path, lang)
	}
	use, rest, found := strings.Cut(prof.Use, ":")
	if !found {
		return prof, fmt.Errorf("%s.profiles.%s.use %q must be builtin:<language> or external:<plugin>", path, lang, prof.Use)
	}
	switch use {
	case "builtin":
		if _, ok := BuiltinProfileExtensions[rest]; !ok {
			return prof, fmt.Errorf("%s.profiles.%s: unknown builtin language %q", path, lang, rest)
		}
	case "external":
		if !extensionNameRe.MatchString(rest) {
			return prof, fmt.Errorf("%s.profiles.%s: invalid external profile name %q", path, lang, rest)
		}
	default:
		return prof, fmt.Errorf("%s.profiles.%s.use %q must be builtin:<language> or external:<plugin>", path, lang, prof.Use)
	}
	return prof, nil
}

// validateParityBindings checks use-reachability and extension
// ownership for one effective binding set. An external profile must
// name an enabled extension; two profiles claiming one file extension
// is a config error. Keys are visited in sorted order so the conflict
// message is deterministic.
func validateParityBindings(path string, profiles map[string]ParityProfile, enabled func(string) bool) error {
	owner := map[string]string{}
	for _, lang := range slices.Sorted(maps.Keys(profiles)) {
		prof := profiles[lang]
		use, rest, _ := strings.Cut(prof.Use, ":")
		switch use {
		case "builtin":
			if rest != lang {
				// Allowed: a language key may bind a differently named
				// builtin (an explicit alias). No ownership rule applies.
				continue
			}
		case "external":
			if !enabled(rest) {
				return fmt.Errorf("%s.profiles.%s: external profile %q names an extension that is not enabled", path, lang, rest)
			}
		}
		exts := append([]string{}, BuiltinProfileExtensions[lang]...)
		// A profile re-listing an extension its language already owns
		// canonically stays one claim; the conflict error is for two
		// different profiles fighting over the same extension.
		for _, ext := range prof.Extensions {
			if !slices.Contains(exts, ext) {
				exts = append(exts, ext)
			}
		}
		for _, ext := range exts {
			if prev, dup := owner[ext]; dup {
				return fmt.Errorf("%s: profiles %q and %q both claim the %s extension", path, prev, lang, ext)
			}
			owner[ext] = lang
		}
	}
	return nil
}

// validateParitySpecs validates a global spec plus every project
// override. Project bindings merge over the global set for ownership
// checks, since both apply to the project.
func validateParitySpecs(c *Config) error {
	enabled := func(name string) bool { return c.HasExtension("", name) }
	if c.Parity != nil {
		if err := validateParityBindings("parity", c.Parity.Profiles, enabled); err != nil {
			return err
		}
	}
	for _, alias := range slices.Sorted(maps.Keys(c.Projects)) {
		proj := c.Projects[alias]
		if proj.Parity == nil {
			continue
		}
		merged := map[string]ParityProfile{}
		if c.Parity != nil {
			for lang, p := range c.Parity.Profiles {
				merged[lang] = p
			}
		}
		for lang, p := range proj.Parity.Profiles {
			merged[lang] = p
		}
		projEnabled := func(name string) bool { return c.HasExtension(alias, name) }
		path := "projects." + alias + ".parity"
		if err := validateParityBindings(path, merged, projEnabled); err != nil {
			return err
		}
		c.Projects[alias] = proj
	}
	return nil
}

// ParityLanguages lists the languages with an explicit binding for
// alias, in deterministic sorted order. It covers languages outside
// the builtin table, which have no canonical extensions of their own.
func (c Config) ParityLanguages(alias string) []string {
	langs := map[string]struct{}{}
	if proj, ok := c.Projects[alias]; ok && proj.Parity != nil {
		for lang := range proj.Parity.Profiles {
			langs[lang] = struct{}{}
		}
	}
	if c.Parity != nil {
		for lang := range c.Parity.Profiles {
			langs[lang] = struct{}{}
		}
	}
	out := make([]string, 0, len(langs))
	for lang := range langs {
		out = append(out, lang)
	}
	slices.Sort(out)
	return out
}

// ParityProfileFor resolves the effective parity binding for one
// language in one project. Project bindings win over global ones. An
// explicit parity section owns the binding surface, so the implicit
// builtin fallback applies only when no section exists anywhere.
func (c Config) ParityProfileFor(alias, lang string) (ParityProfile, bool) {
	if proj, ok := c.Projects[alias]; ok && proj.Parity != nil {
		if prof, ok := proj.Parity.Profiles[lang]; ok {
			return prof, true
		}
	}
	if c.Parity != nil {
		if prof, ok := c.Parity.Profiles[lang]; ok {
			return prof, true
		}
		return ParityProfile{}, false
	}
	if !c.HasExtension(alias, lang) {
		return ParityProfile{}, false
	}
	return ParityProfile{Use: "builtin:" + lang}, true
}
