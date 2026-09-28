// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

func writePidFileForRunning(t *testing.T, rootFolder string) {
	t.Helper()
	pidFile := path.Join(rootFolder, "running", runPidFilename)
	err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), permission)
	require.NoError(t, err)
}

func cleanupPidFileForRunning(t *testing.T, rootFolder string) {
	t.Helper()
	pidFile := path.Join(rootFolder, "running", runPidFilename)
	_ = os.Remove(pidFile)
}

func mustRead(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	return data
}

func TestNewDefaultRepository(t *testing.T) {
	tests := []struct {
		name string
		opts []DefaultRepositoryOption
		want *DefaultRepository
	}{
		{
			want: &DefaultRepository{
				executable:   DefaultExecutable,
				configFolder: DefaultConfigFolder,
			},
		}, {
			opts: []DefaultRepositoryOption{WithConfigFolder("test")},
			want: &DefaultRepository{
				executable:   DefaultExecutable,
				configFolder: "test",
			},
		}, {
			opts: []DefaultRepositoryOption{WithExecutable("test")},
			want: &DefaultRepository{
				executable:   "test",
				configFolder: DefaultConfigFolder,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDefaultRepository(tt.opts...)
			require.Equal(t, tt.want, got)
			require.Equal(t, got.ConfigFolder(), got.configFolder)
		})
	}
}

func TestDefaultRepository_CreateBngBlasterInstance(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder))
	tests := []struct {
		name             string
		instance         string
		config           []byte
		wantErr          error
		deleteAfterwards bool
	}{
		{
			instance:         "new_empty_config",
			config:           []byte(""),
			deleteAfterwards: true,
		}, {
			instance:         "new",
			config:           mustRead(t, "td/new_config.json"),
			deleteAfterwards: true,
		}, {
			instance:         "new",
			config:           mustRead(t, "td/new_second_config.json"),
			deleteAfterwards: true,
		}, {
			instance: "running",
			config:   mustRead(t, "td/new_second_config.json"),
			wantErr:  ErrBlasterRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := path.Join(rootFolder, tt.instance)
			defer func() {
				if tt.deleteAfterwards {
					_ = os.RemoveAll(folder)
				}
			}()
			if err := r.Create(tt.instance, tt.config); err != tt.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if _, err := os.Stat(folder); os.IsNotExist(err) {
				t.Fatalf("%s does not exist", folder)
			}
			config, err := r.config(tt.instance)
			require.NoError(t, err)
			require.Equal(t, tt.config, config)
		})
	}
}

func TestDefaultRepository_States(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder))
	tests := []struct {
		name        string
		wantExists  bool
		wantRunning bool
	}{
		{
			name: "new",
		}, {
			name:       "exists",
			wantExists: true,
		}, {
			name:        "running",
			wantExists:  true,
			wantRunning: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := r.Exists(tt.name), tt.wantExists; got != want {
				t.Fatalf("Exists() got = %v, want %v", got, want)
			}
			if got, want := r.Running(tt.name), tt.wantRunning; got != tt.wantRunning {
				t.Fatalf("Running() got = %v, want %v", got, want)
			}
		})
	}
}

