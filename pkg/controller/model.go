// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package controller

//go:generate moq -out repositorymock.go . Repository

// Repository for managing the bng blaster.
type Repository interface {
	// ConfigFolder returns the config folder of this controller.
	ConfigFolder() string
	// AllowUpload returns true if file upload is allowed.
	AllowUpload() bool
	// Executable returns the bngblaster executable.
	Executable() string
	// Instances returns a list of all bngblaster instances.
	Instances() []string
	// Create a bngblaster instance on the file system.
	// Does not start an instance.
	Create(name string, config []byte) error
	// Delete a bngblaster instance
	// but this does not stop a running instance.
	Delete(name string) error
	// Exists checks if a bngblaster instance exists.
	Exists(name string) bool
	// Running checks if a bngblaster instance is running.
	Running(name string) bool
	// Start the bngblaster instance with the given running configuration.
	Start(name string, runningConfig RunningConfig) error
	// Stop sends a SIGINT to the instance
	Stop(name string)
	// Kill sends a SIGKILL to the instance
	Kill(name string)
	// Command sends a request to the unix socket.
	Command(name string, command SocketCommand) ([]byte, error)
}

// RunningConfig start configuration for the bngblaster.
type RunningConfig struct {
	// Report specifies that a report should be generated
	Report bool `json:"report"`
	// ReportFlags flags that allows to specify what is reported
	// Allowed values: sessions|streams
	ReportFlags []string `json:"report_flags"`
	// Logging specifies if logging is enabled
	Logging bool `json:"logging"`
	// LoggingFlags flags that allows to specify what is logged
	// Allowed values: debug|error|igmp|io|pppoe|info|pcap|ip|loss|l2tp|dhcp|isis|ospf|ldp|bgp|tcp|lag|dpdk|packet|http|timer|timer-detail
	LoggingFlags []string `json:"logging_flags"`
	// PCAPCapture allows to write a pcap file
	PCAPCapture bool `json:"pcap_capture"`
	// Deprecated: PPPoESessionCount specifies the PPPoE session count
	PPPoESessionCount int `json:"pppoe_session_count"`
	// SessionCount overwrites the session count from config
	SessionCount int `json:"session_count"`
	// StreamConfig specifies an optional stream configuration file (absolute path)
	StreamConfig string `json:"stream_config"`
	// MetricFlags flags that allows to specify instance metrics to be reported
	// Allowed values: session_counters|interfaces|access_interfaces|network_interfaces|a10nsp_interfaces|streams
	MetricFlags []string `json:"metric_flags"`
}

// SocketCommand request for a socket command.
type SocketCommand struct {
	// Command
	Command string `json:"command"`
	// Arguments for the command
	Arguments map[string]interface{} `json:"arguments"`
}

// SessionCountersResponse response for session-counters socket command.
type SessionCountersResponse struct {
	Code            int `json:"code"`
	SessionCounters struct {
		Sessions                     int     `json:"sessions"`
		SessionsPPPoE                int     `json:"sessions-pppoe"`
		SessionsIPoE                 int     `json:"sessions-ipoe"`
		SessionsEstablished          int     `json:"sessions-established"`
		SessionsEstablishedMax       int     `json:"sessions-established-max"`
		SessionsTerminated           int     `json:"sessions-terminated"`
		SessionsFlapped              int     `json:"sessions-flapped"`
		DHCPSessions                 int     `json:"dhcp-sessions"`
		DHCPSessionsEstablished      int     `json:"dhcp-sessions-established"`
		DHCPSessionsEstablishedMax   int     `json:"dhcp-sessions-established-max"`
		DHCPv6Sessions               int     `json:"dhcpv6-sessions"`
		DHCPv6SessionsEstablished    int     `json:"dhcpv6-sessions-established"`
		DHCPv6SessionsEstablishedMax int     `json:"dhcpv6-sessions-established-max"`
		SetupTime                    int     `json:"setup-time"`
		SetupRate                    float64 `json:"setup-rate"`
		SetupRateMin                 float64 `json:"setup-rate-min"`
		SetupRateAvg                 float64 `json:"setup-rate-avg"`
		SetupRateMax                 float64 `json:"setup-rate-max"`
		SessionTrafficFlows          int     `json:"session-traffic-flows"`
		SessionTrafficFlowsVerified  int     `json:"session-traffic-flows-verified"`
		StreamTrafficFlows           int     `json:"stream-traffic-flows"`
		StreamTrafficFlowsVerified   int     `json:"stream-traffic-flows-verified"`
	} `json:"session-counters"`
}

