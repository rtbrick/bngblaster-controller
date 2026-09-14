// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package controller

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// ExecCommand exposes the exec command and allows therefore to test.
var ExecCommand = exec.Command

// RunCommand runs the command
// dir working directory the command is started in; relative file paths
// referenced by the command (e.g. a bngblaster config's isis mrt-file or
// bgp raw-update-file) resolve against this directory. Empty inherits the
// caller's own working directory.
// pidFile file that should be written with the pid
// stdFile file that should be written with the stdout
// errFile file that should be written with the stderr
// args first argument will be the command to execute, all the rest are arguments that are used for this command.
// The returned channel receives the command's exit error (nil on a clean
// exit) exactly once, once the process has terminated; it is buffered so a
// caller that stops waiting (e.g. after a startup grace period) never
// leaks the reporting goroutine.
func RunCommand(dir string, pidFile string, stdFile string, errFile string, args ...string) (chan error, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one argument need to be specified")
	}
	log.Info().Str("command", strings.Join(args, " ")).Msg("start Command")
	cmd := ExecCommand(args[0], args[1:]...)
	cmd.Dir = dir

	stdout, err := os.OpenFile(stdFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, permission)
	if err != nil {
		return nil, err
	}
	stderr, err := os.OpenFile(errFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, permission)
	if err != nil {
		return nil, err
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), permission)

	done := make(chan error, 1)
	go func() {
		waitErr := cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		_ = os.Remove(pidFile)
		done <- waitErr
		close(done)
		log.Info().Str("command", strings.Join(args, " ")).Msg("stopped Command")
	}()
	return done, nil
}