func TestDefaultRepository_commandlineParameters(t *testing.T) {
	const rootFolder = "td"
	absRoot, err := filepath.Abs(rootFolder)
	require.NoError(t, err)
	r := NewDefaultRepository(WithConfigFolder(rootFolder))
	tests := []struct {
		name          string
		runningConfig RunningConfig
		want          []string
	}{
		{
			name:          "default",
			runningConfig: RunningConfig{},
			want: []string{
				DefaultExecutable,
				"-C", "td/default/config.json",
				"-S", "td/default/run.sock",
			},
		}, {
			name: "all",
			runningConfig: RunningConfig{
				Logging:           true,
				Report:            true,
				LoggingFlags:      []string{"error", "ip"},
				PCAPCapture:       true,
				PPPoESessionCount: 1000,
			},
			want: []string{
				DefaultExecutable,
				"-C", "td/all/config.json",
				"-S", "td/all/run.sock",
				"-J", "td/all/run_report.json",
				"-L", "td/all/run.log",
				"-l", "error",
				"-l", "ip",
				"-P", "td/all/run.pcap",
				"-c", "1000",
			},
		}, {
			name: "stream config relative path",
			runningConfig: RunningConfig{
				StreamConfig: "streams.json",
			},
			want: []string{
				DefaultExecutable,
				"-C", "td/stream config relative path/config.json",
				"-S", "td/stream config relative path/run.sock",
				"-T", "td/stream config relative path/streams.json",
			},
		}, {
			name: "stream config absolute path",
			runningConfig: RunningConfig{
				StreamConfig: filepath.Join(absRoot, "stream config absolute path", "streams.json"),
			},
			want: []string{
				DefaultExecutable,
				"-C", "td/stream config absolute path/config.json",
				"-S", "td/stream config absolute path/run.sock",
				"-T", filepath.Join(absRoot, "stream config absolute path", "streams.json"),
			},
		}, {
			// Absolute paths outside the instance folder (e.g. a home
			// directory) are passed through unchanged.
			name: "stream config absolute path outside instance",
			runningConfig: RunningConfig{
				StreamConfig: "/home/user/tests/streams.json",
			},
			want: []string{
				DefaultExecutable,
				"-C", "td/stream config absolute path outside instance/config.json",
				"-S", "td/stream config absolute path outside instance/run.sock",
				"-T", "/home/user/tests/streams.json",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.commandlineParameters(tt.name, tt.runningConfig)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultRepository_commandlineParameters_rejectsRelativeStreamConfigEscape(t *testing.T) {
	// A relative stream config must not climb out of the instance folder;
	// files elsewhere have to be referenced by an absolute path.
	r := NewDefaultRepository(WithConfigFolder("td"))
	for _, streamConfig := range []string{
		"../other/streams.json",
		"sub/../../streams.json",
		".",
	} {
		t.Run(streamConfig, func(t *testing.T) {
			_, err := r.commandlineParameters("test", RunningConfig{StreamConfig: streamConfig})
			require.ErrorIs(t, err, ErrInvalidStreamConfig)
		})
	}
}

func TestDefaultRepository_Start(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = fakeExecCommand
	defer func() { ExecCommand = defaultExecCommand }()

	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder), WithExecutable("test"))
	tests := []struct {
		name          string
		runningConfig RunningConfig
		wantErr       bool
		expOut        string
	}{
		{
			name:          "instance_not_found",
			runningConfig: RunningConfig{},
			wantErr:       true,
		}, {
			name:          "running",
			runningConfig: RunningConfig{},
			wantErr:       true,
		}, {
			name:          "exists",
			runningConfig: RunningConfig{},
			wantErr:       false,
			expOut:        "test -C td/exists/config.json -S td/exists/run.sock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.Start(context.Background(), tt.name, tt.runningConfig); (err != nil) != tt.wantErr {
				t.Fatalf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			time.Sleep(1 * time.Second)
			stdoutFile := path.Join(rootFolder, tt.name, RunStdOut)
			got := mustRead(t, stdoutFile)
			want := tt.expOut
			require.Equal(t, want, string(got))
		})
	}
}

func TestDefaultRepository_Delete(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	folder := path.Join(rootFolder, "exists_copy")
	_ = os.Mkdir(folder, permission)
	_ = os.WriteFile(path.Join(folder, "config.json"), []byte("{}"), permission)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		t.Fatalf("%s does not exist", folder)
	}

	r := NewDefaultRepository(WithConfigFolder(rootFolder), WithExecutable("test"))
	tests := []struct {
		name    string
		wantErr bool
		expOut  string
	}{
		{
			name:    "instance_not_found",
			wantErr: false,
		}, {
			name:    "running",
			wantErr: true,
		}, {
			name:    "exists_copy",
			wantErr: false,
			expOut:  "test -C td/exists/config.json -S td/exists/run.sock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.Delete(tt.name); (err != nil) != tt.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			folder := path.Join(rootFolder, tt.name)
			if _, err := os.Stat(folder); !os.IsNotExist(err) {
				t.Fatalf("%s does exist", folder)
			}
		})
	}
}

