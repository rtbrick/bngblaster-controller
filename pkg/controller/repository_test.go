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
	"sync/atomic"
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
	ownProcessIsInstance(t)
}

// ownProcessIsInstance makes the test process count as the bngblaster of
// any instance whose run.pid holds its pid, for tests that fake a running
// instance that way; other pids still get the real check.
func ownProcessIsInstance(t *testing.T) {
	t.Helper()
	isInstanceProcess = func(pid int, socket string, executable string) bool {
		return pid == os.Getpid() || procIsInstanceProcess(pid, socket, executable)
	}
	t.Cleanup(func() { isInstanceProcess = procIsInstanceProcess })
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
	// exactly the case Start waits out until the instance exits.
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
	// signalled. The negated pgid of the test itself stands for a value that
	// would reach real processes. Stop and Kill are not exercised with it
	// here since a regression would interrupt the whole test run instead of
	// failing it.
	ownGroup := strconv.Itoa(-syscall.Getpgrp())
	for _, content := range []string{"0", "1", "-1", ownGroup, "", "abc"} {
		t.Run(content, func(t *testing.T) {
			folder := t.TempDir()
			require.NoError(t, os.MkdirAll(path.Join(folder, "test"), permission))
			pidFile := path.Join(folder, "test", runPidFilename)
			require.NoError(t, os.WriteFile(pidFile, []byte(content), permission))

			r := NewDefaultRepository(WithConfigFolder(folder))
			require.False(t, r.Running("test"))
			require.FileExists(t, pidFile, "Running only reads")
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

func TestDefaultRepository_cleanupRunFiles_removesRotatedLogs(t *testing.T) {
	configFolder := t.TempDir()
	folder := path.Join(configFolder, "instance")
	require.NoError(t, os.Mkdir(folder, 0o700))
	rotated := []string{"run.log", "run.log.1", "run.log.2.gz", "run.log-20261028", "run.log-20261027.gz"}
	kept := []string{"config.json", "run.logs", "my-run.log.1", "streams.json"}
	for _, name := range append(rotated, kept...) {
		require.NoError(t, os.WriteFile(path.Join(folder, name), []byte("x"), 0o600))
	}
	r := NewDefaultRepository(WithConfigFolder(configFolder))
	require.NoError(t, r.Create("instance", []byte("{}")))

	for _, name := range rotated {
		require.NoFileExists(t, path.Join(folder, name))
	}
	for _, name := range kept {
		require.FileExists(t, path.Join(folder, name))
	}
}

// The repository must never act on the config folder itself, whatever
// name a caller passes: Delete("") used to remove the whole folder.
func TestDefaultRepository_invalidInstanceName(t *testing.T) {
	rootFolder := t.TempDir()
	marker := filepath.Join(rootFolder, "other", ConfigFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("{}"), 0o600))

	r := NewDefaultRepository(WithConfigFolder(rootFolder))
	for _, name := range []string{"", ".", "..", "a/b", "a\x00b"} {
		require.ErrorIs(t, r.Delete(name), ErrInvalidInstanceName, "Delete(%q)", name)
		require.ErrorIs(t, r.Create(name, []byte("{}")), ErrInvalidInstanceName, "Create(%q)", name)
		require.ErrorIs(t, r.Start(t.Context(), name, RunningConfig{}), ErrInvalidInstanceName, "Start(%q)", name)
	}
	require.FileExists(t, marker)
	require.NoFileExists(t, filepath.Join(rootFolder, ConfigFilename))
}

// pcap-start makes bngblaster (root) create or truncate its "file" as
// given, so it must stay a plain file name inside the instance folder.
func TestCheckCommandFile(t *testing.T) {
	pcap := func(file any) SocketCommand {
		return SocketCommand{Command: "pcap-start", Arguments: map[string]any{"file": file}}
	}
	for _, tt := range []struct {
		name    string
		command SocketCommand
		wantErr bool
	}{
		{"no arguments", SocketCommand{Command: "pcap-start"}, false},
		{"plain name", pcap("capture.pcap"), false},
		{"regular capture file", pcap(RunPcapFilename), false},
		{"absolute path", pcap("/dev/sda"), true},
		{"other instance", pcap("../other/run.pid"), true},
		{"subdirectory", pcap("sub/capture.pcap"), true},
		{"parent", pcap(".."), true},
		{"hidden or generator temp file", pcap(".gen-x"), true},
		{"empty", pcap(""), true},
		{"nul", pcap("a\x00b"), true},
		{"not a string", pcap(42), true},
		{"config", pcap(ConfigFilename), true},
		{"pid file", pcap(runPidFilename), true},
		{"socket", pcap(RunSockFilename), true},
		{"report", pcap(RunReportFilename), true},
		// Commands that only read their file are not restricted.
		{"read-only file command", SocketCommand{Command: "bgp-raw-update", Arguments: map[string]any{"file": "/home/lab/update.bgp"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCommandFile(tt.command)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidCommandFile)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultRepository_Command_checksFile(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)
	defer cleanupPidFileForRunning(t, rootFolder)

	r := NewDefaultRepository(WithConfigFolder(rootFolder))
	_, err := r.Command("running", SocketCommand{Command: "pcap-start", Arguments: map[string]any{"file": "/dev/sda"}})
	require.ErrorIs(t, err, ErrInvalidCommandFile)
}

// startProcess starts a process that lives until the test ends, with args
// on its command line, and returns its pid once that command line is set
// (it is empty for a moment after the start). sh -c runs two commands, so it does not
// exec sleep in its place and keeps its own command line.
func startProcess(t *testing.T, args ...string) int {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "sh", append([]string{"-c", "sleep 30; :"}, args...)...) //nolint:gosec // fixed test command
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	cmdline := fmt.Sprintf("/proc/%d/cmdline", cmd.Process.Pid)
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(cmdline)
		return err == nil && len(data) > 0
	}, 5*time.Second, time.Millisecond, "command line of the started process not set")
	return cmd.Process.Pid
}

// requireNotKilled fails if process pid, a child of the test, got killed.
// It would stay a zombie, which kill(pid, 0) cannot tell from a live process.
func requireNotKilled(t *testing.T, pid int) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	require.NoError(t, err)
	require.NotContains(t, string(stat), ") Z ", "the process was killed")
}

func TestProcIsInstanceProcess(t *testing.T) {
	folder := t.TempDir()
	socket := path.Join(folder, "test", RunSockFilename)
	pid := startProcess(t, "-S", socket)

	require.True(t, procIsInstanceProcess(pid, socket, DefaultExecutable))
	// A relative and an absolute spelling of the same path match.
	wd, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(wd, socket)
	require.NoError(t, err)
	require.True(t, procIsInstanceProcess(pid, relative, DefaultExecutable))
	// Another instance's socket, a process without -S, and a gone process
	// are not the instance.
	require.False(t, procIsInstanceProcess(pid, path.Join(folder, "other", RunSockFilename), DefaultExecutable))
	require.False(t, procIsInstanceProcess(startProcess(t), socket, DefaultExecutable))
	require.False(t, procIsInstanceProcess(os.Getpid(), socket, DefaultExecutable))
	require.False(t, procIsInstanceProcess(1<<22+1, socket, DefaultExecutable))
}

// A run.pid left behind by a reboot or a container restart may hold the
// pid of an unrelated process by now: the instance must count as stopped,
// and _stop/_kill must not signal that process.
func TestDefaultRepository_stalePidOfOtherProcess(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(path.Join(folder, "test"), permission))
	pidFile := path.Join(folder, "test", runPidFilename)
	other := startProcess(t)
	r := NewDefaultRepository(WithConfigFolder(folder))

	require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(other)), permission))
	r.Kill("test")
	r.Stop("test")
	requireNotKilled(t, other)

	// Not running, but Running never touches run.pid.
	require.False(t, r.Running("test"))
	require.FileExists(t, pidFile)

	// The real bngblaster of the instance is still recognised.
	own := startProcess(t, "-S", path.Join(folder, "test", RunSockFilename))
	require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(own)), permission))
	require.True(t, r.Running("test"))
}

