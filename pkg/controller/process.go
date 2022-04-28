package controller

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

//ExecCommand exposes the exec command and allows therefore to test
var ExecCommand = exec.Command

// RunCommand runs the command
// pidFile file that should be written with the pid
// stdFile file that should be written with the stdout
// errFile file that should be written with the stderr
// args first argument will be the command to execute, all the rest are arguments that are used for this command.
func RunCommand(pidFile string, stdFile string, errFile string, args ...string) (chan bool, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one argument need to be specified")
	}
	log.Info().Str("command", strings.Join(args, " ")).Msg("start Command")
	cmd := ExecCommand(args[0], args[1:]...)

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
	_ = ioutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), permission)

	done := make(chan bool)
	go func() {
		_ = cmd.Wait()
		_ = stdout.Close()
		_ = stderr.Close()
		_ = os.Remove(pidFile)
		close(done)
		log.Info().Str("command", strings.Join(args, " ")).Msg("stopped Command")
	}()
	return done, nil
}