func TestDefaultRepository_Command(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder), WithExecutable("test"))
	tests := []struct {
		name            string
		command         SocketCommand
		startEchoServer bool
		wantErr         bool
		expOut          string
	}{
		{
			name: "instance_not_found",
			command: SocketCommand{
				Command: "session-counters",
				Arguments: map[string]any{
					"outer-vlan": 1,
					"inner-vlan": 1,
					"group":      "232.1.1.3",
					"source1":    "100.0.0.10",
					"source2":    "100.0.0.11",
					"source3":    "100.0.0.12",
				},
			},
			wantErr: true,
		}, {
			name:    "exists",
			command: SocketCommand{},
			wantErr: true,
		}, {
			name:            "running",
			command:         SocketCommand{},
			startEchoServer: true,
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.startEchoServer {
				// open socket
				file := path.Join(r.ConfigFolder(), tt.name, RunSockFilename)
				ln, err := net.Listen("unix", file)
				require.NoError(t, err)
				defer func() {
					_ = ln.Close()
					_ = os.Remove(file)
				}()
				go func() {
					fd, err := ln.Accept()
					if err == nil {
						echoHandler(fd)
					}
				}()
			}
			result, err := r.Command(tt.name, tt.command)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Command() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			var cr SocketCommand
			err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
			require.NoError(t, err)
			require.Equal(t, tt.command, cr)
		})
	}
}

func echoHandler(c net.Conn) {
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			return
		}

		data := buf[0:nr]
		log.Info().Msgf("Server got: %s", string(data))
		_, err = c.Write(data)
		if err != nil {
			log.Fatal().Msgf("Writing client error: %v", err)
		}
		_ = c.Close()
	}
}

func TestDefaultRepository_Signal(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder))

	// Ask for SIGHUP
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(c)

	r.Stop("running")
	waitSig(t, c, os.Interrupt)

	// can't be tested because this kills the test :-)
	// r.Kill("running")
	// waitSig(t, c, os.Kill)
}

func waitSig(t *testing.T, c <-chan os.Signal, sig os.Signal) {
	t.Helper()
	settleTime := time.Second
	// Sleep multiple times to give the kernel more tries to
	// deliver the signal.
	start := time.Now()
	timer := time.NewTimer(settleTime / 10)
	defer timer.Stop()
	// If the caller notified for all signals on c, filter out SIGURG,
	// which is used for runtime preemption and can come at unpredictable times.
	// General user code should filter out all unexpected signals instead of just
	// SIGURG, but since os/signal is tightly coupled to the runtime it seems
	// appropriate to be stricter here.
	for time.Since(start) < settleTime {
		select {
		case s := <-c:
			if s == sig {
				return
			}
			if s != syscall.SIGURG {
				t.Fatalf("signal was %v, want %v", s, sig)
			}
		case <-timer.C:
			timer.Reset(settleTime / 10)
		}
	}
	t.Fatalf("timeout after %v waiting for %v", settleTime, sig)
}

