package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	isf "github.com/matryer/is"
	"github.com/rs/zerolog/log"
)

func writePidFileForRunning(t *testing.T, rootFolder string) {
	t.Helper()
	is := isf.New(t)
	pidFile := path.Join(rootFolder, "running", runPidFilename)
	err := ioutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), permission)
	is.NoErr(err)
}

func mustRead(t *testing.T, filename string) []byte {
	t.Helper()
	is := isf.New(t)
	data, err := ioutil.ReadFile(filename)
	is.NoErr(err)
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
			is := isf.New(t)
			got := NewDefaultRepository(tt.opts...)
			if diff := cmp.Diff(got, tt.want, cmp.AllowUnexported(DefaultRepository{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			is.Equal(got.ConfigFolder(), got.configFolder)
		})
	}
}

func TestDefaultRepository_CreateBngBlasterInstance(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)

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
			is := isf.New(t)
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
			is.NoErr(err)
			if diff := cmp.Diff(config, tt.config); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultRepository_States(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)

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
				"/usr/sbin/bngblaster",
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
				"/usr/sbin/bngblaster",
				"-C", "td/all/config.json",
				"-S", "td/all/run.sock",
				"-J", "td/all/run_report.json",
				"-L", "td/all/run.log",
				"-l", "error",
				"-l", "ip",
				"-P", "td/all/run.pcap",
				"-c", "1000",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := r.commandlineParameters(tt.name, tt.runningConfig), tt.want
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultRepository_Start(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = fakeExecCommand
	defer func() { ExecCommand = defaultExecCommand }()

	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)

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
			if err := r.Start(tt.name, tt.runningConfig); (err != nil) != tt.wantErr {
				t.Fatalf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			time.Sleep(1 * time.Second)
			stdoutFile := path.Join(rootFolder, tt.name, RunStdOut)
			got := mustRead(t, stdoutFile)
			want := tt.expOut
			if diff := cmp.Diff(string(got), want); diff != "" {
				t.Errorf("out mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDefaultRepository_Delete(t *testing.T) {
	const rootFolder = "td"
	writePidFileForRunning(t, rootFolder)

	folder := path.Join(rootFolder, "exists_copy")
	_ = os.Mkdir(folder, permission)
	_ = ioutil.WriteFile(path.Join(folder, "config.json"), []byte("{}"), permission)
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
				Arguments: map[string]interface{}{
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
			is := isf.New(t)
			if tt.startEchoServer {
				// open socket
				file := path.Join(r.ConfigFolder(), tt.name, RunSockFilename)
				ln, err := net.Listen("unix", file)
				is.NoErr(err)
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
			is.NoErr(err)
			if diff := cmp.Diff(tt.command, cr); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
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
