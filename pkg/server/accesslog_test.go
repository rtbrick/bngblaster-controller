// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

// captureLog redirects the global logger into a buffer at debug level for
// the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger, level := log.Logger, zerolog.GlobalLevel()
	log.Logger = zerolog.New(&buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		log.Logger = logger
		zerolog.SetGlobalLevel(level)
	})
	return &buf
}

// logLines decodes the JSON log lines written into buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		lines = append(lines, m)
	}
	return lines
}

func TestAccessLog(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return name == "x" },
		RunningFunc:      func(string) bool { return true },
		StopFunc:         func(string) {},
		CommandFunc: func(string, controller.SocketCommand) ([]byte, error) {
			return []byte(`{"code": 200}`), nil
		},
	}
	tests := []struct {
		name    string
		quietUI bool
		ui      bool
		method  string
		url     string
		body    string
		// wantLevel and wantMsg describe the single expected log line,
		// want the fields it must carry.
		wantLevel string
		wantMsg   string
		want      map[string]any
	}{
		{
			name: "ui read", ui: true, method: http.MethodGet, url: "/api/v1/instances/x",
			wantLevel: "info", wantMsg: "GET /api/v1/instances/x 200",
			want: map[string]any{"instance": "x", "client": "webui"},
		}, {
			name: "quiet ui read", quietUI: true, ui: true, method: http.MethodGet, url: "/api/v1/instances/x",
			wantLevel: "debug", wantMsg: "GET /api/v1/instances/x 200",
		}, {
			name: "quiet non-ui read", quietUI: true, method: http.MethodGet, url: "/api/v1/instances/x",
			wantLevel: "info", wantMsg: "GET /api/v1/instances/x 200",
		}, {
			name: "quiet ui failed read", quietUI: true, ui: true, method: http.MethodGet, url: "/api/v1/instances/y",
			wantLevel: "warn", wantMsg: "GET /api/v1/instances/y 404",
			want: map[string]any{"instance": "y", "error": "404 page not found"},
		}, {
			name: "quiet ui action", quietUI: true, ui: true, method: http.MethodPost, url: "/api/v1/instances/x/_stop",
			wantLevel: "info", wantMsg: "POST /api/v1/instances/x/_stop 202",
			want: map[string]any{"instance": "x", "action": "stop requested"},
		}, {
			name: "quiet ui read-only command", quietUI: true, ui: true, method: http.MethodPost,
			url: "/api/v1/instances/x/_command", body: `{"command":"session-counters"}`,
			wantLevel: "debug", wantMsg: "POST /api/v1/instances/x/_command 200",
			want: map[string]any{"command": "session-counters"},
		}, {
			name: "quiet ui changing command", quietUI: true, ui: true, method: http.MethodPost,
			url: "/api/v1/instances/x/_command", body: `{"command":"session-restart"}`,
			wantLevel: "info", wantMsg: "POST /api/v1/instances/x/_command 200",
			want: map[string]any{"command": "session-restart", "action": "command"},
		}, {
			name: "plain text error", method: http.MethodPut, url: "/api/v1/instances/x",
			wantLevel: "warn", wantMsg: "PUT /api/v1/instances/x 400",
			want: map[string]any{"error": "body not readable"},
		}, {
			name: "unmatched route", method: http.MethodGet, url: "/api/v1/nothing",
			wantLevel: "warn", wantMsg: "GET /api/v1/nothing 404",
		}, {
			name: "cross-origin rejected", method: http.MethodPost, url: "/api/v1/instances/x/_stop",
			wantLevel: "warn", wantMsg: "POST /api/v1/instances/x/_stop 403",
			want: map[string]any{"error": "cross-origin request rejected", "sec_fetch_site": "cross-site"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLog(t)
			handler := NewServer(repository, WithQuietUIRequests(tt.quietUI))
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.url, strings.NewReader(tt.body))
			if tt.ui {
				req.Header.Set(uiClientHeader, uiClientValue)
			}
			if tt.name == "cross-origin rejected" {
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			lines := logLines(t, buf)
			require.Len(t, lines, 1, buf.String())
			line := lines[0]
			require.Equal(t, tt.wantLevel, line["level"])
			// The duration at the end of the message varies.
			msg, _ := line["message"].(string)
			require.True(t, strings.HasPrefix(msg, tt.wantMsg+" "), msg)
			require.Equal(t, "192.0.2.1", line["remote_addr"])
			for key, value := range tt.want {
				require.Equal(t, value, line[key], key)
			}
			if !tt.ui {
				require.NotContains(t, line, "client")
			}
		})
	}
}

func TestAccessRecorder_unwrap(t *testing.T) {
	// disableWriteDeadline and streaming go through http.ResponseController,
	// which must still reach the real writer behind the recorder.
	inner := httptest.NewRecorder()
	rec := &accessRecorder{ResponseWriter: inner}
	require.NoError(t, http.NewResponseController(rec).Flush())
	require.True(t, inner.Flushed)
}
