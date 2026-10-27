// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		request string
		want    Job
		wantErr string
	}{
		{
			name:    "bgpupdate with default file",
			tool:    BGPUpdate,
			request: `{"next-hop-base": "10.0.0.1", "prefix-base": "11.0.0.0/24", "prefix-num": 1000, "end-of-rib": true}`,
			want: Job{
				Tool:    BGPUpdate,
				Args:    []string{"--next-hop-base=10.0.0.1", "--prefix-base=11.0.0.0/24", "--prefix-num=1000", "--end-of-rib", "--file=.gen-out.bgp"},
				Command: []string{"--next-hop-base=10.0.0.1", "--prefix-base=11.0.0.0/24", "--prefix-num=1000", "--end-of-rib", "--file=out.bgp"},
				Outputs: []string{"out.bgp"},
			},
		},
		{
			name:    "repeated option, false flag and append",
			tool:    BGPUpdate,
			request: `{"asn": [65001, 65002], "next-hop-base": "10.0.0.1", "withdraw": false, "file": "u1.bgp", "append": true}`,
			want: Job{
				Tool:    BGPUpdate,
				Args:    []string{"--asn=65001", "--asn=65002", "--next-hop-base=10.0.0.1", "--file=.gen-u1.bgp", "--append"},
				Command: []string{"--asn=65001", "--asn=65002", "--next-hop-base=10.0.0.1", "--file=u1.bgp", "--append"},
				Outputs: []string{"u1.bgp"},
				Appends: []string{"u1.bgp"},
			},
		},
		{
			// protocol is rendered first whatever the request order, and
			// graphviz-file stands for two files.
			name:    "lspgen option order, joined list and suffixes",
			tool:    LSPGen,
			request: `{"mrt-file": "isis.mrt", "sr-algo": [0, 128], "protocol": "isis", "graphviz-file": "topo", "read-config-file": "topo.json"}`,
			want: Job{
				Tool:          LSPGen,
				Args:          []string{"--protocol=isis", "--read-config-file=topo.json", "--sr-algo=0,128", "--mrt-file=.gen-isis.mrt", "--graphviz-file=.gen-topo"},
				Command:       []string{"--protocol=isis", "--read-config-file=topo.json", "--sr-algo=0,128", "--mrt-file=isis.mrt", "--graphviz-file=topo"},
				Outputs:       []string{"isis.mrt", "topo.dot", "topo.svg"},
				Inputs:        []string{"topo.json"},
				SequenceCache: true,
			},
		},
		{
			// A value starting with a dash stays a value.
			name:    "dash value",
			tool:    LDPUpdate,
			request: `{"lsr-id": "--help"}`,
			want: Job{
				Tool:    LDPUpdate,
				Args:    []string{"--lsr-id=--help", "--file=.gen-out.ldp"},
				Command: []string{"--lsr-id=--help", "--file=out.ldp"},
				Outputs: []string{"out.ldp"},
			},
		},
		{name: "unknown option", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "control-socket": "run.sock"}`, wantErr: `unknown option "control-socket"`},
		{name: "required", tool: LDPUpdate, request: `{}`, wantErr: `option "lsr-id" is required`},
		{name: "not an object", tool: LDPUpdate, request: `[]`, wantErr: "must be a JSON object"},
		{name: "integer type", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "label-num": "5"}`, wantErr: "must be an integer"},
		{name: "integer fraction", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "label-num": 1.5}`, wantErr: "must be an integer"},
		{name: "range", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "label-base": 1048576}`, wantErr: "out of range"},
		{name: "enum", tool: LSPGen, request: `{"protocol": "rip"}`, wantErr: "must be one of"},
		{name: "control character", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1\n"}`, wantErr: "control characters"},
		{name: "empty string", tool: LDPUpdate, request: `{"lsr-id": ""}`, wantErr: "1 to 256 characters"},
		{name: "path", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "file": "../other/out.ldp"}`, wantErr: "without directory"},
		{name: "hidden", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "file": ".gen-out.ldp"}`, wantErr: "must not start with a dot"},
		{name: "reserved", tool: LDPUpdate, request: `{"lsr-id": "1.1.1.1", "file": "config.json"}`, wantErr: "reserved"},
		{name: "same output twice", tool: BGPUpdate, request: `{"next-hop-base": "1.1.1.1", "file": "a", "pcap": "a"}`, wantErr: "more than one option"},
		{name: "conflicting options", tool: LSPGen, request: `{"read-config-file": "a.json", "write-config-file": "b.json"}`, wantErr: "cannot be combined"},
		{name: "join comma", tool: LSPGen, request: `{"flex-algo": ["128:igp,129"]}`, wantErr: "must not contain a comma"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Build(tt.tool, []byte(tt.request))
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrInvalidRequest)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuild_unknownTool(t *testing.T) {
	_, err := Build("bngblaster", []byte(`{}`))
	require.ErrorIs(t, err, ErrUnknownTool)
}

// TestTools_specs guards the invariants Build relies on.
func TestTools_specs(t *testing.T) {
	for _, tool := range Tools {
		names := map[string]bool{}
		for _, option := range tool.Options {
			require.False(t, names[option.Name], "%s: duplicate option %s", tool.Name, option.Name)
			names[option.Name] = true
			if option.Kind == KindEnum {
				require.NotEmpty(t, option.Enum, "%s: %s", tool.Name, option.Name)
			}
		}
		for _, option := range tool.Options {
			if option.Appends != "" {
				require.True(t, names[option.Appends], "%s: %s appends to an unknown option", tool.Name, option.Name)
			}
		}
		require.NotContains(t, names, "control-socket", "socket mode is not a file generator")
	}
	require.Equal(t, "protocol", Tools[0].Options[0].Name, "lspgen needs the protocol first")
}
