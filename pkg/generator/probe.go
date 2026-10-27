// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

import (
	"context"
	"strings"
	"time"
)

// probeTimeout bounds a probe. bgpupdate and ldpupdate import scapy before
// they parse their arguments, which takes about a second.
const probeTimeout = 10 * time.Second

// maxProbeOutput bounds the output kept of a probe; only the version line
// or the last line of an error is used.
const maxProbeOutput = 4 << 10

// Info describes a configured tool and whether it can be used.
type Info struct {
	Tool

	Executable string `json:"executable"`
	Available  bool   `json:"available"`
	// Version is reported by lspgen only.
	Version string `json:"version,omitempty"`
	// Error tells why the tool is not available.
	Error string `json:"error,omitempty"`
}

// Probe checks whether tool can be run from executable. A cheap
// invocation (lspgen --version, the Python tools --help) has to succeed:
// besides a missing executable, this catches a missing Python interpreter
// or scapy, without which bgpupdate and ldpupdate exit right away.
func Probe(ctx context.Context, tool Tool, executable string) Info {
	info := Info{Tool: tool, Executable: executable}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	stdout := &tailBuffer{limit: maxProbeOutput}
	stderr := &tailBuffer{limit: maxProbeOutput}
	cmd := ExecCommand(ctx, executable, tool.ProbeArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay
	if err := cmd.Run(); err != nil {
		info.Error = err.Error()
		if msg := lastLine(stdout.String() + "\n" + stderr.String()); msg != "" {
			info.Error += ": " + msg
		}
		return info
	}
	info.Available = true
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if version, ok := strings.CutPrefix(strings.TrimSpace(line), "Version:"); ok {
			info.Version = strings.TrimSpace(version)
		}
	}
	return info
}

// lastLine returns the last non-empty line of output.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