// InterfacesResponse response for interfaces socket command.
type InterfacesResponse struct {
	Code       int `json:"code"`
	Interfaces []struct {
		Name      string `json:"name"`
		IfIndex   int    `json:"ifindex"`
		Type      string `json:"type"`
		TxPackets int    `json:"tx-packets"`
		TxBytes   int    `json:"tx-bytes"`
		RxPackets int    `json:"rx-packets"`
		RxBytes   int    `json:"rx-bytes"`
	} `json:"interfaces"`
}

// AccessInterfacesResponse response for access-interfaces socket command.
type AccessInterfacesResponse struct {
	Code       int `json:"code"`
	Interfaces []struct {
		Name                       string `json:"name"`
		IfIndex                    int    `json:"ifindex"`
		Type                       string `json:"type"`
		TxPackets                  int    `json:"tx-packets"`
		TxBytes                    int    `json:"tx-bytes"`
		TxPPS                      int    `json:"tx-pps"`
		TxKbps                     int    `json:"tx-kbps"`
		RxPackets                  int    `json:"rx-packets"`
		RxBytes                    int    `json:"rx-bytes"`
		RxPPS                      int    `json:"rx-pps"`
		RxKbps                     int    `json:"rx-kbps"`
		RxPacketsMulticast         int    `json:"rx-packets-multicast"`
		RxPPSMulticast             int    `json:"rx-pps-multicast"`
		RxLossPacketsMulticast     int    `json:"rx-loss-packets-multicast"`
		TxPacketsSessionIPv4       int    `json:"tx-packets-session-ipv4"`
		TxPPSSessionIPv4           int    `json:"tx-pps-session-ipv4"`
		RxPacketsSessionIPv4       int    `json:"rx-packets-session-ipv4"`
		RxPPSSessionIPv4           int    `json:"rx-pps-session-ipv4"`
		RxLossPacketsSessionIPv4   int    `json:"rx-loss-packets-session-ipv4"`
		TxPacketsSessionIPv6       int    `json:"tx-packets-session-ipv6"`
		TxPPSSessionIPv6           int    `json:"tx-pps-session-ipv6"`
		RxPacketsSessionIPv6       int    `json:"rx-packets-session-ipv6"`
		RxPPSSessionIPv6           int    `json:"rx-pps-session-ipv6"`
		RxLossPacketsSessionIPv6   int    `json:"rx-loss-packets-session-ipv6"`
		TxPacketsSessionIPv6PD     int    `json:"tx-packets-session-ipv6pd"`
		TxPPSSessionIPv6PD         int    `json:"tx-pps-session-ipv6pd"`
		RxPacketsSessionIPv6PD     int    `json:"rx-packets-session-ipv6pd"`
		RxPPSSessionIPv6PD         int    `json:"rx-pps-session-ipv6pd"`
		RxLossPacketsSessionIPv6PD int    `json:"rx-loss-packets-session-ipv6pd"`
		TxPacketsStreams           int    `json:"tx-packets-streams"`
		TxPPSStreams               int    `json:"tx-pps-streams"`
		RxPacketsStreams           int    `json:"rx-packets-streams"`
		RxPPSStreams               int    `json:"rx-pps-streams"`
		RxLossPacketsStreams       int    `json:"rx-loss-packets-streams"`
	} `json:"access-interfaces"`
}

