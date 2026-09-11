// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

// webUIAssets embeds the built-in single-page application that ships inside
// the bngblaster-controller binary. It is intentionally dependency-free
// (vanilla HTML/CSS/JS) so the controller remains a single static binary
// with no build step or external assets required at install time.
//
//go:embed webui/index.html webui/static
var webUIAssets embed.FS

// processStartToken distinguishes one controller process from another when
// no meaningful release version is available (a "dev" build). It gives the
// asset version something that still changes across restarts, so a developer
// rebuilding the UI is never served a stale asset from the browser cache.
var processStartToken = fmt.Sprintf("dev-%d", time.Now().UnixNano())

// registerUIRoutes wires the embedded web UI into the router. It is only
// called when the UI is enabled (see WithUI). Static assets are served from
// "/static/...", the application shell from "/".
func (s *Server) registerUIRoutes() {
	staticFS, err := fs.Sub(webUIAssets, "webui/static")
	if err != nil {
		// Cannot happen: the sub-directory is embedded at compile time.
		panic(err)
	}

	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
	s.router.PathPrefix("/static/").Methods(http.MethodGet).Handler(s.cacheControl(fileServer))
	s.router.Path("/").Methods(http.MethodGet).Handler(s.index())
	s.router.Path("/favicon.ico").Methods(http.MethodGet).Handler(s.favicon())
}

// uiAssetVersion is the cache busting token appended to every asset URL the
// application shell emits. It is derived from the controller version, which
// is assigned after NewServer returns, so it is resolved lazily on first use
// and then kept for the lifetime of the process.
func (s *Server) uiAssetVersion() string {
	s.assetVersionOnce.Do(func() {
		version := strings.TrimSpace(s.Version)
		if version == "" || version == "dev" {
			s.assetVersion = processStartToken
			return
		}
		s.assetVersion = version
	})
	return s.assetVersion
}

// indexTemplate renders the application shell. The only substitution is
// AssetVersion, used to version every asset URL.
var indexTemplate = sync.OnceValues(func() (*template.Template, error) {
	content, err := webUIAssets.ReadFile("webui/index.html")
	if err != nil {
		return nil, err
	}
	return template.New("index").Parse(string(content))
})

func (s *Server) index() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := indexTemplate()
		if err != nil {
			JSONError(w, "ui not available", http.StatusInternalServerError)
			return
		}
		var rendered bytes.Buffer
		if err := tmpl.Execute(&rendered, struct{ AssetVersion string }{s.uiAssetVersion()}); err != nil {
			JSONError(w, "ui not available", http.StatusInternalServerError)
			return
		}
		// The shell itself carries the asset version, so it must never be
		// cached: a stale shell would keep pointing at the previous release's
		// asset URLs and defeat the versioning entirely.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(contentType, "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rendered.Bytes())
	}
}

func (s *Server) favicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := webUIAssets.ReadFile("webui/static/img/logo.png")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set(contentType, "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

// cacheControl sets the caching policy for the embedded UI assets. Assets are
// compiled into the binary and have no file system timestamp, so the
// FileServer can offer neither Last-Modified nor a useful ETag; the freshness
// signal has to come from the URL instead.
//
// A request carrying the current asset version is immutable by construction -
// a new controller release produces a new version and therefore new URLs - so
// it may be cached indefinitely. Anything else (a bookmarked or hand-typed
// asset URL, or one left over from a previous release) must be revalidated,
// otherwise a browser could keep running a stale app.js against a newer API.
func (s *Server) cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == s.uiAssetVersion() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
