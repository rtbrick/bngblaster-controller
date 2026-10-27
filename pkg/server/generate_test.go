// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/generator"
)

func doGenerate(t *testing.T, handler http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestServer_generate(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       string
		exists     bool
		result     generator.Result
		err        error
		wantStatus int
		wantCalled bool
	}{
		{
			name: "success", target: "/api/v1/instances/test/_generate/bgpupdate", body: `{"next-hop-base": "10.0.0.1"}`,
			exists: true, result: generator.Result{Files: []string{"out.bgp"}}, wantStatus: http.StatusOK, wantCalled: true,
		},
		{
			name: "tool not enabled", target: "/api/v1/instances/test/_generate/ldpupdate", body: `{"lsr-id": "1.1.1.1"}`,
			exists: true, wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown tool", target: "/api/v1/instances/test/_generate/bngblaster", body: `{}`,
			exists: true, wantStatus: http.StatusNotFound,
		},
		{
			name: "instance missing", target: "/api/v1/instances/test/_generate/bgpupdate", body: `{"next-hop-base": "10.0.0.1"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "invalid request", target: "/api/v1/instances/test/_generate/bgpupdate", body: `{"control-socket": "run.sock"}`,
			exists: true, wantStatus: http.StatusBadRequest,
		},
		{
			name: "busy", target: "/api/v1/instances/test/_generate/lspgen", body: `{}`,
			exists: true, err: controller.ErrGeneratorBusy, wantStatus: http.StatusConflict, wantCalled: true,
		},
		{
			name: "output exists", target: "/api/v1/instances/test/_generate/lspgen", body: `{"mrt-file": "isis.mrt"}`,
			exists: true, err: fmt.Errorf("%w: isis.mrt", generator.ErrOutputExists), wantStatus: http.StatusConflict, wantCalled: true,
		},
		{
			name: "input missing", target: "/api/v1/instances/test/_generate/lspgen", body: `{"read-config-file": "t.json"}`,
			exists: true, err: generator.ErrInputMissing, wantStatus: http.StatusBadRequest, wantCalled: true,
		},
		{
			name: "unavailable", target: "/api/v1/instances/test/_generate/lspgen", body: `{}`,
			exists: true, err: generator.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCalled: true,
		},
		{
			name: "failed", target: "/api/v1/instances/test/_generate/lspgen", body: `{}`,
			exists: true, result: generator.Result{ExitCode: 1, Stderr: "bad level"}, err: generator.ErrFailed,
			wantStatus: http.StatusUnprocessableEntity, wantCalled: true,
		},
		{
			name: "timeout", target: "/api/v1/instances/test/_generate/lspgen", body: `{}`,
			exists: true, err: generator.ErrTimeout, wantStatus: http.StatusGatewayTimeout, wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string { return configFolder },
				ExistsFunc:       func(_ string) bool { return tt.exists },
				GenerateFunc: func(_ context.Context, name, executable string, job generator.Job) (generator.Result, error) {
					called = true
					require.Equal(t, "test", name)
					require.Equal(t, "/opt/"+job.Tool, executable)
					return tt.result, tt.err
				},
			}
			handler := NewServer(repository, WithGenerators(map[string]string{
				generator.LSPGen:    "/opt/lspgen",
				generator.BGPUpdate: "/opt/bgpupdate",
			}))

			recorder := doGenerate(t, handler, tt.target, tt.body)
			require.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
			require.Equal(t, tt.wantCalled, called)
			if !tt.wantCalled {
				return
			}
			// The tool's output is part of every response, failed or not.
			var response generateResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, tt.result, response.Result)
			if tt.err != nil {
				require.Equal(t, tt.err.Error(), response.Message)
			}
		})
	}
}

func TestServer_generate_disabled(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(_ string) bool { return true },
	}
	handler := NewServer(repository)

	require.Equal(t, http.StatusNotFound, doGet(t, handler, "/api/v1/generators").Code)
	recorder := doGenerate(t, handler, "/api/v1/instances/test/_generate/lspgen", `{}`)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestServer_generatorList(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "lspgen")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\necho 'Version: 0.9.1'\n"), 0o700)) //nolint:gosec // the fake tool must be executable
	repository := &controller.RepositoryMock{ConfigFolderFunc: func() string { return configFolder }}
	handler := NewServer(repository, WithGenerators(map[string]string{
		generator.LSPGen:    tool,
		generator.LDPUpdate: filepath.Join(t.TempDir(), "missing"),
	}))

	recorder := doGet(t, handler, "/api/v1/generators")
	require.Equal(t, http.StatusOK, recorder.Code)
	var infos []generator.Info
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &infos))
	require.Len(t, infos, 2, "only the enabled tools, in a fixed order")
	require.Equal(t, generator.LSPGen, infos[0].Name)
	require.True(t, infos[0].Available)
	require.Equal(t, "0.9.1", infos[0].Version)
	require.NotEmpty(t, infos[0].Options)
	require.Equal(t, generator.LDPUpdate, infos[1].Name)
	require.False(t, infos[1].Available)
	require.NotEmpty(t, infos[1].Error)
}

func TestServer_fileDownload_hidesGeneratorTempFiles(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "test", ".gen-isis.mrt"), []byte("partial"), 0o600))
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		ExistsFunc:       func(_ string) bool { return true },
	}
	handler := NewServer(repository)

	require.Equal(t, http.StatusNotFound, doGet(t, handler, "/api/v1/instances/test/_files/.gen-isis.mrt").Code)
}
