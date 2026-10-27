// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbe(t *testing.T) {
	tool := Tool{Name: LSPGen, ProbeArgs: []string{"--version"}}

	info := Probe(t.Context(), tool, fakeTool(t, `[ "$1" = --version ] && echo "Version: 0.9.1"`+"\n"))
	require.True(t, info.Available)
	require.Equal(t, "0.9.1", info.Version)
	require.Empty(t, info.Error)

	info = Probe(t.Context(), tool, fakeTool(t, "echo 'Failed to load scapy!'\nexit 1\n"))
	require.False(t, info.Available)
	require.True(t, strings.HasSuffix(info.Error, "Failed to load scapy!"), info.Error)
}
