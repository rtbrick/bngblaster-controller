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

	// DefaultStopTimeout is how long StopInstance gives the instance for
	// its graceful teardown by default; large tests need longer, see
	// WithStopTimeout. Where the teardown cannot finish because the data
	// interfaces are gone (containerlab removes them before it stops the
	// container), StopInstance does not wait for it.
	DefaultStopTimeout = 30 * time.Second
	// TerminateTimeout is how long StopInstance waits after SIGTERM, for
	// bngblaster to write its report and exit, before it kills it.
	TerminateTimeout = 10 * time.Second
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

// WithStopTimeout sets how long StopInstance gives the instance for its
// graceful teardown (SIGINT) before it stops it at once (SIGTERM); 0 skips
// the graceful teardown. A teardown of many sessions can take minutes.
func WithStopTimeout(timeout time.Duration) AutostartOption {
	return func(a *Autostart) {
		a.stopTimeout = timeout
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
	// stopTimeout is the graceful teardown time of StopInstance, and
	// stopPoll how often it checks for the exit; the latter is replaced in
	// tests.
	stopTimeout time.Duration
	stopPoll    time.Duration
	// startInterfaces is the number of data interfaces when the instance
	// was started; StopInstance takes fewer for links that are gone.
	startInterfaces int

	// changed is called after each start and stop of the instance, see
	// OnLifecycleChange.
	changed func()

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
		stopTimeout:  DefaultStopTimeout,
		stopPoll:     stopPollInterval,
		// Run begins with the interface wait (which ends at once without
		// interfaces to wait for), so that is also the state until then.
		status: AutostartStatus{State: AutostartWaitingInterfaces},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// OnLifecycleChange registers fn to be called after the autostart has
// started or stopped the instance, like the server does on _start and
// _stop to drop the summaries it caches. Call it before Run.
func (a *Autostart) OnLifecycleChange(fn func()) {
	a.changed = fn
}

// lifecycleChanged calls the OnLifecycleChange function, if any.
func (a *Autostart) lifecycleChanged() {
	if a.changed != nil {
		a.changed()
	}
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
	// Without data interfaces (or /sys/class/net), StopInstance has none
	// to miss later.
	a.startInterfaces, _ = countInterfaces(a.sysClassNet)
	runningConfig, err := a.runningConfig()
	if err == nil {
		logger.Info().Msg("autostart: starting instance")
		err = a.repository.Start(ctx, a.instance, runningConfig)
		a.lifecycleChanged()
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
// except ignoredInterfaces. Interfaces are the symbolic links there; other
// entries, such as the bonding_masters file of the bonding module, are not.
func countInterfaces(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, entry := range entries {
		if _, ok := ignoredInterfaces[entry.Name()]; !ok && entry.Type()&os.ModeSymlink != 0 {
			n++
		}
	}
	return n, nil
}

// StopDeadline is how long StopInstance takes at most: the graceful
// teardown time plus TerminateTimeout. Give it a ctx with this deadline.
func (a *Autostart) StopDeadline() time.Duration {
	return a.stopTimeout + TerminateTimeout
}

// StopInstance stops the instance, if it is running, and waits until it has
// exited, so that it can write its report before the controller (and with
// it a container) exits. Call it only after Run has returned, or the
// instance could be started right after.
//
// It first asks for a graceful teardown (SIGINT, like the _stop API). If
// the instance is still running after the stop timeout - a teardown of
// many sessions takes long - it stops it at once with SIGTERM, with which
// bngblaster still writes its report. A teardown cannot finish once data
// interfaces the instance was started with are gone (containerlab removes
// the links before it stops the container), so then it sends SIGTERM right
// away. If the instance is still running when ctx is done, it kills it
// (SIGKILL).
func (a *Autostart) StopInstance(ctx context.Context) {
	if !a.repository.Running(a.instance) {
		return
	}
	logger := log.With().Str("instance", a.instance).Logger()
	terminated := false
	terminate := func() {
		if !terminated {
			terminated = true
			a.repository.Terminate(a.instance)
		}
	}
	logger.Info().Dur("stop_timeout", a.stopTimeout).Msg("autostart: stopping instance")
	if found, gone := a.interfacesGone(); gone {
		logger.Warn().Int("interfaces", found).Int("started_with", a.startInterfaces).
			Msg("autostart: data interfaces are gone, terminating instance")
		terminate()
	} else if a.stopTimeout > 0 {
		a.repository.Stop(a.instance)
	} else {
		terminate()
	}
	a.lifecycleChanged()
	graceful := time.NewTimer(a.stopTimeout)
	defer graceful.Stop()
	ticker := time.NewTicker(a.stopPoll)
	defer ticker.Stop()
	for a.repository.Running(a.instance) {
		select {
		case <-graceful.C:
			if !terminated {
				logger.Warn().Dur("stop_timeout", a.stopTimeout).Msg("autostart: instance did not stop in time, terminating it")
				terminate()
			}
		case <-ctx.Done():
			logger.Warn().Msg("autostart: instance did not terminate in time, killing it")
			a.repository.Kill(a.instance)
			return
		case <-ticker.C:
			if found, gone := a.interfacesGone(); gone && !terminated {
				logger.Warn().Int("interfaces", found).Int("started_with", a.startInterfaces).
					Msg("autostart: data interfaces are gone, terminating instance")
				terminate()
			}
		}
	}
}

// interfacesGone reports whether fewer data interfaces exist than the
// instance was started with, and how many there are.
func (a *Autostart) interfacesGone() (int, bool) {
	if a.startInterfaces == 0 {
		return 0, false
	}
	found, err := countInterfaces(a.sysClassNet)
	return found, err == nil && found < a.startInterfaces
}
