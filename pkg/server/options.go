// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"net/http"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

// DefaultSchemaPath is the default location of the bngblaster configuration
// JSON schema, used to drive the "New Instance" config editor in the web UI.
const DefaultSchemaPath = "/usr/share/bngblaster/bngblaster-config.json"

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

// WithUI enables or disables serving the embedded web UI on "/". Disabled
// by default.
func WithUI(enabled bool) Option {
	return func(s *Server) {
		s.enableUI = enabled
	}
}

// WithInterfacesAPI enables or disables the "/api/v1/interfaces" endpoint
// which reports the network interfaces available on the host. Disabled by
// default.
func WithInterfacesAPI(enabled bool) Option {
	return func(s *Server) {
		s.enableInterfaces = enabled
	}
}

// WithHostAPI enables or disables the "/api/v1/host" endpoints, which
// report CPU, NUMA, memory and disk information of the host, the CPUs
// pinned by bngblaster instances and the hardware topology rendered by
// lstopo. Disabled by default.
func WithHostAPI(enabled bool) Option {
	return func(s *Server) {
		s.enableHost = enabled
	}
}

// WithGenerators enables the "/api/v1/generators" endpoints, which run the
// BNG Blaster file generators (lspgen, bgpupdate, ldpupdate) in instance
// folders. executables maps each tool to enable to its executable; tools
// left out are not offered. Disabled by default.
func WithGenerators(executables map[string]string) Option {
	return func(s *Server) {
		s.generators = executables
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

// WithAllowedHosts restricts the host names clients may use to address the
// server (the Host header), as a defense against DNS rebinding. IP literals
// and localhost are always accepted. An empty list, the default, accepts
// any host.
func WithAllowedHosts(hosts []string) Option {
	return func(s *Server) {
		s.allowedHosts = hosts
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

// WithQuietUIRequests logs the successful reads of the embedded web UI -
// its polling of instance state, logs and statistics - and of the health
// check (bngblasterctrl -health) at debug level only. Failed requests and
// actions (start, stop, upload, ...) are logged as usual. Disabled by
// default.
func WithQuietUIRequests(enabled bool) Option {
	return func(s *Server) {
		s.quietUI = enabled
	}
}

// WithAutostart reports the state of an instance autostart in the status
// of that instance (GET /api/v1/instances/<name>), also before the
// instance exists. Disabled by default.
func WithAutostart(autostart *controller.Autostart) Option {
	return func(s *Server) {
		s.autostart = autostart
		if autostart != nil {
			// Like _start and _stop, see invalidateInstanceCaches.
			autostart.OnLifecycleChange(func() { s.invalidateInstanceCaches(autostart.Instance()) })
		}
	}
}
