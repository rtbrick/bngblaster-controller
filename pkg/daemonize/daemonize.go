package daemonize

import (
	"os"
	"os/signal"
	"syscall"
)

// Daemon function that is used to start.
type Daemon func() error

// Daemonize the function.
func Daemonize(start Daemon) (os.Signal, error) {
	// Handle common process-killing signals so we can gracefully shut down:
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	var err error
	go func() {
		// Start a server; `err` will be returned to the caller:
		err = start()
		// Signal completion:
		sigc <- NormalTerminationSignal{}
		signal.Stop(sigc)
	}()

	// Wait for a termination signal (normal or otherwise):
	sig := <-sigc

	return sig, err
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