// A run.pid whose process has exited is ignored, and left to the next
// Start, Create or Delete: Running only reads, see Running.
func TestDefaultRepository_Running_ignoresPidOfExitedProcess(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(path.Join(folder, "test"), permission))
	pidFile := path.Join(folder, "test", runPidFilename)
	cmd := exec.CommandContext(t.Context(), "true")
	require.NoError(t, cmd.Run())
	require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), permission))

	r := NewDefaultRepository(WithConfigFolder(folder))
	require.False(t, r.Running("test"))
	require.FileExists(t, pidFile)
}

// run.pid is replaced atomically: a reader never sees it missing or partly
// written.
func TestWritePidFile_neverVisibleEmpty(t *testing.T) {
	folder := t.TempDir()
	pidFile := path.Join(folder, runPidFilename)
	require.NoError(t, writePidFile(pidFile, os.Getpid()))

	done := make(chan struct{})
	var partial atomic.Int32
	go func() {
		defer close(done)
		for range 20000 {
			data, err := os.ReadFile(pidFile)
			if err != nil || strings.TrimSpace(string(data)) != strconv.Itoa(os.Getpid()) {
				partial.Add(1)
			}
		}
	}()
	for {
		select {
		case <-done:
			require.Zero(t, partial.Load(), "run.pid was missing or incomplete while being rewritten")
			require.NoFileExists(t, path.Join(folder, runPidTempFilename))
			return
		default:
			require.NoError(t, writePidFile(pidFile, os.Getpid()))
		}
	}
}

