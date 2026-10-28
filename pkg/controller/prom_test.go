// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"errors"
	"os"
	"path"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestProm_Collect(t *testing.T) {
	responses := map[string]string{
		"session-counters": `{"code": 200, "session-counters": {"sessions": 10, "sessions-established": 8}}`,
		"stream-summary":   `{"code": 200, "stream-summary": [{"flow-id": 1, "name": "s1", "tx-packets": 5}]}`,
	}
	tests := []struct {
		name     string
		command  func(command string) ([]byte, error)
		wantMore bool
	}{
		{
			name: "collected",
			command: func(command string) ([]byte, error) {
				return []byte(responses[command]), nil
			},
			wantMore: true,
		}, {
			name:    "command_failed",
			command: func(string) ([]byte, error) { return nil, errors.New("connection refused") },
		}, {
			name:    "invalid_response",
			command: func(string) ([]byte, error) { return []byte("not json"), nil },
		}, {
			name:    "command_error_code",
			command: func(string) ([]byte, error) { return []byte(`{"code": 500}`), nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFolder := t.TempDir()
			folder := path.Join(configFolder, "x")
			require.NoError(t, os.Mkdir(folder, 0o700))
			require.NoError(t, os.WriteFile(path.Join(folder, RunConfigFilename),
				[]byte(`{"metric_flags": ["session_counters", "streams"]}`), 0o600))
			repository := &RepositoryMock{
				ConfigFolderFunc: func() string { return configFolder },
				RunningFunc:      func(string) bool { return true },
				CommandFunc: func(_ string, command SocketCommand) ([]byte, error) {
					return tt.command(command.Command)
				},
			}
			p := NewProm(repository)

			// The instance totals are always there; the instance metrics
			// only when both commands answered.
			got := testutil.CollectAndCount(p)
			if tt.wantMore {
				require.Greater(t, got, 2)
				require.Equal(t, 1, testutil.CollectAndCount(p, metricSessions))
			} else {
				require.Equal(t, 2, got)
			}
			// One query per metric flag and collection.
			collections := 1
			if tt.wantMore {
				collections = 2
			}
			require.Len(t, repository.CommandCalls(), 2*collections)
		})
	}
}
