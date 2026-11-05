// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"net/http"

	"github.com/rtbrick/bngblaster-controller/docs"
)

// registerAPIDocsRoutes exposes the embedded OpenAPI/Swagger definition and
// a Swagger UI viewer for it at /docs/. It is independent of the web UI
// (registered regardless of WithUI) since it documents the REST API itself.
func (s *Server) registerAPIDocsRoutes() {
	s.router.Path("/docs").Methods(http.MethodGet).Handler(http.RedirectHandler("/docs/", http.StatusMovedPermanently))
	s.router.Path("/docs/").Methods(http.MethodGet).Handler(withCSP(apiDocsCSP, s.apiDocsAsset("index.html", "text/html; charset=utf-8")))
	s.router.Path("/docs/swagger.yaml").Methods(http.MethodGet).Handler(s.apiDocsAsset("swagger.yaml", "application/yaml"))
	// The vendored Swagger UI and its bootstrap: served from the binary, so
	// the page loads no third-party script (see apiDocsCSP).
	s.router.Path("/docs/swagger-initializer.js").Methods(http.MethodGet).
		Handler(s.apiDocsAsset("swagger-initializer.js", "text/javascript; charset=utf-8"))
	s.router.Path("/docs/swagger-ui/swagger-ui-bundle.js").Methods(http.MethodGet).
		Handler(s.apiDocsAsset("swagger-ui/swagger-ui-bundle.js", "text/javascript; charset=utf-8"))
	s.router.Path("/docs/swagger-ui/swagger-ui.css").Methods(http.MethodGet).
		Handler(s.apiDocsAsset("swagger-ui/swagger-ui.css", "text/css; charset=utf-8"))
}

func (s *Server) apiDocsAsset(name, ct string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		content, err := docs.Assets.ReadFile(name)
		if err != nil {
			JSONError(w, "api docs not available", http.StatusInternalServerError)
			return
		}
		w.Header().Set(contentType, ct)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

// withCSP replaces the default Content-Security-Policy for a handler that
// serves an actual page.
func withCSP(policy string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}
