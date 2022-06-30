package controller

//go:generate moq -out repositorymock.go . Repository

// Repository for managing the bng blaster.
type Repository interface {
	// ConfigFolder returns the config folder of this controller.
	ConfigFolder() string
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
	// Allowed values: debug|error|igmp|io|pppoe|info|pcap|timer|timer-detail|ip|loss|l2tp|dhcp|isis|bgp|tcp
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
	// Allowed values: session_counters|interfaces
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
		Name                     string `json:"name"`
		IfIndex                  int    `json:"ifindex"`
		Type                     string `json:"type"`
		TxPackets                int    `json:"tx-packets"`
		TxBytes                  int    `json:"tx-bytes"`
		TxPPS                    int    `json:"tx-pps"`
		TxKbps                   int    `json:"tx-kbps"`
		RxPackets                int    `json:"rx-packets"`
		RxBytes                  int    `json:"rx-bytes"`
		RxPPS                    int    `json:"rx-pps"`
		RxKbps                   int    `json:"rx-kbps"`
		TxPacketsMulticast       int    `json:"tx-packets-multicast"`
		TxPPSMulticast           int    `json:"tx-pps-multicast"`
		TxPacketsSessionIPv4     int    `json:"tx-packets-session-ipv4"`
		TxPPSSessionIPv4         int    `json:"tx-pps-session-ipv4"`
		RxPacketsSessionIPv4     int    `json:"rx-packets-session-ipv4"`
		RxPPSSessionIPv4         int    `json:"rx-pps-session-ipv4"`
		LossPacketsSessionIPv4   int    `json:"loss-packets-session-ipv4"`
		TxPacketsSessionIPv6     int    `json:"tx-packets-session-ipv6"`
		TxPPSSessionIPv6         int    `json:"tx-pps-session-ipv6"`
		RxPacketsSessionIPv6     int    `json:"rx-packets-session-ipv6"`
		RxPPSSessionIPv6         int    `json:"rx-pps-session-ipv6"`
		LossPacketsSessionIPv6   int    `json:"loss-packets-session-ipv6"`
		TxPacketsSessionIPv6PD   int    `json:"tx-packets-session-ipv6pd"`
		TxPPSSessionIPv6PD       int    `json:"tx-pps-session-ipv6pd"`
		RxPacketsSessionIPv6PD   int    `json:"rx-packets-session-ipv6pd"`
		RxPPSSessionIPv6PD       int    `json:"rx-pps-session-ipv6pd"`
		LossPacketsSessionIPv6PD int    `json:"loss-packets-session-ipv6pd"`
		TxPacketsStreams         int    `json:"tx-packets-streams"`
		TxPPSStreams             int    `json:"tx-pps-streams"`
		RxPacketsStreams         int    `json:"rx-packets-streams"`
		RxPPSStreams             int    `json:"rx-pps-streams"`
		LossPacketsStreams       int    `json:"loss-packets-streams"`
	} `json:"interfaces"`
}
