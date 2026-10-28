// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/server"
)

func TestInstanceHealth(t *testing.T) {
	code := func(c int) *int { return &c }
	autostart := func(state, err string) *controller.AutostartStatus {
		return &controller.AutostartStatus{State: state, Error: err}
	}
	tests := []struct {
		name    string
		status  instanceStatus
		wantErr string
	}{
		{
			name:    "waiting_interfaces",
			status:  instanceStatus{Status: "stopped", Autostart: autostart(controller.AutostartWaitingInterfaces, "")},
			wantErr: "waiting for interfaces",
		}, {
			name:    "starting",
			status:  instanceStatus{Status: "started", Autostart: autostart(controller.AutostartStarting, "")},
			wantErr: "instance test is starting",
		}, {
			name: "interfaces_timeout",
			status: instanceStatus{Status: "stopped", Autostart: autostart(controller.AutostartFailed,
				"timed out after 5m0s waiting for 2 interfaces, found 1")},
			wantErr: "autostart of instance test failed: timed out after 5m0s waiting for 2 interfaces, found 1",
		}, {
			name:    "start_failed",
			status:  instanceStatus{Status: "stopped", Autostart: autostart(controller.AutostartFailed, "invalid run.json: EOF")},
			wantErr: "autostart of instance test failed: invalid run.json: EOF",
		}, {
			name:   "running",
			status: instanceStatus{Status: "started", Autostart: autostart(controller.AutostartStarted, "")},
		}, {
			name:   "exited_ok",
			status: instanceStatus{Status: "stopped", ExitCode: code(0), Autostart: autostart(controller.AutostartStarted, "")},
		}, {
			name:    "exited_error",
			status:  instanceStatus{Status: "stopped", ExitCode: code(1), Autostart: autostart(controller.AutostartStarted, "")},
			wantErr: "instance test terminated with exit code 1",
		}, {
			name:    "killed",
			status:  instanceStatus{Status: "stopped", ExitCode: code(137), Autostart: autostart(controller.AutostartStarted, "")},
			wantErr: "instance test terminated with exit code 137",
		}, {
			name:    "exit_unknown",
			status:  instanceStatus{Status: "stopped", Autostart: autostart(controller.AutostartStarted, "")},
			wantErr: "instance test stopped, exit code unknown",
		}, {
			// A controller without (this) autostart: only the instance counts.
			name:   "no_autostart_running",
			status: instanceStatus{Status: "started"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := instanceHealth("test", tt.status)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, msg)
		})
	}
}

func TestCheckHealth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(server.ClientHeader) != server.HealthClientValue {
			http.Error(w, "not marked as health check", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/api/v1/instances":
			_, _ = w.Write([]byte(`["test"]`))
		case "/api/v1/instances/test":
			_, _ = w.Write([]byte(`{"status":"stopped","exit_code":0,"autostart":{"state":"started"}}`))
		case "/api/v1/instances/failing":
			_, _ = w.Write([]byte(`{"status":"stopped","autostart":{"state":"failed","error":"no config.json"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")

	_, err := checkHealth(t.Context(), addr, "")
	require.NoError(t, err)
	_, err = checkHealth(t.Context(), addr, "test")
	require.NoError(t, err)
	_, err = checkHealth(t.Context(), addr, "failing")
	require.EqualError(t, err, "autostart of instance failing failed: no config.json")
	_, err = checkHealth(t.Context(), addr, "unknown")
	require.EqualError(t, err, "GET /api/v1/instances/unknown: 404 Not Found")

	ts.Close()
	_, err = checkHealth(t.Context(), addr, "")
	require.ErrorContains(t, err, "controller not reachable")
}

func TestLocalAddr(t *testing.T) {
	for addr, want := range map[string]string{
		":8001":          "127.0.0.1:8001",
		"0.0.0.0:8001":   "127.0.0.1:8001",
		"[::]:8001":      "[::1]:8001",
		"10.0.0.1:8080":  "10.0.0.1:8080",
		"localhost:8001": "localhost:8001",
	} {
		require.Equal(t, want, localAddr(addr), addr)
	}
}
