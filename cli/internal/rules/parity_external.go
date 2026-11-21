// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/estebanforge/pylons/cli/internal/config"
	"github.com/estebanforge/pylons/cli/internal/resolve"
)

// parityExtRequest is one parity ops request line for an external
// profile plugin. The engine reads the source itself (bounded,
// regular-file-only) and sends the text, so the plugin stays a pure
// function and works against any checked-out tree.
type parityExtRequest struct {
	SchemaVersion int            `json:"schema_version"`
	Op            string         `json:"op"`
	Profile       string         `json:"profile"`
	Name          string         `json:"name"`
	Options       map[string]any `json:"options,omitempty"`
	Source        string         `json:"source"`
}

// parityExtCase mirrors parityCase on the wire. Valued is encoded by
// the presence of a value; an empty-string value uses "value": "".
type parityExtCase struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

type parityExtResponse struct {
	Cases []parityExtCase `json:"cases"`
	Error string          `json:"error,omitempty"`
}

// extractExternalStates asks the project's external profile plugin for
// the enum case list. Transport failures (disabled, missing binary,
// timeout, invalid JSON) are not-evaluated with a cause; a plugin that
// runs but cannot resolve the shape reports its error, which the set
// checks grade as not-evaluated too.
func extractExternalStates(cfg config.Config, alias, lang, enumName, source string, prof config.ParityProfile, kbRoot string, allowExternal bool) ([]parityCase, error) {
	_, name, _ := strings.Cut(prof.Use, ":")
	reg := resolve.NewRegistryWithOptions(resolve.RegistryOptions{
		Extensions:    cfg.ProjectExtensions(alias),
		ExtDir:        filepath.Join(kbRoot, "extensions"),
		AllowExternal: allowExternal,
	})
	req := parityExtRequest{
		SchemaVersion: 1,
		Op:            "states",
		Profile:       lang,
		Name:          enumName,
		Options:       prof.Options,
		Source:        source,
	}
	var resp parityExtResponse
	if err := reg.CallExternal(name, req, &resp); err != nil {
		return nil, fmt.Errorf("external parity profile %s failed: %s", name, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("external parity profile %s cannot resolve %s: %s", name, enumName, resp.Error)
	}
	cases := make([]parityCase, 0, len(resp.Cases))
	for _, c := range resp.Cases {
		if c.Name == "" {
			return nil, fmt.Errorf("external parity profile %s returned a case without a name", name)
		}
		cases = append(cases, parityCase{name: c.Name, value: c.Value, valued: c.Value != ""})
	}
	return cases, nil
}
