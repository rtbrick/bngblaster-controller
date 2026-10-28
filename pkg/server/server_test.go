// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

const (
	configFolder = "configs"
)

func TestServer_create(t *testing.T) {
	tests := []struct {
		name         string
		resultCreate error
		resultExists bool
		body         string
		wantBody     string
		want         int
	}{
		{
			name:         "not_exists",
			resultExists: false,
			resultCreate: nil,
			wantBody:     "body not readable",
			want:         http.StatusBadRequest,
		}, {
			name:         "exists",
			resultExists: true,
			resultCreate: nil,
			body:         "{}",
			want:         http.StatusNoContent,
		}, {
			name:         "running",
			resultExists: true,
			resultCreate: controller.ErrBlasterRunning,
			body:         "{}",
			wantBody:     "instance is running\n",
			want:         http.StatusPreconditionFailed,
		}, {
			name:         "error",
			resultExists: true,
			resultCreate: fmt.Errorf("other error"),
			body:         "{}",
			wantBody:     "not able to create instance\n",
			want:         http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				CreateFunc: func(name string, config []byte) error {
					return tt.resultCreate
				},
				ExistsFunc: func(name string) bool {
					return tt.resultExists
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.PUT("/api/v1/instances/{instance_name}", tt.name)
			request.WithText(tt.body)

			response := request.Expect().
				Status(tt.want)
			if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.Text().Contains(tt.wantBody)
			}
			for _, call := range repository.ExistsCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_status(t *testing.T) {
	exitCode := func(code int) *int { return &code }
	tests := []struct {
		name          string
		resultRunning bool
		resultExists  bool
		// resultExitCode is the exit code of the last run, nil if unknown.
		resultExitCode *int
		wantBody       string
		want           int
		// wantExitCode is the expected exit_code, nil for none.
		wantExitCode any
	}{
		{
			name:          "not_exists",
			resultExists:  false,
			resultRunning: false,
			wantBody:      "404 page not found",
			want:          http.StatusNotFound,
		}, {
			name:          "exists",
			resultExists:  true,
			resultRunning: false,
			wantBody:      "stopped",
			want:          http.StatusOK,
		}, {
			name:          "running",
			resultExists:  true,
			resultRunning: true,
			wantBody:      "started",
			want:          http.StatusOK,
		}, {
			name:           "exited",
			resultExists:   true,
			resultExitCode: exitCode(0),
			wantBody:       "stopped",
			want:           http.StatusOK,
			wantExitCode:   0.0,
		}, {
			name:           "failed",
			resultExists:   true,
			resultExitCode: exitCode(137),
			wantBody:       "stopped",
			want:           http.StatusOK,
			wantExitCode:   137.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				RunningFunc: func(string) bool {
					return tt.resultRunning
				},
				ExistsFunc: func(string) bool {
					return tt.resultExists
				},
				ExitCodeFunc: func(string) (int, bool) {
					if tt.resultExitCode == nil {
						return 0, false
					}
					return *tt.resultExitCode, true
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.GET("/api/v1/instances/{instance_name}", tt.name)

			response := request.Expect().
				Status(tt.want)

			if tt.wantBody == "" {
				response.NoContent()
			} else if response.Raw().StatusCode == http.StatusOK {
				object := response.JSON().Object()
				object.ContainsKey("status").ValueEqual("status", tt.wantBody)
				object.NotContainsKey("autostart")
				if tt.wantExitCode == nil {
					object.NotContainsKey("exit_code")
				} else {
					object.ValueEqual("exit_code", tt.wantExitCode)
				}
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.wantBody)
			}
			for _, call := range repository.ExistsCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_status_autostart(t *testing.T) {
	tests := []struct {
		name string
		// run runs the autostart, which fails for lack of a config.json;
		// otherwise it is still waiting for interfaces.
		run       bool
		exists    bool
		running   bool
		wantState string
		wantError bool
		wantBody  string
	}{
		{
			// Before the instance exists, its autostart state replaces
			// the 404.
			name: "waiting", wantState: controller.AutostartWaitingInterfaces, wantBody: "stopped",
		}, {
			name: "failed", run: true, wantState: controller.AutostartFailed, wantError: true, wantBody: "stopped",
		}, {
			name: "running", exists: true, running: true, wantState: controller.AutostartWaitingInterfaces, wantBody: "started",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := t.TempDir()
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string { return folder },
				RunningFunc:      func(string) bool { return tt.running },
				ExistsFunc:       func(name string) bool { return name == "auto" && tt.exists },
				ExitCodeFunc:     func(string) (int, bool) { return 0, false },
			}
			autostart := controller.NewAutostart(repository, "auto")
			if tt.run {
				autostart.Run(t.Context())
			}
			server := httptest.NewServer(NewServer(repository, WithAutostart(autostart)))
			defer server.Close()
			e := httpexpect.New(t, server.URL)

			object := e.GET("/api/v1/instances/auto").Expect().Status(http.StatusOK).JSON().Object()
			object.ValueEqual("status", tt.wantBody)
			state := object.Value("autostart").Object()
			state.ValueEqual("state", tt.wantState)
			if tt.wantError {
				state.Value("error").String().Contains("config.json")
			} else {
				state.NotContainsKey("error")
			}

			// Other instances are not affected.
			e.GET("/api/v1/instances/other").Expect().Status(http.StatusNotFound)
		})
	}
}

func TestServer_delete(t *testing.T) {
	tests := []struct {
		name         string
		resultDelete error
		body         string
		want         int
	}{
		{
			name:         "not_exists",
			resultDelete: nil,
			want:         http.StatusNoContent,
		}, {
			name:         "exists",
			resultDelete: nil,
			want:         http.StatusNoContent,
		}, {
			name:         "running",
			resultDelete: controller.ErrBlasterRunning,
			body:         "instance is running",
			want:         http.StatusPreconditionFailed,
		}, {
			name:         "error",
			resultDelete: fmt.Errorf("other error"),
			body:         "not able to delete instance",
			want:         http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				DeleteFunc: func(name string) error {
					return tt.resultDelete
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			response := e.DELETE("/api/v1/instances/{instance_name}", tt.name).
				Expect().
				Status(tt.want)
			if tt.body == "" {
				response.NoContent()
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.body)
			}
			require.Equal(t, len(repository.DeleteCalls()), 1)
			for _, call := range repository.DeleteCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_start(t *testing.T) {
	tests := []struct {
		name        string
		resultStart error
		body        *controller.RunningConfig
		wantBody    string
		want        int
	}{
		{
			name:        "bad_body",
			resultStart: nil,
			want:        http.StatusBadRequest,
		}, {
			name:        "not_exists",
			resultStart: controller.ErrBlasterNotExists,
			body:        &controller.RunningConfig{},
			wantBody:    "404 page not found",
			want:        http.StatusNotFound,
		}, {
			name:        "exists",
			resultStart: nil,
			body:        &controller.RunningConfig{},
			want:        http.StatusNoContent,
		}, {
			name:        "running",
			resultStart: controller.ErrBlasterRunning,
			body:        &controller.RunningConfig{},
			wantBody:    "instance is running",
			want:        http.StatusPreconditionFailed,
		}, {
			name:        "aborted",
			resultStart: controller.ErrBlasterStartupAborted,
			body:        &controller.RunningConfig{},
			wantBody:    "blaster instance startup aborted",
			want:        http.StatusConflict,
		}, {
			name:        "error",
			resultStart: fmt.Errorf("other error"),
			body:        &controller.RunningConfig{},
			wantBody:    "other error",
			want:        http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				StartFunc: func(_ context.Context, _ string, _ controller.RunningConfig) error {
					return tt.resultStart
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_start", tt.name)
			if tt.body != nil {
				request.WithJSON(tt.body)
			}
			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.JSON().Object().ContainsKey("message").ValueEqual("message", tt.wantBody)
			}
			for _, call := range repository.StartCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_stop(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "not_exists",
			want: http.StatusAccepted,
		}, {
			name: "exists",
			want: http.StatusAccepted,
		}, {
			name: "running",
			want: http.StatusAccepted,
		}, {
			name: "error",
			want: http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				StopFunc: func(name string) {
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_stop", tt.name)

			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			response.NoContent()
			for _, call := range repository.StopCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_kill(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{
			name: "not_exists",
			want: http.StatusAccepted,
		}, {
			name: "exists",
			want: http.StatusAccepted,
		}, {
			name: "running",
			want: http.StatusAccepted,
		}, {
			name: "error",
			want: http.StatusAccepted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				KillFunc: func(name string) {
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_kill", tt.name)

			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			response.NoContent()
			for _, call := range repository.KillCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_command(t *testing.T) {
	tests := []struct {
		name        string
		resultStart []byte
		resultError error
		body        *controller.SocketCommand
		wantBody    any
		wantCode    int
		want        int
	}{
		{
			name:        "bad_body",
			resultStart: nil,
			want:        http.StatusBadRequest,
		}, {
			name:        "not_exists",
			resultError: controller.ErrBlasterNotExists,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "404 page not found"},
			want:        http.StatusNotFound,
		}, {
			name:        "not running",
			resultError: controller.ErrBlasterNotRunning,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "instance is not running"},
			want:        http.StatusPreconditionFailed,
		}, {
			name:        "null returned or not json",
			resultStart: nil,
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "EOF"},
			want:        http.StatusInternalServerError,
		}, {
			name:        "exists ok",
			resultStart: []byte(`{"code": 200}`),
			body:        &controller.SocketCommand{},
			wantBody:    " ",
			wantCode:    200,
			want:        http.StatusOK,
		}, {
			name:        "exists not found",
			resultStart: []byte(`{"code": 404}`),
			body:        &controller.SocketCommand{},
			wantBody:    " ",
			wantCode:    404,
			want:        http.StatusNotFound,
		}, {
			name:        "error",
			resultError: fmt.Errorf("other error"),
			body:        &controller.SocketCommand{},
			wantBody:    &message{Message: "not able to send command"},
			want:        http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &controller.RepositoryMock{
				ConfigFolderFunc: func() string {
					return configFolder
				},
				CommandFunc: func(name string, config controller.SocketCommand) ([]byte, error) {
					return tt.resultStart, tt.resultError
				},
			}

			handler := NewServer(repository)
			server := httptest.NewServer(handler)
			defer server.Close()
			e := httpexpect.New(t, server.URL)
			request := e.POST("/api/v1/instances/{instance_name}/_command", tt.name)
			if tt.body != nil {
				request.WithJSON(tt.body)
			}
			response := request.Expect().Status(tt.want)
			if tt.want == http.StatusBadRequest {
				return
			}
			if tt.wantCode > 0 {
				response.JSON().Object().ContainsKey("code").Value("code").Equal(tt.wantCode)
			} else if tt.wantBody == "" {
				response.NoContent()
			} else {
				response.JSON().Equal(tt.wantBody)
			}
			for _, call := range repository.StartCalls() {
				require.Equal(t, call.Name, tt.name)
			}
		})
	}
}

func TestServer_fileServing_noCache(t *testing.T) {
	// A saved config must not be served from the browser cache afterwards,
	// otherwise the web UI editor reopens the previous version.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "test"), 0o755))
	file := filepath.Join(dir, "test", controller.ConfigFilename)
	require.NoError(t, os.WriteFile(file, []byte(`{"old":true}`), 0o600))
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return dir },
	}
	server := httptest.NewServer(NewServer(repository))
	defer server.Close()
	e := httpexpect.New(t, server.URL)

	e.GET("/api/v1/instances/test/config.json").Expect().
		Status(http.StatusOK).
		Header("Cache-Control").Equal("no-cache")
	require.NoError(t, os.WriteFile(file, []byte(`{"new":true}`), 0o600))
	e.GET("/api/v1/instances/test/config.json").Expect().
		Status(http.StatusOK).
		Body().Equal(`{"new":true}`)
}
