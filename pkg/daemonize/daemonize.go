// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package daemonize

import (
	"os"
	"os/signal"
	"syscall"
)

// Daemon function that is used to start.
type Daemon func() error

// Daemonize the function.
//
// It blocks until either start returns, in which case the returned signal is
// NormalTerminationSignal together with start's error, or a termination
// signal arrives, in which case that signal is returned with a nil error and
// start is left running so the caller can shut it down gracefully.
func Daemonize(start Daemon) (os.Signal, error) {
	// Handle common process-killing signals so we can gracefully shut down:
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(sigc)

	// The error travels over its own buffered channel rather than a shared
	// variable: on a signal, start keeps running and returns later, which
	// would otherwise race with the caller reading the error.
	errc := make(chan error, 1)
	go func() {
		errc <- start()
	}()

	select {
	case sig := <-sigc:
		return sig, nil
	case err := <-errc:
		return NormalTerminationSignal{}, err
	}
}

// NormalTerminationSignal signal implementation for normal program termination.
type NormalTerminationSignal struct{}

// String implements Signal interface.
func (t NormalTerminationSignal) String() string {
	return "Normal program termination."
}

// Signal implements signal interface.
func (t NormalTerminationSignal) Signal() {
	// This is only a marker function for the signal interface
}
