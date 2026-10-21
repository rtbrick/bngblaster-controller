// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// serveTestInfo answers every connection on the control socket in folder
// with the test-info returned by info for the n-th request (starting at 0),
// as bngblaster does: one response per connection, then close.
func serveTestInfo(t *testing.T, folder string, info func(n int) map[string]any) {
	t.Helper()
	ln, err := net.Listen("unix", path.Join(folder, RunSockFilename))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var requests atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			var command SocketCommand
			_ = json.NewDecoder(c).Decode(&command)
			n := int(requests.Add(1)) - 1
			_ = json.NewEncoder(c).Encode(map[string]any{
				"status": "ok", "code": 200, "test-info": info(n),
			})
			_ = c.Close()
		}
	}()
}

func TestWaitForStartup(t *testing.T) {
	failed := errors.New("exit status 1")
	tests := []struct {
		name string
		// info is the test-info of the n-th request, nil for no socket.
		info func(n int) map[string]any
		// exitAfter makes the process exit with an error after this
		// duration, 0 for a process that keeps running.
		exitAfter time.Duration
		wantErr   string
		// minWait is how long the wait has to take at least.
		minWait time.Duration
	}{
		{
			name: "older_version_without_status",
			info: func(int) map[string]any {
				return map[string]any{"state": "active", "duration": 0}
			},
		}, {
			name: "running",
			info: func(int) map[string]any {
				return map[string]any{"state": "active", "status": "running", "task": "running"}
			},
		}, {
			// The socket now exists from the beginning of the startup,
			// so it no longer signals that the instance came up.
			name: "waits_while_starting",
			info: func(n int) map[string]any {
				if n < 5 {
					return map[string]any{"status": "starting", "task": "init-interfaces"}
				}
				return map[string]any{"status": "running", "task": "running"}
			},
			minWait: 5 * startupPollInterval,
		}, {
			// An invalid configuration is detected after the socket was
			// opened: the error must still be reported.
			name: "fails_while_starting",
			info: func(n int) map[string]any {
				if n < 3 {
					return map[string]any{"status": "starting", "task": "load-config"}
				}
				return map[string]any{"status": "stopping", "task": "cleanup"}
			},
			exitAfter: 500 * time.Millisecond,
			wantErr:   "Error: invalid configuration",
		}, {
			name:      "fails_without_socket",
			exitAfter: 200 * time.Millisecond,
			wantErr:   "Error: invalid configuration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Short folder: unix socket paths are limited to 108 bytes.
			folder, err := os.MkdirTemp("", "bbl")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(folder) })
			require.NoError(t, os.WriteFile(path.Join(folder, RunStdErr),
				[]byte("Error: invalid configuration\n"), 0o600))
			if tt.info != nil {
				serveTestInfo(t, folder, tt.info)
			}
			done := make(chan error, 1)
			if tt.exitAfter > 0 {
				time.AfterFunc(tt.exitAfter, func() { done <- failed })
			}

			started := time.Now()
			err = waitForStartup(context.Background(), folder, done)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.GreaterOrEqual(t, time.Since(started), tt.minWait)
		})
	}
}

func TestWaitForStartup_aborted(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		// kill sends SIGKILL to the process once started, like _kill.
		kill    bool
		wantErr error
	}{
		{
			// test-stop or a signal handled during the startup.
			name:    "teardown_request",
			command: []string{"sh", "-c", "exit 3"},
			wantErr: ErrBlasterStartupAborted,
		}, {
			name:    "killed",
			command: []string{"sleep", "10"},
			kill:    true,
			wantErr: ErrBlasterStartupAborted,
		}, {
			name:    "failed",
			command: []string{"sh", "-c", "exit 1"},
			wantErr: errors.New("Error: invalid configuration"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := t.TempDir()
			require.NoError(t, os.WriteFile(path.Join(folder, RunStdErr),
				[]byte("Error: invalid configuration\n"), 0o600))
			cmd := exec.CommandContext(t.Context(), tt.command[0], tt.command[1:]...)
			require.NoError(t, cmd.Start())
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			if tt.kill {
				require.NoError(t, cmd.Process.Kill())
			}

			err := waitForStartup(context.Background(), folder, done)
			require.Equal(t, tt.wantErr, err)
		})
	}
}