// NetworkInterfacesResponse response for network-interfaces socket command.
type NetworkInterfacesResponse struct {
	Code       int `json:"code"`
	Interfaces []struct {
		Name                       string `json:"name"`
		IfIndex                    int    `json:"ifindex"`
		Type                       string `json:"type"`
		TxPackets                  int    `json:"tx-packets"`
		TxBytes                    int    `json:"tx-bytes"`
		TxPPS                      int    `json:"tx-pps"`
		TxKbps                     int    `json:"tx-kbps"`
		RxPackets                  int    `json:"rx-packets"`
		RxBytes                    int    `json:"rx-bytes"`
		RxPPS                      int    `json:"rx-pps"`
		RxKbps                     int    `json:"rx-kbps"`
		TxPacketsMulticast         int    `json:"rx-packets-multicast"`
		TxPPSMulticast             int    `json:"rx-pps-multicast"`
		TxPacketsSessionIPv4       int    `json:"tx-packets-session-ipv4"`
		TxPPSSessionIPv4           int    `json:"tx-pps-session-ipv4"`
		RxPacketsSessionIPv4       int    `json:"rx-packets-session-ipv4"`
		RxPPSSessionIPv4           int    `json:"rx-pps-session-ipv4"`
		RxLossPacketsSessionIPv4   int    `json:"rx-loss-packets-session-ipv4"`
		TxPacketsSessionIPv6       int    `json:"tx-packets-session-ipv6"`
		TxPPSSessionIPv6           int    `json:"tx-pps-session-ipv6"`
		RxPacketsSessionIPv6       int    `json:"rx-packets-session-ipv6"`
		RxPPSSessionIPv6           int    `json:"rx-pps-session-ipv6"`
		RxLossPacketsSessionIPv6   int    `json:"rx-loss-packets-session-ipv6"`
		TxPacketsSessionIPv6PD     int    `json:"tx-packets-session-ipv6pd"`
		TxPPSSessionIPv6PD         int    `json:"tx-pps-session-ipv6pd"`
		RxPacketsSessionIPv6PD     int    `json:"rx-packets-session-ipv6pd"`
		RxPPSSessionIPv6PD         int    `json:"rx-pps-session-ipv6pd"`
		RxLossPacketsSessionIPv6PD int    `json:"rx-loss-packets-session-ipv6pd"`
		TxPacketsStreams           int    `json:"tx-packets-streams"`
		TxPPSStreams               int    `json:"tx-pps-streams"`
		RxPacketsStreams           int    `json:"rx-packets-streams"`
		RxPPSStreams               int    `json:"rx-pps-streams"`
		RxLossPacketsStreams       int    `json:"rx-loss-packets-streams"`
	} `json:"network-interfaces"`
}

// A10nspInterfacesResponse response for a10nsp-interfaces socket command.
type A10nspInterfacesResponse struct {
	Code       int `json:"code"`
	Interfaces []struct {
		Name                       string `json:"name"`
		IfIndex                    int    `json:"ifindex"`
		Type                       string `json:"type"`
		TxPackets                  int    `json:"tx-packets"`
		TxBytes                    int    `json:"tx-bytes"`
		TxPPS                      int    `json:"tx-pps"`
		TxKbps                     int    `json:"tx-kbps"`
		RxPackets                  int    `json:"rx-packets"`
		RxBytes                    int    `json:"rx-bytes"`
		RxPPS                      int    `json:"rx-pps"`
		RxKbps                     int    `json:"rx-kbps"`
		TxPacketsSessionIPv4       int    `json:"tx-packets-session-ipv4"`
		TxPPSSessionIPv4           int    `json:"tx-pps-session-ipv4"`
		RxPacketsSessionIPv4       int    `json:"rx-packets-session-ipv4"`
		RxPPSSessionIPv4           int    `json:"rx-pps-session-ipv4"`
		RxLossPacketsSessionIPv4   int    `json:"rx-loss-packets-session-ipv4"`
		TxPacketsSessionIPv6       int    `json:"tx-packets-session-ipv6"`
		TxPPSSessionIPv6           int    `json:"tx-pps-session-ipv6"`
		RxPacketsSessionIPv6       int    `json:"rx-packets-session-ipv6"`
		RxPPSSessionIPv6           int    `json:"rx-pps-session-ipv6"`
		RxLossPacketsSessionIPv6   int    `json:"rx-loss-packets-session-ipv6"`
		TxPacketsSessionIPv6PD     int    `json:"tx-packets-session-ipv6pd"`
		TxPPSSessionIPv6PD         int    `json:"tx-pps-session-ipv6pd"`
		RxPacketsSessionIPv6PD     int    `json:"rx-packets-session-ipv6pd"`
		RxPPSSessionIPv6PD         int    `json:"rx-pps-session-ipv6pd"`
		RxLossPacketsSessionIPv6PD int    `json:"rx-loss-packets-session-ipv6pd"`
		TxPacketsStreams           int    `json:"tx-packets-streams"`
		TxPPSStreams               int    `json:"tx-pps-streams"`
		RxPacketsStreams           int    `json:"rx-packets-streams"`
		RxPPSStreams               int    `json:"rx-pps-streams"`
		RxLossPacketsStreams       int    `json:"rx-loss-packets-streams"`
	} `json:"a10nsp-interfaces"`
}

// StreamSummaryResponse response for stream-summary socket command.
type StreamSummaryResponse struct {
	Code    int `json:"code"`
	Streams []struct {
		FlowId    int    `json:"flow-id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		SubType   string `json:"sub-type"`
		Direction string `json:"direction"`
		TxPackets int    `json:"tx-packets"`
		TxBytes   int    `json:"tx-bytes"`
		RxPackets int    `json:"rx-packets"`
		RxBytes   int    `json:"rx-bytes"`
		RxLoss    int    `json:"rx-loss"`
		SessionId int    `json:"session-id"`
	} `json:"stream-summary"`
}