// After a reboot, a stale run.pid may name a kernel thread (low pids):
// the instance is not running, and it must not stay "started" for good.
func TestDefaultRepository_Running_kernelThreadPid(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(path.Join(folder, "test"), permission))
	require.NoError(t, os.WriteFile(path.Join(folder, "test", runPidFilename), []byte("2"), permission))
	require.False(t, NewDefaultRepository(WithConfigFolder(folder)).Running("test"))
}

// A started process counts as the instance for as long as the controller
// has not reaped it, whatever its command line says: Running must not end
// before the exit code is recorded and run.pid is gone, or a new run could
// start and have both overwritten by the end of the old one.
func TestDefaultRepository_Running_untilReaped(t *testing.T) {
	folder := t.TempDir()
	dir := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(dir, permission))
	r := NewDefaultRepository(WithConfigFolder(folder))

	done, err := RunCommand(dir, path.Join(dir, runPidFilename), path.Join(dir, RunStdOut), path.Join(dir, RunStdErr), "sleep", "30")
	require.NoError(t, err)
	require.True(t, r.Running("test"), "started process without -S on its command line")

	// Not waiting for done, which comes after all of this: the moment the
	// instance is stopped, the run has to be over.
	r.Kill("test")
	for r.Running("test") {
		time.Sleep(100 * time.Microsecond)
	}
	code, ok := r.ExitCode("test")
	require.True(t, ok)
	require.Equal(t, exitSignalBase+int(syscall.SIGKILL), code)
	require.NoFileExists(t, path.Join(dir, runPidFilename))
	<-done
}

// startZombie starts a process from executable and kills it without
// reaping it, as a dying bngblaster looks until all its threads are gone.
func startZombie(t *testing.T, executable string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), executable, "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Wait() })
	require.NoError(t, cmd.Process.Kill())
	stat := fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid)
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(stat)
		return err == nil && strings.Contains(string(data), ") Z ")
	}, 5*time.Second, time.Millisecond, "process did not become a zombie")
	return cmd
}

// A killed bngblaster stays a zombie with an empty command line until it
// is reaped, and keeps its interface locks until then: the instance must
// not be reported stopped before, or a restart fails with "interface in
// use". A child of the controller is known by its entry in children.
func TestDefaultRepository_Running_zombieChild(t *testing.T) {
	folder := t.TempDir()
	dir := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(dir, permission))
	r := NewDefaultRepository(WithConfigFolder(folder))
	cmd := startZombie(t, "sleep")
	require.NoError(t, writePidFile(path.Join(dir, runPidFilename), cmd.Process.Pid))

	require.False(t, r.Running("test"), "zombie that is not known as a child")
	children.add(dir, cmd.Process)
	t.Cleanup(func() { children.remove(dir) })
	require.True(t, r.Running("test"))
	require.ErrorIs(t, r.Delete("test"), ErrBlasterRunning)
	require.ErrorIs(t, r.Create("test", []byte("{}")), ErrBlasterRunning)
}

