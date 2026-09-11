// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

func uiServer(version string) *Server {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	server := NewServer(repository)
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
	require.Contains(t, body, "/static/js/app.js?v=1.2.3")
	require.Contains(t, body, "/static/css/app.css?v=1.2.3")
}

func TestServer_index_devBuildsGetAPerProcessVersion(t *testing.T) {
	// A "dev" build has no release version to key the cache off, so assets
	// must still be re-fetched after a rebuild and restart.
	body := doGet(t, uiServer("dev"), "/").Body.String()
	require.Contains(t, body, "/static/js/app.js?v=dev-")
}

func TestServer_staticAssets_cachePolicyFollowsTheVersion(t *testing.T) {
	handler := uiServer("1.2.3")

	// The versioned URL the shell emits is immutable by construction.
	versioned := doGet(t, handler, "/static/js/app.js?v=1.2.3")
	require.Equal(t, http.StatusOK, versioned.Code)
	require.Equal(t, "public, max-age=31536000, immutable", versioned.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", versioned.Header().Get("X-Content-Type-Options"))

	// Anything else - a bookmark, or a URL left over from an older release -
	// must be revalidated so a stale app.js never runs against a newer API.
	for _, target := range []string{"/static/js/app.js", "/static/js/app.js?v=0.9.0"} {
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
