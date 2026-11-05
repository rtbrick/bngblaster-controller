// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

func uiServer(version string) *Server {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	server := NewServer(repository, WithUI(true))
	server.Version = version
	return server
}

func TestServer_index_versionsEveryAssetURL(t *testing.T) {
	handler := uiServer("1.2.3")

	recorder := doGet(t, handler, "/")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/html; charset=utf-8", recorder.Header().Get("Content-Type"))
	// The shell carries the asset version, so caching it would pin the
	// browser to the previous release's asset URLs.
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))

	body := recorder.Body.String()
	require.NotContains(t, body, "{{", "the template must be fully rendered")
	require.Contains(t, body, "/static/js/app.js?v=1.2.3-"+webUIContentHash())
	require.Contains(t, body, "/static/css/app.css?v=1.2.3-"+webUIContentHash())
}

func TestServer_index_assetVersionFollowsTheContent(t *testing.T) {
	// Every build between two release tags carries the same version, so
	// the version alone would let a browser keep the cached assets of an
	// older build: the content hash has to tell them apart.
	hash := webUIContentHash()
	require.Len(t, hash, 12)
	require.Equal(t, hash, webUIContentHash(), "the hash must be stable")
	body := doGet(t, uiServer("dev"), "/").Body.String()
	require.Contains(t, body, "/static/js/app.js?v=dev-"+hash)
}

func TestServer_staticAssets_cachePolicyFollowsTheVersion(t *testing.T) {
	handler := uiServer("1.2.3")

	// The versioned URL the shell emits is immutable by construction.
	versioned := doGet(t, handler, "/static/js/app.js?v=1.2.3-"+webUIContentHash())
	require.Equal(t, http.StatusOK, versioned.Code)
	require.Equal(t, "public, max-age=31536000, immutable", versioned.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", versioned.Header().Get("X-Content-Type-Options"))

	// Anything else - a bookmark, or a URL left over from an older release -
	// must be revalidated so a stale app.js never runs against a newer API.
	// The same version with other assets is a different build.
	for _, target := range []string{"/static/js/app.js", "/static/js/app.js?v=0.9.0", "/static/js/app.js?v=1.2.3"} {
		stale := doGet(t, handler, target)
		require.Equal(t, http.StatusOK, stale.Code)
		require.Equal(t, "no-cache", stale.Header().Get("Cache-Control"), target)
	}
}

func TestServer_uiCanBeDisabled(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	handler := NewServer(repository, WithUI(false))

	require.Equal(t, http.StatusNotFound, doGet(t, handler, "/").Code)
	require.Equal(t, http.StatusNotFound, doGet(t, handler, "/static/js/app.js").Code)
}

func TestServer_apiDocsAreServedIndependentlyOfTheUI(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	handler := NewServer(repository, WithUI(false))

	docs := doGet(t, handler, "/docs/swagger.yaml")
	require.Equal(t, http.StatusOK, docs.Code)
	require.Equal(t, "application/yaml", docs.Header().Get("Content-Type"))
	require.True(t, strings.HasPrefix(docs.Body.String(), "openapi:"),
		"expected the embedded OpenAPI document")

	require.Equal(t, http.StatusOK, doGet(t, handler, "/docs/").Code)
}

// The API docs page must load nothing from third parties and run no
// inline script (apiDocsCSP forbids both), and every asset it references
// must be served with a type the browser accepts under nosniff.
func TestServer_apiDocsPageIsSelfContained(t *testing.T) {
	handler := NewServer(&controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	})
	page := doGet(t, handler, "/docs/").Body.String()
	require.NotContains(t, page, "://", "the docs page references an external URL")
	require.NotRegexp(t, `<script>|<script [^>]*>[^<]`, page, "the docs page has an inline script")

	assets := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(page, -1)
	require.Len(t, assets, 3)
	for _, asset := range assets {
		response := doGet(t, handler, "/docs/"+asset[1])
		require.Equal(t, http.StatusOK, response.Code, asset[1])
		want := "text/javascript; charset=utf-8"
		if strings.HasSuffix(asset[1], ".css") {
			want = "text/css; charset=utf-8"
		}
		require.Equal(t, want, response.Header().Get("Content-Type"), asset[1])
		require.NotEmpty(t, response.Body.Len(), asset[1])
	}
}
