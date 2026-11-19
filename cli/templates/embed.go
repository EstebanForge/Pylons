// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package templates

import "embed"

// FS holds all compiled-in protocol markdown templates.
//
//go:embed *.md
var FS embed.FS
