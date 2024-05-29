package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"syscall"
	"time"
)

const (
	// DefaultConfigFolder is the default config folder.
	DefaultConfigFolder = "/var/bngblaster"

	// DefaultExecutable is the default executable for bngblaster.
	DefaultExecutable = "/usr/sbin/bngblaster"

	// permission file and folder permissions to use.
	permission os.FileMode = 0o777

	readTimeout                = 5 * time.Second
	writeTimeout               = 5 * time.Second
	bufferLength               = 512
	initialReceiveBufferLength = 20000

	// ConfigFilename configuration file of the blaster.
	ConfigFilename = "config.json"
	// runPidFilename file that contains the process id of the bngblaster instance if it is running.
	runPidFilename = "run.pid"
	// RunLogFilename file that contains the logs of the execution.
	RunLogFilename = "run.log"
	// RunConfigFilename configuration of one run.
	RunConfigFilename = "run.json"
	// RunReportFilename result report generated after the test.
	RunReportFilename = "run_report.json"
	// RunPcapFilename optional traffic capture generated by the bngblaster itself.
	RunPcapFilename = "run.pcap"
	// RunSockFilename control socket.
	RunSockFilename = "run.sock"
	// RunStdErr redirected standard error output of the bngblaster.
	RunStdErr = "run.stderr"
	// RunStdOut redirected standard output of the bngblaster.
	RunStdOut = "run.stdout"
)

// make sure the DefaultRepository implements UseRepository.
var _ Repository = &DefaultRepository{}

// DefaultRepository is the default Repository implementation.
type DefaultRepository struct {
	executable   string
	configFolder string
}

// NewDefaultRepository is a constructor function for Repository.
func NewDefaultRepository(opts ...DefaultRepositoryOption) *DefaultRepository {
	r := &DefaultRepository{
		executable:   DefaultExecutable,
		configFolder: DefaultConfigFolder,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ConfigFolder implements Repository.
func (r DefaultRepository) ConfigFolder() string {
	return r.configFolder
}

// Create implements Repository.
func (r *DefaultRepository) Create(name string, config []byte) error {
	if r.Running(name) {
		return ErrBlasterRunning
	}
	folder := path.Join(r.configFolder, name)
	if err := os.MkdirAll(folder, permission); err != nil {
		return err
	}

	file := path.Join(folder, ConfigFilename)
	if err := os.WriteFile(file, config, permission); err != nil {
		return err
	}
	if err := r.cleanupRunFiles(name); err != nil {
		return err
	}
	return nil
}

// Delete implements Repository.
func (r *DefaultRepository) Delete(name string) error {
	if r.Running(name) {
		return ErrBlasterRunning
	}
	folder := path.Join(r.configFolder, name)
	_ = os.RemoveAll(folder)
	return nil
}

func (r *DefaultRepository) cleanupRunFiles(name string) error {
	folder := path.Join(r.configFolder, name)
	files := []string{
		path.Join(folder, runPidFilename),
		path.Join(folder, RunConfigFilename),
		path.Join(folder, RunReportFilename),
		path.Join(folder, RunPcapFilename),
		path.Join(folder, RunSockFilename),
		path.Join(folder, RunStdErr),
		path.Join(folder, RunStdOut),
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// Instances implements Repository.
func (r *DefaultRepository) Instances() []string {
	instances := []string{}
	entries, err := os.ReadDir(r.configFolder)
	if err != nil {
		return instances // Return the empty slice if there's an error.
	}
	for _, entry := range entries {
		if entry.IsDir() {
			instances = append(instances, entry.Name())
		}
	}
	return instances
}

// Exists implements Repository.
func (r *DefaultRepository) Exists(name string) bool {
	folder := path.Join(r.configFolder, name)
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return false
	}
	return true
}

// Running implements Repository.
func (r *DefaultRepository) Running(name string) bool {
	folder := path.Join(r.configFolder, name)
	file := path.Join(folder, runPidFilename)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	// Read in the pid file as a slice of bytes.
	if piddata, err := os.ReadFile(file); err == nil {
		// Convert the file contents to an integer.
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			// Look for the pid in the process list.
			if process, err := os.FindProcess(pid); err == nil {
				// Send the process a signal zero kill.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// We only get an error if the pid isn't running, or it's not ours.
					return true
				}
			}
		}
	}
	_ = os.Remove(file)
	return false
}

// Start implements Repository.
func (r *DefaultRepository) Start(name string, runningConfig RunningConfig) error {
	if !r.Exists(name) {
		return ErrBlasterNotExists
	}
	if r.Running(name) {
		return ErrBlasterRunning
	}
	if err := r.cleanupRunFiles(name); err != nil {
		return err
	}
	folder := path.Join(r.configFolder, name)
	file := path.Join(folder, RunConfigFilename)
	config, err := json.Marshal(runningConfig)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, config, permission); err != nil {
		return err
	}
	params := r.commandlineParameters(name, runningConfig)
	_, err = RunCommand(
		path.Join(folder, runPidFilename),
		path.Join(folder, RunStdOut),
		path.Join(folder, RunStdErr),
		params...)
	return err
}