// After a controller restart the instance is no child any more, and its
// dying bngblaster is only known by its name, which the kernel cuts off.
func TestDefaultRepository_Running_zombieAfterRestart(t *testing.T) {
	folder := t.TempDir()
	dir := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(dir, permission))
	sleep, err := exec.LookPath("sleep")
	require.NoError(t, err)
	executable := path.Join(t.TempDir(), "bngblaster-with-a-long-name")
	require.NoError(t, os.Symlink(sleep, executable))
	cmd := startZombie(t, executable)
	require.NoError(t, writePidFile(path.Join(dir, runPidFilename), cmd.Process.Pid))

	r := NewDefaultRepository(WithConfigFolder(folder), WithExecutable(executable))
	require.True(t, r.Running("test"), "dying bngblaster")
	r.Kill("test") // A zombie takes no signal, and this must not fail.
	other := NewDefaultRepository(WithConfigFolder(folder))
	require.False(t, other.Running("test"), "zombie of another program")

	_ = cmd.Wait()
	require.False(t, r.Running("test"), "reaped")
}

// The pid of a child is free from the moment it is reaped, while the child
// is still tracked until the run is over. A signal in that moment must not
// reach a process that got the pid: the reused pid is simulated here by
// pointing the handle of a reaped child at a live process.
func TestDefaultRepository_sendSignal_reapedChild(t *testing.T) {
	folder := t.TempDir()
	dir := path.Join(folder, "test")
	require.NoError(t, os.MkdirAll(dir, permission))
	r := NewDefaultRepository(WithConfigFolder(folder))
	reused := startProcess(t)

	cmd := exec.CommandContext(t.Context(), "true")
	require.NoError(t, cmd.Run())
	cmd.Process.Pid = reused
	children.add(dir, cmd.Process)
	t.Cleanup(func() { children.remove(dir) })
	require.NoError(t, writePidFile(path.Join(dir, runPidFilename), reused))

	r.Kill("test")
	requireNotKilled(t, reused)
}

// Of concurrent starts of an instance only one may spawn a process: the
// exit of a second one would end the tracking of the first.
func TestDefaultRepository_Start_concurrent(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(context.Background(), "sleep", "30")
	}
	defer func() { ExecCommand = defaultExecCommand }()
	r := NewDefaultRepository(WithConfigFolder(t.TempDir()))
	require.NoError(t, r.Create("test", []byte("{}")))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	const starts = 8
	results := make(chan error, starts)
	for range starts {
		go func() { results <- r.Start(ctx, "test", RunningConfig{}) }()
	}
	var started int
	for range starts {
		if err := <-results; err == nil {
			started++
		} else {
			require.ErrorIs(t, err, ErrBlasterRunning)
		}
	}
	require.Equal(t, 1, started)

	r.Kill("test")
	require.Eventually(t, func() bool { return !r.Running("test") }, 5*time.Second, time.Millisecond)
}

// Running leaves a stale run.pid in place, so the next run has to replace it,
// even if the pid is alive and belongs to something else by now.
func TestDefaultRepository_Start_replacesStalePid(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(context.Background(), "sleep", "30")
	}
	defer func() { ExecCommand = defaultExecCommand }()
	folder := t.TempDir()
	r := NewDefaultRepository(WithConfigFolder(folder))
	require.NoError(t, r.Create("test", []byte("{}")))
	pidFile := path.Join(folder, "test", runPidFilename)
	other := startProcess(t)
	require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(other)), permission))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	require.NoError(t, r.Start(ctx, "test", RunningConfig{}))
	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	require.NotEqual(t, strconv.Itoa(other), string(data))

	r.Kill("test")
	require.Eventually(t, func() bool { return !r.Running("test") }, 5*time.Second, time.Millisecond)
	requireNotKilled(t, other)
}
