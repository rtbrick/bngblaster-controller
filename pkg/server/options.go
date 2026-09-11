// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import "net/http"

// DefaultSchemaPath is the default location of the bngblaster configuration
// JSON schema, used to drive the "New Instance" config editor in the web UI.
const DefaultSchemaPath = "/etc/bngblaster/bngblaster-config.json"

// AuthMiddleware is the function signature used to plug in authentication.
// It wraps a http.Handler and is invoked for every request routed through
// the server, before the UI and API handlers.
type AuthMiddleware func(http.Handler) http.Handler

// noopAuthMiddleware is the default AuthMiddleware. It performs no
// authentication and simply forwards the request. Replace it with
// WithAuthMiddleware once a login/authentication mechanism is required.
func noopAuthMiddleware(next http.Handler) http.Handler {
	return next
}

// Option configures optional behavior of the Server.
type Option func(*Server)

// WithUI enables or disables serving the embedded web UI on "/".
// Enabled by default.
func WithUI(enabled bool) Option {
	return func(s *Server) {
		s.enableUI = enabled
	}
}

// WithInterfacesAPI enables or disables the "/api/v1/interfaces" endpoint
// which reports the network interfaces available on the host. Enabled by
// default.
func WithInterfacesAPI(enabled bool) Option {
	return func(s *Server) {
		s.enableInterfaces = enabled
	}
}

// WithSchemaPath sets the file system location of the bngblaster
// configuration JSON schema served via "/api/v1/schema". Defaults to
// DefaultSchemaPath.
func WithSchemaPath(path string) Option {
	return func(s *Server) {
		s.schemaPath = path
	}
}

// WithAuthMiddleware installs the given middleware in front of every route
// (UI and API alike). This is the extension point intended for adding
// login/session/token based authentication later without restructuring the
// routing table.
func WithAuthMiddleware(mw AuthMiddleware) Option {
	return func(s *Server) {
		if mw != nil {
			s.authMiddleware = mw
		}
	}
}
