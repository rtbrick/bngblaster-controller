// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package main

import (
	"flag"
	"fmt"
	"strings"
)

// envPrefix starts the environment variable of every flag, see envName.
// BNGBLASTERCTRL_OPTS of the systemd unit does not collide: there is no
// flag "opts", and the unit expands it into command line flags anyway.
const envPrefix = "BNGBLASTERCTRL_"

// noEnv are the flags without an environment variable: -health turns the
// binary into a one-shot check, which a variable in a container's
// environment would do to the controller itself as well.
var noEnv = map[string]bool{"health": true}

// envName returns the environment variable that sets the flag name:
// BNGBLASTERCTRL_ followed by the name in upper case with dashes as
// underscores ("wait-interfaces" is BNGBLASTERCTRL_WAIT_INTERFACES).
func envName(name string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// applyEnv sets every flag of fs that is not given on the command line
// (and not in noEnv) from its environment variable, if that is set and not
// empty, so that a container can be configured through its environment. A
// flag on the command line wins. The value is parsed like the flag's (booleans accept
// strconv.ParseBool values); an invalid one is an error naming the
// variable. lookup is os.LookupEnv outside of tests.
func applyEnv(fs *flag.FlagSet, lookup func(string) (string, bool)) error {
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	var err error
	fs.VisitAll(func(f *flag.Flag) {
		if err != nil || explicit[f.Name] || noEnv[f.Name] {
			return
		}
		value, ok := lookup(envName(f.Name))
		if !ok || value == "" {
			return
		}
		if setErr := fs.Set(f.Name, value); setErr != nil {
			err = fmt.Errorf("invalid value %q for %s: %w", value, envName(f.Name), setErr)
		}
	})
	return err
}

// usage is the -h output: flag's default listing, plus the environment
// variable of each flag.
func usage(fs *flag.FlagSet) func() {
	return func() {
		out := fs.Output()
		_, _ = fmt.Fprintf(out, "Usage of %s:\n", fs.Name())
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(out, "\nEvery flag except -health can also be set with the environment variable\n"+
			"%s<FLAG> (upper case, dashes as underscores, e.g. %s);\n"+
			"a flag on the command line wins.\n",
			envPrefix, envName("wait-interfaces"))
	}
}
