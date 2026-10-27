// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package generator

// Tool names.
const (
	LSPGen    = "lspgen"
	BGPUpdate = "bgpupdate"
	LDPUpdate = "ldpupdate"
)

// Upper bounds for the counts that drive how much a tool generates. They
// are far above anything a test needs; the run timeout is the actual bound
// on the work, these only reject obvious mistakes early.
const (
	maxNodeCount   = 1_000_000
	maxPrefixCount = 100_000_000
	maxLabel       = 1_048_575
)

// bound returns a pointer to v, for Option.Min and Option.Max.
func bound(v int64) *int64 { return new(v) }

// logLevels of bgpupdate and ldpupdate, and their default.
var logLevels = []string{"warning", defaultLogLevel, "debug"}

const defaultLogLevel = "info"

// Tools lists the supported generator tools and their options. The
// options mirror the tools' long options one to one, except for lspgen's
// control socket mode (--control-socket, --control-instance, --quit-loop),
// which injects into a running instance instead of writing files.
// The bounds and defaults are the tools' own, and the group names only
// structure the list, so neither is worth a named constant.
//
//nolint:mnd,goconst // see above
var Tools = []Tool{
	{
		Name:              LSPGen,
		Description:       "ISIS and OSPF topology generator (MRT, PCAP, stream and topology files)",
		DefaultExecutable: "/usr/bin/lspgen",
		ProbeArgs:         []string{"--version"},
		SequenceCache:     true,
		Options: []Option{
			// lspgen processes its arguments in order and protocol resets
			// the protocol dependent defaults, so it has to stay first.
			{Name: "protocol", Kind: KindEnum, Enum: []string{"isis", "ospf2", "ospf3"}, Default: "isis", Group: "topology", Help: "protocol"},
			{Name: "area", Kind: KindString, Repeat: true, Group: "topology", Help: "area (ISIS up to 3, e.g. 49.0001/24; OSPF area ID)"},
			{Name: "level", Kind: KindInt, Min: bound(1), Max: bound(2), Group: "topology", Help: "ISIS level"},
			{Name: "connector", Kind: KindString, Repeat: true, Group: "topology", Help: "link to the BNG Blaster (ISIS system-id; OSPF remote-router-id:local-address)"},
			{Name: "node-count", Kind: KindInt, Min: bound(1), Max: bound(maxNodeCount), Default: 10, Group: "topology", Help: "node count"},
			{Name: "seed", Kind: KindInt, Group: "topology", Help: "seed for a reproducible random topology"},
			{Name: "link-multiplier", Kind: KindInt, Min: bound(1), Max: bound(150), Default: 1, Group: "topology", Help: "parallel links"},
			{Name: "external-count", Kind: KindInt, Min: bound(0), Max: bound(maxPrefixCount), Default: 0, Group: "topology", Help: "external prefix count"},
			{Name: "read-config-file", Kind: KindInput, Conflicts: "write-config-file", Group: "topology", Help: "read the topology from this file of the instance instead of generating it"},
			{Name: "sequence", Kind: KindString, Group: "lsp", Help: "sequence number (ISIS 0x1, OSPF 0x80000001)"},
			{Name: "lsp-lifetime", Kind: KindInt, Min: bound(120), Max: bound(65535), Group: "lsp", Help: "LSP lifetime in seconds (ISIS 65535, OSPF 3600)"},
			{Name: "authentication-type", Kind: KindEnum, Enum: []string{"none", "simple", "md5"}, Group: "lsp", Help: "authentication type"},
			{Name: "authentication-key", Kind: KindString, Group: "lsp", Help: "authentication key"},
			{Name: "purge", Kind: KindBool, Group: "lsp", Help: "generate all link state packets as purges"},
			{Name: "ipv4-node-prefix", Kind: KindString, Default: "192.168.0.0/32", Group: "addressing", Help: "IPv4 node prefix base"},
			{Name: "ipv4-link-prefix", Kind: KindString, Default: "172.16.0.0/31", Group: "addressing", Help: "IPv4 link prefix base"},
			{Name: "ipv4-external-prefix", Kind: KindString, Default: "10.0.0.0/28", Group: "addressing", Help: "IPv4 external prefix base"},
			{Name: "ipv6-node-prefix", Kind: KindString, Default: "fc00::c0a8:0/128", Group: "addressing", Help: "IPv6 node prefix base"},
			{Name: "ipv6-link-prefix", Kind: KindString, Default: "fc00::ac10:0/127", Group: "addressing", Help: "IPv6 link prefix base"},
			{Name: "ipv6-external-prefix", Kind: KindString, Default: "fc00::a00:0/124", Group: "addressing", Help: "IPv6 external prefix base"},
			{Name: "no-ipv4", Kind: KindBool, Group: "addressing", Help: "no IPv4"},
			{Name: "no-ipv6", Kind: KindBool, Group: "addressing", Help: "no IPv6"},
			{Name: "no-sr", Kind: KindBool, Group: "segment-routing", Help: "no segment routing node SIDs"},
			{Name: "sr-algo", Kind: KindInt, Join: true, Min: bound(0), Max: bound(255), Group: "segment-routing", Help: "SR algorithms (0, 1, 128-255; up to 4)"},
			{Name: "flex-algo", Kind: KindString, Join: true, Group: "segment-routing", Help: "Flex-Algo definitions <algo>:igp|min-delay|te[:priority]"},
			{Name: "link-delay", Kind: KindString, Group: "segment-routing", Help: "link delay in microseconds <min>[:<max>]"},
			{Name: "te-metric", Kind: KindString, Group: "segment-routing", Help: "TE metric <min>[:<max>]"},
			{Name: "node-msd", Kind: KindInt, Min: bound(1), Max: bound(255), Group: "segment-routing", Help: "node maximum SID depth (BMI-MSD)"},
			{Name: "elc", Kind: KindBool, Group: "segment-routing", Help: "entropy label capability (E-flag)"},
			{Name: "mrt-file", Kind: KindOutput, Group: "output", Help: "MRT file for the isis/ospf external mrt-file"},
			{Name: "pcap-file", Kind: KindOutput, Group: "output", Help: "PCAP file with all link state packets"},
			{Name: "stream-file", Kind: KindOutput, Group: "output", Help: "JSON file with all prefixes as stream destinations"},
			{Name: "write-config-file", Kind: KindOutput, Group: "output", Help: "topology configuration file (lspgen uses only the last of read-config-file and write-config-file)"},
			{Name: "graphviz-file", Kind: KindOutput, Suffixes: []string{".dot", ".svg"}, Group: "output", Help: "graphviz base name (<name>.dot, <name>.svg if dot is installed)"},
			{Name: "log", Kind: KindEnum, Repeat: true, Enum: []string{"normal", "debug", "lsp", "lsdb", "packet", "ctrl", "error"}, Group: "output", Help: "log categories"},
		},
	},
	{
		Name:              BGPUpdate,
		Description:       "BGP RAW update file generator (IPv4/IPv6, labeled, EVPN, L2VPN)",
		DefaultExecutable: "/usr/bin/bgpupdate",
		ProbeArgs:         []string{"--help"},
		Options: []Option{
			{Name: "asn", Kind: KindInt, Repeat: true, Min: bound(0), Max: bound(4294967295), Group: "update", Help: "AS path"},
			{Name: "next-hop-base", Kind: KindString, Required: true, Group: "update", Help: "next-hop base address (IPv4 or IPv6)"},
			{Name: "next-hop-num", Kind: KindInt, Min: bound(1), Max: bound(maxPrefixCount), Default: 1, Group: "update", Help: "next-hop count"},
			{Name: "prefix-base", Kind: KindString, Group: "update", Help: "prefix base network (IPv4 or IPv6)"},
			{Name: "prefix-num", Kind: KindInt, Min: bound(1), Max: bound(maxPrefixCount), Default: 1, Group: "update", Help: "prefix count"},
			{Name: "label-base", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Group: "update", Help: "label base (labeled unicast)"},
			{Name: "label-num", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Default: 1, Group: "update", Help: "label count"},
			{Name: "local-pref", Kind: KindInt, Min: bound(0), Max: bound(4294967295), Group: "update", Help: "local preference"},
			{Name: "med", Kind: KindInt, Min: bound(0), Max: bound(4294967295), Group: "update", Help: "MED"},
			{Name: "withdraw", Kind: KindBool, Group: "update", Help: "withdraw prefixes"},
			{Name: "end-of-rib", Kind: KindBool, Group: "update", Help: "add end-of-rib message"},
			{Name: "file", Kind: KindOutput, Default: "out.bgp", Group: "output", Help: "BGP RAW update file"},
			{Name: "append", Kind: KindBool, Appends: "file", Group: "output", Help: "append to the update file if it exists"},
			{Name: "pcap", Kind: KindOutput, Group: "output", Help: "PCAP file with the BGP updates"},
			{Name: "log-level", Kind: KindEnum, Enum: logLevels, Default: defaultLogLevel, Group: "output", Help: "log level"},
			{Name: "streams", Kind: KindOutput, Group: "streams", Help: "BNG Blaster traffic stream file"},
			{Name: "stream-append", Kind: KindBool, Appends: "streams", Group: "streams", Help: "append to the stream file if it exists"},
			{Name: "stream-direction", Kind: KindEnum, Enum: []string{"upstream", "downstream", "both"}, Default: "downstream", Group: "streams", Help: "stream direction"},
			{Name: "stream-pps", Kind: KindNumber, Min: bound(0), Default: 1.0, Group: "streams", Help: "stream packets per second"},
			{Name: "stream-interface", Kind: KindString, Group: "streams", Help: "stream interface"},
			{Name: "stream-group-id", Kind: KindInt, Min: bound(0), Max: bound(65535), Group: "streams", Help: "stream group identifier"},
			{Name: "stream-tx-label", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Group: "streams", Help: "stream TX outer label"},
			{Name: "stream-tx-inner-label", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Group: "streams", Help: "stream TX inner label"},
			{Name: "stream-rx-label", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Group: "streams", Help: "stream RX label"},
			{Name: "stream-rx-label-num", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Default: 1, Group: "streams", Help: "stream RX label count"},
			{Name: "evpn-vpws", Kind: KindBool, Group: "evpn-vpws", Help: "generate EVPN VPWS per-EVI Ethernet A-D routes (route type 1)"},
			{Name: "rd", Kind: KindString, Group: "evpn-vpws", Help: "route distinguisher (ASN:N, ASNL:N or IPv4:N)"},
			{Name: "rd-increment", Kind: KindBool, Group: "evpn-vpws", Help: "increment route distinguisher per service"},
			{Name: "rt", Kind: KindString, Repeat: true, Group: "evpn-vpws", Help: "route target (ASN:N, ASNL:N or IPv4:N)"},
			{Name: "rt-increment", Kind: KindBool, Group: "evpn-vpws", Help: "increment route targets per service"},
			{Name: "esi", Kind: KindString, Group: "evpn-vpws", Help: "ethernet segment identifier (default 0)"},
			{Name: "service-id", Kind: KindInt, Min: bound(0), Max: bound(0xfffffffe), Default: 1, Group: "evpn-vpws", Help: "local VPWS service identifier base (ethernet tag)"},
			{Name: "service-num", Kind: KindInt, Min: bound(1), Max: bound(maxPrefixCount), Default: 1, Group: "evpn-vpws", Help: "service count"},
			{Name: "mtu", Kind: KindInt, Min: bound(0), Max: bound(0xffff), Default: 0, Group: "evpn-vpws", Help: "L2 MTU"},
			{Name: "control-word", Kind: KindBool, Group: "evpn-vpws", Help: "request control word (C flag)"},
			{Name: "primary", Kind: KindBool, Group: "evpn-vpws", Help: "set primary flag (P flag)"},
			{Name: "backup", Kind: KindBool, Group: "evpn-vpws", Help: "set backup flag (B flag)"},
			{Name: "encap-mpls", Kind: KindBool, Group: "evpn-vpws", Help: "add MPLS encapsulation extended community"},
			{Name: "stream-ce-interface", Kind: KindString, Group: "evpn-vpws", Help: "stream CE interface (CE to PE streams)"},
			{Name: "stream-ce-vlan", Kind: KindInt, Min: bound(1), Max: bound(4094), Group: "evpn-vpws", Help: "stream CE interface VLAN base (incremented per service)"},
			{Name: "stream-vpws-vlan", Kind: KindInt, Min: bound(1), Max: bound(4094), Group: "evpn-vpws", Help: "customer VLAN within VPWS base (PE to CE streams)"},
			{Name: "stream-vpws-inner-vlan", Kind: KindInt, Min: bound(1), Max: bound(4094), Group: "evpn-vpws", Help: "customer inner VLAN within VPWS (PE to CE streams)"},
			{Name: "stream-vpws-qinq", Kind: KindBool, Group: "evpn-vpws", Help: "customer outer VLAN ethertype 0x88a8"},
			{Name: "stream-vpws-arp", Kind: KindBool, Group: "evpn-vpws", Help: "reply to ARP, ND and ICMP echo and learn CE MAC"},
			{Name: "stream-evpn-rd", Kind: KindString, Group: "evpn-vpws", Help: "route distinguisher of DUT routes"},
			{Name: "stream-evpn-service-id", Kind: KindInt, Min: bound(0), Max: bound(0xfffffffe), Group: "evpn-vpws", Help: "DUT local VPWS service identifier base"},
			{Name: "stream-destination-mac", Kind: KindString, Group: "evpn-vpws", Help: "CE interface MAC address"},
			{Name: "stream-pe-address", Kind: KindString, Default: "192.0.2.1", Group: "evpn-vpws", Help: "PE side stream IPv4 address"},
			{Name: "stream-ce-address", Kind: KindString, Default: "192.0.2.2", Group: "evpn-vpws", Help: "CE side stream IPv4 address"},
			{Name: "evpn-elan", Kind: KindBool, Group: "evpn-elan", Help: "generate EVPN E-LAN IMET (type 3) and MAC/IP routes (type 2)"},
			{Name: "ethernet-tag", Kind: KindInt, Min: bound(0), Max: bound(0xfffffffe), Default: 0, Group: "evpn-elan", Help: "ethernet tag"},
			{Name: "mac", Kind: KindString, Repeat: true, Group: "evpn-elan", Help: "MAC address advertised per EVI"},
			{Name: "l2vpn", Kind: KindBool, Group: "l2vpn", Help: "generate BGP L2VPN label blocks (RFC 6624)"},
			{Name: "ce-id", Kind: KindInt, Min: bound(0), Max: bound(0xffff), Default: 1, Group: "l2vpn", Help: "local CE ID"},
			{Name: "label-block-offset", Kind: KindInt, Min: bound(0), Max: bound(0xffff), Default: 1, Group: "l2vpn", Help: "label block offset (first remote CE ID)"},
			{Name: "label-block-size", Kind: KindInt, Min: bound(1), Max: bound(0xffff), Default: 8, Group: "l2vpn", Help: "label block size"},
			{Name: "encapsulation", Kind: KindEnum, Enum: []string{"ethernet-vlan", "ethernet"}, Default: "ethernet-vlan", Group: "l2vpn", Help: "encapsulation type"},
			{Name: "stream-l2vpn-rd", Kind: KindString, Group: "l2vpn", Help: "route distinguisher of DUT routes"},
			{Name: "stream-l2vpn-ce-id", Kind: KindInt, Min: bound(0), Max: bound(0xffff), Group: "l2vpn", Help: "DUT CE ID"},
		},
	},
	{
		Name:              LDPUpdate,
		Description:       "LDP RAW update file generator",
		DefaultExecutable: "/usr/bin/ldpupdate",
		ProbeArgs:         []string{"--help"},
		Options: []Option{
			{Name: "lsr-id", Kind: KindString, Required: true, Group: "update", Help: "LSR identifier"},
			{Name: "message-id-base", Kind: KindInt, Min: bound(0), Max: bound(4294967295), Default: 1000, Group: "update", Help: "message identifier base"},
			{Name: "withdraw", Kind: KindBool, Group: "update", Help: "withdraw"},
			{Name: "address-base", Kind: KindString, Group: "update", Help: "address message base"},
			{Name: "address-num", Kind: KindInt, Min: bound(0), Max: bound(maxPrefixCount), Default: 0, Group: "update", Help: "address message count"},
			{Name: "prefix-base", Kind: KindString, Group: "update", Help: "label mapping base prefix"},
			{Name: "prefix-num", Kind: KindInt, Min: bound(0), Max: bound(maxPrefixCount), Default: 0, Group: "update", Help: "label mapping prefix count"},
			{Name: "label-base", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Default: 10000, Group: "update", Help: "label base"},
			{Name: "label-num", Kind: KindInt, Min: bound(1), Max: bound(maxLabel), Default: 1, Group: "update", Help: "label count"},
			{Name: "file", Kind: KindOutput, Default: "out.ldp", Group: "output", Help: "LDP RAW update file"},
			{Name: "append", Kind: KindBool, Appends: "file", Group: "output", Help: "append to the update file if it exists"},
			{Name: "pcap", Kind: KindOutput, Group: "output", Help: "PCAP file with the LDP updates"},
			{Name: "log-level", Kind: KindEnum, Enum: logLevels, Default: defaultLogLevel, Group: "output", Help: "log level"},
		},
	},
}
