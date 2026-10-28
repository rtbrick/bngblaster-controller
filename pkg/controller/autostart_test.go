// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeSysClassNet returns a folder listing the interfaces names, like
// /sys/class/net.
func fakeSysClassNet(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		require.NoError(t, os.Mkdir(path.Join(dir, name), 0o700))
	}
	return dir
}

func TestCountInterfaces(t *testing.T) {
	tests := []struct {
		name       string
		interfaces []string
		want       int
	}{
		{name: "none", want: 0},
		{name: "management_only", interfaces: []string{"lo", "eth0"}, want: 0},
		{name: "fallback_tunnels", interfaces: []string{"lo", "eth0", "tunl0", "sit0", "gre0", "gretap0"}, want: 0},
		{name: "data", interfaces: []string{"lo", "eth0", "eth1", "eth2"}, want: 2},
		// Names are user-defined: everything else counts.
		{name: "custom_names", interfaces: []string{"lo", "eth0", "access1", "net-a"}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := countInterfaces(fakeSysClassNet(t, tt.interfaces...))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAutostart_waitForInterfaces(t *testing.T) {
	tests := []struct {
		name string
		want int
		// attach is attached after the wait has begun.
		attach  []string
		timeout time.Duration
		wantErr string
	}{
		{name: "present", want: 1},
		{name: "attached_later", want: 2, attach: []string{"eth2"}, timeout: 5 * time.Second},
		{
			name: "timeout", want: 3, timeout: 100 * time.Millisecond,
			wantErr: "timed out after 100ms waiting for 3 interfaces, found 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakeSysClassNet(t, "lo", "eth0", "eth1")
			a := NewAutostart(&RepositoryMock{}, "test", WithWaitInterfaces(tt.want, tt.timeout))
			a.sysClassNet = dir
			a.pollInterval = 10 * time.Millisecond
			for _, name := range tt.attach {
				time.AfterFunc(50*time.Millisecond, func() { _ = os.Mkdir(path.Join(dir, name), 0o700) })
			}
			err := a.waitForInterfaces(t.Context())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAutostart_Run(t *testing.T) {
	tests := []struct {
		name string
		// config and runConfig are the content of config.json and
		// run.json, nil for no file.
		config    []byte
		runConfig []byte
		// interfaces to wait for; the fake sysfs has one.
		interfaces int
		startErr   error
		wantState  string
		wantError  string
		wantConfig RunningConfig
	}{
		{
			name: "defaults", config: []byte("{}"),
			wantState: AutostartStarted,
		}, {
			name: "run_json", config: []byte("{}"),
			runConfig:  []byte(`{"report": true, "logging": true, "logging_flags": ["bgp"], "unknown": 1}`),
			wantState:  AutostartStarted,
			wantConfig: RunningConfig{Report: true, Logging: true, LoggingFlags: []string{"bgp"}},
		}, {
			name: "invalid_run_json", config: []byte("{}"), runConfig: []byte(`{"report": "yes"}`),
			wantState: AutostartFailed, wantError: "invalid run.json",
		}, {
			name: "empty_run_json", config: []byte("{}"), runConfig: []byte{},
			wantState: AutostartFailed, wantError: "invalid run.json: EOF",
		}, {
			name:      "no_config",
			wantState: AutostartFailed, wantError: "no config.json for instance test",
		}, {
			name: "start_failed", config: []byte("{}"), startErr: errors.New("Error: invalid configuration"),
			wantState: AutostartFailed, wantError: "Error: invalid configuration",
		}, {
			name: "running_already", config: []byte("{}"), startErr: ErrBlasterRunning,
			wantState: AutostartStarted,
		}, {
			name: "interfaces_timeout", config: []byte("{}"), interfaces: 2,
			wantState: AutostartFailed, wantError: "timed out after 50ms waiting for 2 interfaces, found 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFolder := t.TempDir()
			folder := path.Join(configFolder, "test")
			require.NoError(t, os.Mkdir(folder, 0o700))
			if tt.config != nil {
				require.NoError(t, os.WriteFile(path.Join(folder, ConfigFilename), tt.config, 0o600))
			}
			if tt.runConfig != nil {
				require.NoError(t, os.WriteFile(path.Join(folder, RunConfigFilename), tt.runConfig, 0o600))
			}
			var started []RunningConfig
			repository := &RepositoryMock{
				ConfigFolderFunc: func() string { return configFolder },
				StartFunc: func(_ context.Context, name string, runningConfig RunningConfig) error {
					require.Equal(t, "test", name)
					started = append(started, runningConfig)
					return tt.startErr
				},
			}
			a := NewAutostart(repository, "test", WithWaitInterfaces(tt.interfaces, 50*time.Millisecond))
			a.sysClassNet = fakeSysClassNet(t, "lo", "eth0", "eth1")
			a.pollInterval = 10 * time.Millisecond
			require.Equal(t, AutostartWaitingInterfaces, a.Status().State)

			a.Run(t.Context())
			status := a.Status()
			require.Equal(t, tt.wantState, status.State)
			require.Contains(t, status.Error, tt.wantError)
			if tt.wantError == "" {
				require.Empty(t, status.Error)
			}
			if tt.startErr != nil || tt.wantState == AutostartStarted {
				require.Equal(t, []RunningConfig{tt.wantConfig}, started)
			} else {
				require.Empty(t, started)
			}
		})
	}
}

func TestAutostart_Run_cancelled(t *testing.T) {
	// A shutdown while waiting for interfaces is not a failure.
	a := NewAutostart(&RepositoryMock{}, "test", WithWaitInterfaces(1, 0))
	a.sysClassNet = fakeSysClassNet(t, "lo", "eth0")
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	a.Run(ctx)
	require.Equal(t, AutostartStatus{Instance: "test", State: AutostartWaitingInterfaces}, a.Status())
}

func TestAutostart_StopInstance(t *testing.T) {
	var running atomic.Bool
	running.Store(true)
	repository := &RepositoryMock{
		RunningFunc: func(string) bool { return running.Load() },
		StopFunc: func(string) {
			// bngblaster takes a moment for its teardown.
			time.AfterFunc(200*time.Millisecond, func() { running.Store(false) })
		},
	}
	a := NewAutostart(repository, "test")
	a.StopInstance(t.Context())
	require.False(t, running.Load())
	require.Len(t, repository.StopCalls(), 1)

	// Nothing to stop.
	a.StopInstance(t.Context())
	require.Len(t, repository.StopCalls(), 1)
}

func TestExitCode(t *testing.T) {
	run := func(name string, args ...string) error {
		return exec.CommandContext(t.Context(), name, args...).Run()
	}
	tests := []struct {
		name    string
		waitErr error
		want    int
		wantOK  bool
	}{
		{name: "success", waitErr: nil, want: 0, wantOK: true},
		{name: "exit_code", waitErr: run("sh", "-c", "exit 5"), want: 5, wantOK: true},
		{name: "signal", waitErr: run("sh", "-c", "kill -9 $$"), want: 137, wantOK: true},
		{name: "not_waited", waitErr: errors.New("wait failed"), wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := exitCode(tt.waitErr)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultRepository_ExitCode(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(t.Context(), "sh", "-c", "exit 5")
	}
	defer func() { ExecCommand = defaultExecCommand }()

	configFolder := t.TempDir()
	require.NoError(t, os.Mkdir(path.Join(configFolder, "instance"), 0o700))
	r := NewDefaultRepository(WithConfigFolder(configFolder), WithExecutable("test"))
	_, ok := r.ExitCode("instance")
	require.False(t, ok, "no run yet")

	require.EqualError(t, r.Start(t.Context(), "instance", RunningConfig{}), "exit status 5")
	code, ok := r.ExitCode("instance")
	require.True(t, ok)
	require.Equal(t, 5, code)

	// A new configuration starts over.
	require.NoError(t, r.Create("instance", []byte("{}")))
	_, ok = r.ExitCode("instance")
	require.False(t, ok)
}

func TestWriteRunConfig(t *testing.T) {
	file := path.Join(t.TempDir(), RunConfigFilename)
	// The user's own run.json, as mounted into a container.
	mounted := []byte("{\n  \"report\": true\n}\n")
	require.NoError(t, os.WriteFile(file, mounted, 0o600))

	// The same arguments leave the file as it is.
	require.NoError(t, writeRunConfig(file, RunningConfig{Report: true}))
	got, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, mounted, got)

	// Other arguments replace it.
	require.NoError(t, writeRunConfig(file, RunningConfig{Logging: true}))
	runningConfig, err := ReadRunConfig(file)
	require.NoError(t, err)
	require.Equal(t, RunningConfig{Logging: true}, runningConfig)
}