// Stop implements Repository.
func (r *DefaultRepository) Stop(name string) {
	r.sendSignal(name, os.Interrupt)
}

// Kill implements Repository.
func (r *DefaultRepository) Kill(name string) {
	r.sendSignal(name, os.Kill)
}

func (r *DefaultRepository) sendSignal(name string, signal os.Signal) {
	folder := path.Join(r.configFolder, name)
	file := path.Join(folder, runPidFilename)
	// Read in the pid file as a slice of bytes.
	if piddata, err := os.ReadFile(file); err == nil {
		// Convert the file contents to an integer.
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			// Look for the pid in the process list.
			if process, err := os.FindProcess(pid); err == nil {
				// Send the process a signal.
				_ = process.Signal(signal)
				return
			}
		}
	}
}

func (r *DefaultRepository) commandlineParameters(name string, runningConfig RunningConfig) []string {
	folder := path.Join(r.configFolder, name)
	var params []string
	params = append(params, r.executable)
	params = append(params, "-C", path.Join(folder, ConfigFilename))
	params = append(params, "-S", path.Join(folder, RunSockFilename))
	if runningConfig.Report {
		params = append(params, "-J", path.Join(folder, RunReportFilename))
	}
	for _, flag := range runningConfig.ReportFlags {
		params = append(params, "-j", flag)
	}
	if runningConfig.Logging {
		params = append(params, "-L", path.Join(folder, RunLogFilename))
	}
	for _, flag := range runningConfig.LoggingFlags {
		params = append(params, "-l", flag)
	}
	if runningConfig.PCAPCapture {
		params = append(params, "-P", path.Join(folder, RunPcapFilename))
	}
	if runningConfig.SessionCount > 0 {
		// SessionCount has priority over the deprecated PPPoESessionCount
		params = append(params, "-c", fmt.Sprintf("%d", runningConfig.SessionCount))
	} else if runningConfig.PPPoESessionCount > 0 {
		params = append(params, "-c", fmt.Sprintf("%d", runningConfig.PPPoESessionCount))
	}
	if len(runningConfig.StreamConfig) > 0 {
		params = append(params, "-T", runningConfig.StreamConfig)
	}
	return params
}

func (r *DefaultRepository) config(name string) ([]byte, error) {
	folder := path.Join(r.configFolder, name)
	file := path.Join(folder, ConfigFilename)
	return os.ReadFile(file)
}

// Command implements Repository.
func (r *DefaultRepository) Command(name string, command SocketCommand) ([]byte, error) {
	if !r.Exists(name) {
		return nil, ErrBlasterNotExists
	}
	if !r.Running(name) {
		return nil, ErrBlasterNotRunning
	}
	folder := path.Join(r.configFolder, name)
	file := path.Join(folder, RunSockFilename)
	// Open Socket
	c, err := net.Dial("unix", file)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = c.Close()
	}()
	_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))

	// Send Command
	if err := json.NewEncoder(c).Encode(command); err != nil {
		return nil, err
	}

	// Receive Response
	received := make([]byte, 0, initialReceiveBufferLength)
	_ = c.SetReadDeadline(time.Now().Add(readTimeout))
	for {
		buf := make([]byte, bufferLength)
		count, err := c.Read(buf)
		received = append(received, buf[:count]...)
		if err != nil {
			if errors.Is(err, syscall.ECONNRESET) {
				continue
			}
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	return received, nil
}
