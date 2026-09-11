// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
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
	s.router.Path("/docs/").Methods(http.MethodGet).Handler(s.apiDocsAsset("index.html", "text/html; charset=utf-8"))
	s.router.Path("/docs/swagger.yaml").Methods(http.MethodGet).Handler(s.apiDocsAsset("swagger.yaml", "application/yaml"))
}

func (s *Server) apiDocsAsset(name, ct string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
