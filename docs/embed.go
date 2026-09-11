// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.

// Package docs embeds this directory's OpenAPI/Swagger definition and its
// Swagger UI viewer page - the same files GitHub Pages serves at
// https://rtbrick.github.io/bngblaster-controller - so a running controller
// can also serve its own API documentation directly, with no separate
// deploy step and no risk of drifting from the spec actually shipped.
package docs

import "embed"

//go:embed swagger.yaml index.html
var Assets embed.FS
