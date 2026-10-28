// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Autostart states, see AutostartStatus.
const (
	// AutostartWaitingInterfaces: waiting for the data interfaces to appear.
	AutostartWaitingInterfaces = "waiting-interfaces"
	// AutostartStarting: the instance is being started and has not come up yet.
	AutostartStarting = "starting"
	// AutostartStarted: the instance has come up. Whether it is still
	// running, or how it ended, is the instance status.
	AutostartStarted = "started"
	// AutostartFailed: the instance could not be started; see the error.
	AutostartFailed = "failed"
)

const (
	// DefaultSysClassNet is where the network interfaces are listed.
	DefaultSysClassNet = "/sys/class/net"

	// interfacePollInterval is how often the interface wait looks again.
	interfacePollInterval = time.Second
	// stopPollInterval is how often StopInstance checks whether the
	// instance has exited.
	stopPollInterval = 100 * time.Millisecond
)

// ignoredInterfaces are never counted as data interfaces: loopback, the
// management interface of a container, and the fallback tunnel devices the
// kernel creates in every network namespace once the module of the tunnel
// type is loaded on the host (e.g. tunl0 with ipip), which would otherwise
// satisfy the wait before any data interface is attached.
var ignoredInterfaces = map[string]struct{}{
	"lo": {}, "eth0": {},
	"tunl0": {}, "sit0": {}, "ip6tnl0": {}, "gre0": {}, "gretap0": {}, "erspan0": {},
	"ip6gre0": {}, "ip_vti0": {}, "ip6_vti0": {},
}

// AutostartStatus is the state of the autostart of an instance.
type AutostartStatus struct {
	// Instance is the name of the instance to start.
	Instance string `json:"-"`
	// State is one of the Autostart* states.
	State string `json:"state"`
	// Error tells why the autostart failed.
	Error string `json:"error,omitempty"`
}

// AutostartOption configures an Autostart.
type AutostartOption func(*Autostart)

// WithWaitInterfaces makes the autostart wait until at least n data
// interfaces exist, for at most timeout (0 waits without limit).
func WithWaitInterfaces(n int, timeout time.Duration) AutostartOption {
	return func(a *Autostart) {
		a.waitInterfaces = n
		a.waitTimeout = timeout
	}
}

// Autostart starts one instance once, when the controller starts, e.g. to
// run a test at container start. It keeps its state in memory only: after
// a controller restart, a state found in a persistent instance folder
// would describe an earlier deployment.
type Autostart struct {
	repository     Repository
	instance       string
	waitInterfaces int
	waitTimeout    time.Duration
	// sysClassNet lists the network interfaces; replaced in tests.
	sysClassNet  string
	pollInterval time.Duration

	mu     sync.Mutex
	status AutostartStatus
}

