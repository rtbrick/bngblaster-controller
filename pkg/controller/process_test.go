// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2024, RtBrick, Inc.
package controller

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	pidFile    = "td/pid"
	stdoutFile = "td/out"
	stderrFile = "td/err"
)

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "error":
		_, _ = fmt.Fprint(os.Stdout, cmd)
		for _, s := range args {
			_, _ = fmt.Fprintf(os.Stdout, " %s", s)
		}
		os.Exit(2)
	default:
		_, _ = fmt.Fprint(os.Stdout, cmd)
		for _, s := range args {
			_, _ = fmt.Fprintf(os.Stdout, " %s", s)
		}
	}
	os.Exit(0)
}

func TestApplication_ExecCommand(t *testing.T) {
	defaultExecCommand := ExecCommand
	ExecCommand = fakeExecCommand
	defer func() { ExecCommand = defaultExecCommand }()
	tcs := []struct {
		command []string
		expOut  []byte
		wantErr bool
	}{
		{command: nil, wantErr: true, expOut: nil},
		{command: []string{}, wantErr: true, expOut: nil},
		{command: []string{"test"}, wantErr: false, expOut: []byte("test")},
		{command: []string{"/lib/platform-config/current/onl/bin/onlpdump"}, wantErr: false, expOut: []byte("/lib/platform-config/current/onl/bin/onlpdump")},
		{command: []string{"/lib/platform-config/current/onl/bin/onlpdump", "-la"}, wantErr: false, expOut: []byte("/lib/platform-config/current/onl/bin/onlpdump -la")},
		{command: []string{"onlpdump"}, wantErr: false, expOut: []byte("onlpdump")},
		{command: []string{"./onlpdump"}, wantErr: false, expOut: []byte("./onlpdump")},
		{command: []string{"sleep", "5"}, wantErr: false, expOut: []byte("sleep 5")},
		// error in this case not true because we are not checking the outcome of a long running process.
		{command: []string{"error", "-la"}, wantErr: false, expOut: []byte("error -la")},
	}
	for i, tt := range tcs {
		t.Run(fmt.Sprintf("Number %d", i), func(t *testing.T) {
			defer func() {
				_ = os.Remove(stderrFile)
			}()
			defer func() {
				_ = os.Remove(stdoutFile)
			}()
			done, err := RunCommand(pidFile, stdoutFile, stderrFile, tt.command...)
			if (err == nil) == tt.wantErr {
				t.Fatalf("RunCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if done != nil {
				<-done
			}
			got, err := os.ReadFile(stdoutFile)
			require.NoError(t, err)
			want := tt.expOut
			require.Equal(t, want, got)
		})
	}
}

func TestApplication_runCommandNoTimeOut(t *testing.T) {
	defer func() {
		_ = os.Remove(stderrFile)
	}()
	defer func() {
		_ = os.Remove(stdoutFile)
	}()
	_, err := RunCommand(pidFile, stdoutFile, stderrFile, "sleep", "10")
	require.NoError(t, err)
}

func TestApplication_ExecCommand_Real(t *testing.T) {
	tcs := []struct {
		command []string
		expOut  []byte
		wantErr bool
	}{
		{command: nil, wantErr: true, expOut: nil},
		{command: []string{}, wantErr: true, expOut: nil},
		{command: []string{"not_found"}, wantErr: true, expOut: nil},
		{command: []string{"sleep", "1"}, wantErr: false, expOut: []byte("")},
		//{command: []string{"go", "-test"}, wantErr: false, expOut: []byte("")},
	}
	for i, tt := range tcs {
		t.Run(fmt.Sprintf("Number %d", i), func(t *testing.T) {
			defer func() {
				_ = os.Remove(stderrFile)
			}()
			defer func() {
				_ = os.Remove(stdoutFile)
			}()
			done, err := RunCommand(pidFile, stdoutFile, stderrFile, tt.command...)
			if (err == nil) == tt.wantErr {
				t.Fatalf("RunCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if done != nil {
				<-done
			}
			got := mustRead(t, stdoutFile)
			want := tt.expOut
			require.Equal(t, string(got), string(want))
		})
	}
}
