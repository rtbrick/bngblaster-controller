// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

func TestServer_rejectsCrossOriginStateChanges(t *testing.T) {
	var stopped int
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		StopFunc:         func(_ string) { stopped++ },
	}
	handler := NewServer(repository)

	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		// A form on another site, submitted through the victim's browser.
		{"cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign origin", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"same origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusAccepted},
		{"matching origin", map[string]string{"Origin": "http://example.com"}, http.StatusAccepted},
		// curl, scripts and CI send neither header.
		{"non-browser client", nil, http.StatusAccepted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopped = 0
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/instances/test/_stop", nil)
			for k, v := range tt.headers {
				request.Header.Set(k, v)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			require.Equal(t, tt.want, recorder.Code)
			require.Equal(t, tt.want == http.StatusAccepted, stopped == 1)
		})
	}

	// Reads stay possible cross-site; the browser's same-origin policy
	// already keeps another site from seeing the response.
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/instances", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	repository.InstancesFunc = func() []string { return nil }
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestServer_hostAllowlist(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		InstancesFunc:    func() []string { return nil },
	}
	handler := NewServer(repository, WithAllowedHosts([]string{"Lab01.example.com."}))

	for host, want := range map[string]int{
		"lab01.example.com:8001": http.StatusOK,
		"LAB01.EXAMPLE.COM":      http.StatusOK,
		"10.0.0.5:8001":          http.StatusOK,
		"[2001:db8::1]:8001":     http.StatusOK,
		"localhost:8001":         http.StatusOK,
		// A rebinding attacker's own domain, pointed at the controller.
		"rebind.evil.example:8001": http.StatusForbidden,
	} {
		t.Run(host, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/instances", nil)
			request.Host = host
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			require.Equal(t, want, recorder.Code)
		})
	}

	// Without an allowlist every host is accepted, as before.
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/instances", nil)
	request.Host = "rebind.evil.example"
	recorder := httptest.NewRecorder()
	NewServer(repository).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestServer_securityHeaders(t *testing.T) {
	handler := uiServer("1.2.3")

	api := doGet(t, handler, "/api/v1/schema")
	require.Equal(t, "DENY", api.Header().Get("X-Frame-Options"))
	require.Equal(t, "nosniff", api.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "no-referrer", api.Header().Get("Referrer-Policy"))
	require.Equal(t, defaultCSP, api.Header().Get("Content-Security-Policy"),
		"files an API caller controls must never render as an active page")

	ui := doGet(t, handler, "/")
	require.Equal(t, uiCSP, ui.Header().Get("Content-Security-Policy"))
	require.Equal(t, "DENY", ui.Header().Get("X-Frame-Options"))

	require.Equal(t, apiDocsCSP, doGet(t, handler, "/docs/").Header().Get("Content-Security-Policy"))
}

func TestServer_create_rejectsOversizedConfig(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(_ string) bool { return false },
		CreateFunc:       func(_ string, _ []byte) error { return nil },
	}
	body := strings.NewReader("{\"x\":\"" + strings.Repeat("a", maxConfigSize) + "\"}")
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/instances/test", body)
	recorder := httptest.NewRecorder()
	NewServer(repository).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Empty(t, repository.CreateCalls())
}

func TestServer_start_rejectsInvalidStreamConfig(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		StartFunc: func(_ context.Context, _ string, _ controller.RunningConfig) error {
			return controller.ErrInvalidStreamConfig
		},
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/instances/test/_start",
		strings.NewReader(`{"stream_config": "/etc/shadow"}`))
	recorder := httptest.NewRecorder()
	NewServer(repository).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestServer_uploadFile_rejectsWhenDiskIsTooSmall(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		AllowUploadFunc:  func() bool { return true },
		ExistsFunc:       func(_ string) bool { return true },
	}

	request := uploadRequest(t, "test", "big.bin", "payload")
	// No file system has an exabyte free.
	request.ContentLength = 1 << 60
	recorder := httptest.NewRecorder()
	NewServer(repository).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusInsufficientStorage, recorder.Code)

	entries, err := os.ReadDir(filepath.Join(folder, "test"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestServer_uploadFile_leavesNoTemporaryFile(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		AllowUploadFunc:  func() bool { return true },
		ExistsFunc:       func(_ string) bool { return true },
	}

	recorder := httptest.NewRecorder()
	NewServer(repository).ServeHTTP(recorder, uploadRequest(t, "test", "streams.json", "{}"))
	require.Equal(t, http.StatusOK, recorder.Code)

	entries, err := os.ReadDir(filepath.Join(folder, "test"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "streams.json", entries[0].Name())
}
