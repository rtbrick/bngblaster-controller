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

// RunningConfig start configuration for the bngblaster
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
}

// SocketCommand request for a sock command.
type SocketCommand struct {
	// Command
	Command string `json:"command"`
	// Arguments for the command
	Arguments map[string]interface{} `json:"arguments"`
}
