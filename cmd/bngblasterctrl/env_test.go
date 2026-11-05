// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package main

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEnvName(t *testing.T) {
	require.Equal(t, "BNGBLASTERCTRL_WAIT_INTERFACES_TIMEOUT", envName("wait-interfaces-timeout"))
	require.Equal(t, "BNGBLASTERCTRL_D", envName("d"))
	require.Equal(t, "BNGBLASTERCTRL_LOG_UI", envName("log-ui"))
}

func TestApplyEnv(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    map[string]string
		wantErr string
	}{
		{
			name: "defaults",
			want: map[string]string{"d": "/var/bngblaster", "ui": "true", "wait-interfaces": "0", "autostart": "", "stop-timeout": "30s"},
		}, {
			name: "env",
			env: map[string]string{
				"BNGBLASTERCTRL_D": "/tmp/x", "BNGBLASTERCTRL_UI": "false", "BNGBLASTERCTRL_AUTOSTART": "test",
				"BNGBLASTERCTRL_WAIT_INTERFACES": "2", "BNGBLASTERCTRL_WAIT_INTERFACES_TIMEOUT": "90s",
				"BNGBLASTERCTRL_STOP_TIMEOUT": "6m",
			},
			want: map[string]string{
				"d": "/tmp/x", "ui": "false", "autostart": "test",
				"wait-interfaces": "2", "wait-interfaces-timeout": "1m30s", "stop-timeout": "6m0s",
			},
		}, {
			name: "flag_wins",
			args: []string{"-d", "/flag", "-ui=true", "-wait-interfaces", "3", "-stop-timeout", "0"},
			env: map[string]string{
				"BNGBLASTERCTRL_D": "/env", "BNGBLASTERCTRL_UI": "false", "BNGBLASTERCTRL_WAIT_INTERFACES": "x",
				"BNGBLASTERCTRL_STOP_TIMEOUT": "1m",
			},
			want: map[string]string{"d": "/flag", "ui": "true", "wait-interfaces": "3", "stop-timeout": "0s"},
		}, {
			name: "booleans",
			env:  map[string]string{"BNGBLASTERCTRL_UI": "0", "BNGBLASTERCTRL_DEBUG": "TRUE", "BNGBLASTERCTRL_LOG_UI": "t"},
			want: map[string]string{"ui": "false", "debug": "true", "log-ui": "true"},
		}, {
			name: "empty_is_unset",
			env:  map[string]string{"BNGBLASTERCTRL_D": "", "BNGBLASTERCTRL_UI": ""},
			want: map[string]string{"d": "/var/bngblaster", "ui": "true"},
		}, {
			name:    "invalid_bool",
			env:     map[string]string{"BNGBLASTERCTRL_UI": "yes"},
			wantErr: `invalid value "yes" for BNGBLASTERCTRL_UI`,
		}, {
			name:    "invalid_stop_timeout",
			env:     map[string]string{"BNGBLASTERCTRL_STOP_TIMEOUT": "20"},
			wantErr: `invalid value "20" for BNGBLASTERCTRL_STOP_TIMEOUT`,
		}, {
			name:    "invalid_int",
			env:     map[string]string{"BNGBLASTERCTRL_WAIT_INTERFACES": "two"},
			wantErr: `invalid value "two" for BNGBLASTERCTRL_WAIT_INTERFACES`,
		}, {
			name:    "invalid_duration",
			env:     map[string]string{"BNGBLASTERCTRL_WAIT_INTERFACES_TIMEOUT": "5"},
			wantErr: `invalid value "5" for BNGBLASTERCTRL_WAIT_INTERFACES_TIMEOUT`,
		}, {
			// It would turn the controller of a container into a check.
			name:    "health_not_from_env",
			env:     map[string]string{"BNGBLASTERCTRL_HEALTH": "true", "BNGBLASTERCTRL_DEBUG": "maybe"},
			wantErr: `invalid value "maybe" for BNGBLASTERCTRL_DEBUG`,
		}, {
			name: "health_ignored",
			env:  map[string]string{"BNGBLASTERCTRL_HEALTH": "true"},
			want: map[string]string{"health": "false"},
		}, {
			// Only BNGBLASTERCTRL_<FLAG> is read; the packaging's variable
			// for command line flags is not one.
			name: "opts_ignored",
			env:  map[string]string{"BNGBLASTERCTRL_OPTS": "-ui=false"},
			want: map[string]string{"ui": "true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.String("d", "/var/bngblaster", "")
			fs.Bool("ui", true, "")
			fs.Bool("debug", false, "")
			fs.Bool("log-ui", false, "")
			fs.String("autostart", "", "")
			fs.Int("wait-interfaces", 0, "")
			fs.Duration("wait-interfaces-timeout", 5*time.Minute, "")
			fs.Duration("stop-timeout", 30*time.Second, "")
			fs.Bool("health", false, "")
			require.NoError(t, fs.Parse(tt.args))

			err := applyEnv(fs, func(key string) (string, bool) {
				value, ok := tt.env[key]
				return value, ok
			})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			for name, want := range tt.want {
				require.Equal(t, want, fs.Lookup(name).Value.String(), name)
			}
		})
	}
}

func TestContainerFlags_validate(t *testing.T) {
	flags := func(autostart string, stopTimeout time.Duration) containerFlags {
		return containerFlags{autostart: &autostart, stopTimeout: &stopTimeout}
	}
	require.NoError(t, flags("", 30*time.Second).validate())
	require.NoError(t, flags("test", 0).validate(), "0 stops at once with SIGTERM")
	require.ErrorContains(t, flags("test", -time.Second).validate(), "invalid -stop-timeout -1s")
	require.ErrorContains(t, flags("a.b", time.Second).validate(), "invalid -autostart instance")
}