func TestDefaultRepository_Start_returnsWhenTheCallerGivesUp(t *testing.T) {
	// A process that stays alive without ever creating a control socket:
	// exactly the case Start waits out, up to startupMaxWait.
	defaultExecCommand := ExecCommand
	ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(t.Context(), "sleep", "10")
	}
	defer func() { ExecCommand = defaultExecCommand }()

	// Its own config folder: Start writes run files into the instance folder,
	// and the checked-in td/ fixtures are shared with the other tests.
	configFolder := t.TempDir()
	folder := path.Join(configFolder, "instance")
	require.NoError(t, os.MkdirAll(folder, 0o700))
	r := NewDefaultRepository(WithConfigFolder(configFolder), WithExecutable("test"))

	// The caller's HTTP client has gone away. The instance has been spawned
	// either way; only the observation of its outcome is abandoned, so Start
	// must return at once instead of blocking for the full startup window.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx, "instance", RunningConfig{}) }()

	select {
	case err := <-done:
		require.NoError(t, err)
		if waited := time.Since(started); waited >= 2*time.Second {
			t.Fatalf("Start() waited %s: it ignored the cancelled context and "+
				"blocked on the process instead", waited)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() ignored the cancelled context and kept waiting")
	}

	// Leave no stray process behind.
	if piddata, err := os.ReadFile(path.Join(folder, runPidFilename)); err == nil {
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func TestDefaultRepository_ignoresPidsThatCannotBeAnInstance(t *testing.T) {
	// 0 and negative pids address process groups in kill(2) and 1 is init;
	// a pid file holding one of them (corrupt, or planted) must never be
	// signalled. The negated pgid of the test itself is the case that is
	// observable without root: kill(-pgid, 0) succeeds for our own group.
	// Stop and Kill share pid(), but are not exercised with it here since a
	// regression would interrupt the whole test run instead of failing it.
	ownGroup := strconv.Itoa(-syscall.Getpgrp())
	for _, content := range []string{"0", "1", "-1", ownGroup, "", "abc"} {
		t.Run(content, func(t *testing.T) {
			folder := t.TempDir()
			require.NoError(t, os.MkdirAll(path.Join(folder, "test"), permission))
			pidFile := path.Join(folder, "test", runPidFilename)
			require.NoError(t, os.WriteFile(pidFile, []byte(content), permission))

			r := NewDefaultRepository(WithConfigFolder(folder))
			require.False(t, r.Running("test"))
			require.NoFileExists(t, pidFile, "a stale pid file is cleaned up")
		})
	}
}

func TestIsRunFile(t *testing.T) {
	for _, name := range []string{
		runPidFilename, RunSockFilename, RunConfigFilename, RunLogFilename,
		RunReportFilename, RunPcapFilename, RunStdErr, RunStdOut,
	} {
		require.True(t, IsRunFile(name), name)
	}
	// config.json and user files are legitimately replaced by uploads.
	for _, name := range []string{ConfigFilename, "streams.json", "run.pid.bak"} {
		require.False(t, IsRunFile(name), name)
	}
}

func TestDefaultRepository_Instances(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(path.Join(folder, "a"), permission))
	require.NoError(t, os.MkdirAll(path.Join(folder, "b"), permission))
	require.NoError(t, os.WriteFile(path.Join(folder, "not-an-instance"), nil, permission))

	require.Equal(t, []string{"a", "b"}, NewDefaultRepository(WithConfigFolder(folder)).Instances())

	missing := NewDefaultRepository(WithConfigFolder(path.Join(folder, "missing")))
	require.Equal(t, []string{}, missing.Instances(), "a missing config folder is an empty list, not nil")
}

func TestDefaultRepository_Files(t *testing.T) {
	folder := t.TempDir()
	instance := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(path.Join(instance, "subdir"), permission))
	require.NoError(t, os.WriteFile(path.Join(instance, ConfigFilename), []byte("{}"), permission))
	require.NoError(t, os.WriteFile(path.Join(instance, RunLogFilename), []byte("log"), permission))
	require.NoError(t, os.WriteFile(path.Join(instance, runPidFilename), []byte("42"), permission))
	require.NoError(t, os.WriteFile(path.Join(instance, RunSockFilename), nil, permission))

	r := NewDefaultRepository(WithConfigFolder(folder))
	files, err := r.Files("test")
	require.NoError(t, err)
	// The pid file and socket are internal and directories are skipped.
	require.ElementsMatch(t, []InstanceFile{
		{Name: ConfigFilename, Size: 2},
		{Name: RunLogFilename, Size: 3},
	}, files)

	_, err = r.Files("missing")
	require.ErrorIs(t, err, ErrBlasterNotExists)
}