// NewAutostart returns the autostart of instance. Nothing happens before
// Run is called.
func NewAutostart(repository Repository, instance string, opts ...AutostartOption) *Autostart {
	a := &Autostart{
		repository:   repository,
		instance:     instance,
		sysClassNet:  DefaultSysClassNet,
		pollInterval: interfacePollInterval,
		// Run begins with the interface wait (which ends at once without
		// interfaces to wait for), so that is also the state until then.
		status: AutostartStatus{Instance: instance, State: AutostartWaitingInterfaces},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Instance returns the name of the instance to start.
func (a *Autostart) Instance() string {
	return a.instance
}

// Status returns the current state of the autostart.
func (a *Autostart) Status() AutostartStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *Autostart) setState(state string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.State = state
	a.status.Error = ""
	if err != nil {
		a.status.Error = err.Error()
	}
}

// Run waits for the data interfaces and then starts the instance through
// Repository.Start, the code path of the _start API, with the arguments
// of the instance's run.json if there is one and defaults otherwise. It
// starts the instance once only and returns when the instance has come up
// or the start has failed. Cancelling ctx aborts the wait; an instance
// spawned already keeps running (see StopInstance).
func (a *Autostart) Run(ctx context.Context) {
	logger := log.With().Str("instance", a.instance).Logger()
	if a.waitInterfaces > 0 {
		logger.Info().Int("interfaces", a.waitInterfaces).Msg("autostart: waiting for interfaces")
		if err := a.waitForInterfaces(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.setState(AutostartFailed, err)
			logger.Error().Err(err).Msg("autostart failed")
			return
		}
	}
	a.setState(AutostartStarting, nil)
	runningConfig, err := a.runningConfig()
	if err == nil {
		logger.Info().Msg("autostart: starting instance")
		err = a.repository.Start(ctx, a.instance, runningConfig)
	}
	if errors.Is(err, ErrBlasterRunning) {
		// Started by an earlier controller process, which this one
		// cannot tell apart from an own start: the instance runs either way.
		logger.Warn().Msg("autostart: instance is running already")
		err = nil
	}
	if err != nil {
		a.setState(AutostartFailed, err)
		logger.Error().Err(err).Msg("autostart failed")
		return
	}
	a.setState(AutostartStarted, nil)
	logger.Info().Msg("autostart: instance started")
}

// runningConfig returns the start arguments of the instance: its run.json,
// if there is one, the defaults otherwise.
func (a *Autostart) runningConfig() (RunningConfig, error) {
	folder := path.Join(a.repository.ConfigFolder(), a.instance)
	if _, err := os.Stat(path.Join(folder, ConfigFilename)); err != nil {
		return RunningConfig{}, fmt.Errorf("no %s for instance %s: %w", ConfigFilename, a.instance, err)
	}
	runningConfig, err := ReadRunConfig(path.Join(folder, RunConfigFilename))
	if errors.Is(err, os.ErrNotExist) {
		return RunningConfig{}, nil
	}
	if err != nil {
		return RunningConfig{}, fmt.Errorf("invalid %s: %w", RunConfigFilename, err)
	}
	return runningConfig, nil
}

// waitForInterfaces polls until at least waitInterfaces data interfaces
// exist, see countInterfaces.
//
// Their presence is enough: bngblaster sets the links of its interfaces
// up itself, and whether a link has carrier depends on the other end of
// the test, which is no reason to delay the start.
func (a *Autostart) waitForInterfaces(ctx context.Context) error {
	if a.waitTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.waitTimeout)
		defer cancel()
	}
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()
	for {
		found, err := countInterfaces(a.sysClassNet)
		if err == nil && found >= a.waitInterfaces {
			log.Info().Str("instance", a.instance).Int("interfaces", found).Msg("autostart: interfaces found")
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("waiting for %d interfaces: %w", a.waitInterfaces, err)
			}
			return fmt.Errorf("timed out after %s waiting for %d interfaces, found %d",
				a.waitTimeout, a.waitInterfaces, found)
		case <-ticker.C:
		}
	}
}

// countInterfaces counts the network interfaces in dir (/sys/class/net)
// except ignoredInterfaces.
func countInterfaces(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, entry := range entries {
		if _, ok := ignoredInterfaces[entry.Name()]; !ok {
			n++
		}
	}
	return n, nil
}

// StopInstance stops the instance the way the _stop API does, if it is
// running, and waits until it has exited or ctx is done, so that it can
// write its report before the controller (and with it a container) exits.
// Call it only after Run has returned, or the instance could be started
// right after.
func (a *Autostart) StopInstance(ctx context.Context) {
	if !a.repository.Running(a.instance) {
		return
	}
	log.Info().Str("instance", a.instance).Msg("autostart: stopping instance")
	a.repository.Stop(a.instance)
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()
	for a.repository.Running(a.instance) {
		select {
		case <-ctx.Done():
			log.Warn().Str("instance", a.instance).Msg("autostart: instance did not stop in time")
			return
		case <-ticker.C:
		}
	}
}
