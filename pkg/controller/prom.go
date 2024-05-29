package controller

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

const (
	metricInstancesTotal   = "instances_total"
	metricInstancesRunning = "instances_running"

	metricSessions                     = "sessions"
	metricSessionsPPPoE                = "sessions_pppoe"
	metricSessionsIPoE                 = "sessions_ipoe"
	metricSessionsEstablished          = "sessions_established"
	metricSessionsEstablishedMax       = "sessions_established_max"
	metricSessionsTerminated           = "sessions_terminated"
	metricSessionsFlapped              = "sessions_flapped"
	metricDHCPSessions                 = "dhcp_sessions"
	metricDHCPSessionsEstablished      = "dhcp_sessions_established"
	metricDHCPSessionsEstablishedMax   = "dhcp_sessions_established_max"
	metricDHCPv6Sessions               = "dhcpv6_sessions"
	metricDHCPv6SessionsEstablished    = "dhcpv6_sessions_established"
	metricDHCPv6SessionsEstablishedMax = "dhcpv6_sessions_established_max"
	metricSetupTime                    = "setup_time"
	metricSetupRate                    = "setup_rate"
	metricSetupRateMin                 = "setup_rate_min"
	metricSetupRateAvg                 = "setup_rate_avg"
	metricSetupRateMax                 = "setup_rate_max"
	metricSessionTrafficFlows          = "session_traffic_flows"
	metricSessionTrafficFlowsVerified  = "session_traffic_flows_verified"
	metricStreamTrafficFlows           = "stream_traffic_flows"
	metricStreamTrafficFlowsVerified   = "stream_traffic_flows_verified"
	metricIfTxPackets                  = "interfaces_tx_packets"
	metricIfTxBytes                    = "interfaces_tx_bytes"
	metricIfTxPPS                      = "interfaces_tx_pps"
	metricIfTxKbps                     = "interfaces_tx_kbps"
	metricIfRxPackets                  = "interfaces_rx_packets"
	metricIfRxBytes                    = "interfaces_rx_bytes"
	metricIfRxPPS                      = "interfaces_rx_pps"
	metricIfRxKbps                     = "interfaces_rx_kbps"
	metricIfTxPacketsMulticast         = "interfaces_tx_packets_multicast"
	metricIfTxPPSMulticast             = "interfaces_tx_pps_multicast"
	metricIfRxPacketsMulticast         = "interfaces_rx_packets_multicast"
	metricIfRxPPSMulticast             = "interfaces_rx_pps_multicast"
	metricIfRxLossPacketsMulticast     = "interfaces_rx_loss_packets_multicast"
	metricIfTxPacketsSessionIPv4       = "interfaces_tx_packets_session_ipv4"
	metricIfTxPPSSessionIPv4           = "interfaces_tx_pps_session_ipv4"
	metricIfRxPacketsSessionIPv4       = "interfaces_rx_packets_session_ipv4"
	metricIfRxPPSSessionIPv4           = "interfaces_rx_pps_session_ipv4"
	metricIfRxLossPacketsSessionIPv4   = "interfaces_rx_loss_packets_ipv4"
	metricIfTxPacketsSessionIPv6       = "interfaces_tx_packets_session_ipv6"
	metricIfTxPPSSessionIPv6           = "interfaces_tx_pps_session_ipv6"
	metricIfRxPacketsSessionIPv6       = "interfaces_rx_packets_session_ipv6"
	metricIfRxPPSSessionIPv6           = "interfaces_rx_pps_session_ipv6"
	metricIfRxLossPacketsSessionIPv6   = "interfaces_rx_loss_packets_ipv6"
	metricIfTxPacketsSessionIPv6PD     = "interfaces_tx_packets_session_ipv6pd"
	metricIfTxPPSSessionIPv6PD         = "interfaces_tx_pps_session_ipv6pd"
	metricIfRxPacketsSessionIPv6PD     = "interfaces_rx_packets_session_ipv6pd"
	metricIfRxPPSSessionIPv6PD         = "interfaces_rx_pps_session_ipv6pd"
	metricIfRxLossPacketsSessionIPv6PD = "interfaces_rx_loss_packets_ipv6pd"
	metricIfTxPacketsStreams           = "interfaces_tx_packets_streams"
	metricIfTxPPSStreams               = "interfaces_tx_pps_streams"
	metricIfRxPacketsStreams           = "interfaces_rx_packets_streams"
	metricIfRxPPSStreams               = "interfaces_rx_pps_streams"
	metricIfRxLossPacketsStreams       = "interfaces_rx_loss_packets_streams"
	metricStreamTxPackets              = "stream_tx_packets"
	metricStreamTxBytes                = "stream_tx_bytes"
	metricStreamRxPackets              = "stream_rx_packets"
	metricStreamRxBytes                = "stream_rx_bytes"
	metricStreamRxLoss                 = "stream_rx_loss"

	labelHostname        = "hostname"
	labelInstanceName    = "instance_name"
	labelInterfaceName   = "interface_name"
	labelInterfaceType   = "interface_type"
	labelSessionId       = "session_id"
	labelFlowId          = "flow_id"
	labelStreamName      = "stream_name"
	labelStreamDirection = "stream_direction"
	labelStreamType      = "stream_type"
	labelStreamSubType   = "stream_sub_type"
)

// Prom defines a prometheus export object.
type Prom struct {
	Registry   *prometheus.Registry
	repository Repository

	// Metrics.
	InstancesTotal   *prometheus.Desc
	InstancesRunning *prometheus.Desc
	// Session counters.
	Sessions                     *prometheus.Desc
	SessionsPPPoE                *prometheus.Desc
	SessionsIPoE                 *prometheus.Desc
	SessionsEstablished          *prometheus.Desc
	SessionsEstablishedMax       *prometheus.Desc
	SessionsTerminated           *prometheus.Desc
	SessionsFlapped              *prometheus.Desc
	DHCPSessions                 *prometheus.Desc
	DHCPSessionsEstablished      *prometheus.Desc
	DHCPSessionsEstablishedMax   *prometheus.Desc
	DHCPv6Sessions               *prometheus.Desc
	DHCPv6SessionsEstablished    *prometheus.Desc
	DHCPv6SessionsEstablishedMax *prometheus.Desc
	SetupTime                    *prometheus.Desc
	SetupRate                    *prometheus.Desc
	SetupRateMin                 *prometheus.Desc
	SetupRateAvg                 *prometheus.Desc
	SetupRateMax                 *prometheus.Desc
	SessionTrafficFlows          *prometheus.Desc
	SessionTrafficFlowsVerified  *prometheus.Desc
	StreamTrafficFlows           *prometheus.Desc
	StreamTrafficFlowsVerified   *prometheus.Desc
	// Interfaces.
	IfTxPackets                  *prometheus.Desc
	IfTxBytes                    *prometheus.Desc
	IfTxPPS                      *prometheus.Desc
	IfTxKbps                     *prometheus.Desc
	IfRxPackets                  *prometheus.Desc
	IfRxBytes                    *prometheus.Desc
	IfRxPPS                      *prometheus.Desc
	IfRxKbps                     *prometheus.Desc
	IfTxPacketsMulticast         *prometheus.Desc
	IfTxPPSMulticast             *prometheus.Desc
	IfRxPacketsMulticast         *prometheus.Desc
	IfRxPPSMulticast             *prometheus.Desc
	IfRxLossPacketsMulticast     *prometheus.Desc
	IfTxPacketsSessionIPv4       *prometheus.Desc
	IfTxPPSSessionIPv4           *prometheus.Desc
	IfRxPacketsSessionIPv4       *prometheus.Desc
	IfRxPPSSessionIPv4           *prometheus.Desc
	IfRxLossPacketsSessionIPv4   *prometheus.Desc
	IfTxPacketsSessionIPv6       *prometheus.Desc
	IfTxPPSSessionIPv6           *prometheus.Desc
	IfRxPacketsSessionIPv6       *prometheus.Desc
	IfRxPPSSessionIPv6           *prometheus.Desc
	IfRxLossPacketsSessionIPv6   *prometheus.Desc
	IfTxPacketsSessionIPv6PD     *prometheus.Desc
	IfTxPPSSessionIPv6PD         *prometheus.Desc
	IfRxPacketsSessionIPv6PD     *prometheus.Desc
	IfRxPPSSessionIPv6PD         *prometheus.Desc
	IfRxLossPacketsSessionIPv6PD *prometheus.Desc
	IfTxPacketsStreams           *prometheus.Desc
	IfTxPPSStreams               *prometheus.Desc
	IfRxPacketsStreams           *prometheus.Desc
	IfRxPPSStreams               *prometheus.Desc
	IfRxLossPacketsStreams       *prometheus.Desc
	// Streams.
	StreamTxPackets *prometheus.Desc
	StreamTxBytes   *prometheus.Desc
	StreamRxPackets *prometheus.Desc
	StreamRxBytes   *prometheus.Desc
	StreamRxLoss    *prometheus.Desc
}

// NewProm creates a new prometheus export object.
func NewProm(repository Repository) *Prom {
	p := &Prom{
		Registry:   prometheus.NewRegistry(),
		repository: repository,
	}
	// Get system hostname.
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	// Init metrics.
	p.InstancesTotal = prometheus.NewDesc(metricInstancesTotal,
		"The total number of instances",
		nil, prometheus.Labels{labelHostname: hostname},
	)
	p.InstancesRunning = prometheus.NewDesc(metricInstancesRunning,
		"The number of running instances",
		nil, prometheus.Labels{labelHostname: hostname},
	)
	// Session counters.
	p.Sessions = prometheus.NewDesc(metricSessions,
		"The total number of sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsPPPoE = prometheus.NewDesc(metricSessionsPPPoE,
		"The total number of PPPoE sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsIPoE = prometheus.NewDesc(metricSessionsIPoE,
		"The total number of IPoE sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsEstablished = prometheus.NewDesc(metricSessionsEstablished,
		"The number of sessions in state established",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsEstablishedMax = prometheus.NewDesc(metricSessionsEstablishedMax,
		"The max number of sessions in state established (peak)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsTerminated = prometheus.NewDesc(metricSessionsTerminated,
		"The number of sessions in state terminated",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionsFlapped = prometheus.NewDesc(metricSessionsFlapped,
		"The number of sessions flapped",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPSessions = prometheus.NewDesc(metricDHCPSessions,
		"The number of DHCP sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPSessionsEstablished = prometheus.NewDesc(metricDHCPSessionsEstablished,
		"The number of DHCP sessions in state established",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPSessionsEstablishedMax = prometheus.NewDesc(metricDHCPSessionsEstablishedMax,
		"The max number of DHCP sessions in state established (peak)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPv6Sessions = prometheus.NewDesc(metricDHCPv6Sessions,
		"The number of DHCPv6 sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPv6SessionsEstablished = prometheus.NewDesc(metricDHCPv6SessionsEstablished,
		"The number of DHCPv6 sessions in state established",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.DHCPv6SessionsEstablishedMax = prometheus.NewDesc(metricDHCPv6SessionsEstablishedMax,
		"The max number of DHCPv6 sessions in state established (peak)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SetupTime = prometheus.NewDesc(metricSetupTime,
		"Total setup time",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SetupRate = prometheus.NewDesc(metricSetupRate,
		"Total setup rate (CPS)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SetupRateMin = prometheus.NewDesc(metricSetupRateMin,
		"Minimum setup rate (CPS)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SetupRateAvg = prometheus.NewDesc(metricSetupRateAvg,
		"Average setup rate (CPS)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SetupRateMax = prometheus.NewDesc(metricSetupRateMax,
		"Maximum setup rate (CPS)",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionTrafficFlows = prometheus.NewDesc(metricSessionTrafficFlows,
		"The number of sessions traffic flows",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.SessionTrafficFlowsVerified = prometheus.NewDesc(metricSessionTrafficFlowsVerified,
		"The number of sessions traffic flows verified",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamTrafficFlows = prometheus.NewDesc(metricStreamTrafficFlows,
		"The number of stream traffic flows",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamTrafficFlowsVerified = prometheus.NewDesc(metricStreamTrafficFlowsVerified,
		"The number of stream traffic flows verified",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)
	// Interfaces.
	p.IfTxPackets = prometheus.NewDesc(metricIfTxPackets,
		"Interface TX packets",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxBytes = prometheus.NewDesc(metricIfTxBytes,
		"Interface TX bytes",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPS = prometheus.NewDesc(metricIfTxPPS,
		"Interface TX PPS",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxKbps = prometheus.NewDesc(metricIfTxKbps,
		"Interface TX Kbps",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPackets = prometheus.NewDesc(metricIfRxPackets,
		"Interface RX packets",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxBytes = prometheus.NewDesc(metricIfRxBytes,
		"Interface RX bytes",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPS = prometheus.NewDesc(metricIfRxPPS,
		"Interface RX PPS",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxKbps = prometheus.NewDesc(metricIfRxKbps,
		"Interface RX Kbps",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPacketsMulticast = prometheus.NewDesc(metricIfTxPacketsMulticast,
		"Interface TX packets multicast",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPSMulticast = prometheus.NewDesc(metricIfTxPPSMulticast,
		"Interface TX PPS multicast",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPacketsMulticast = prometheus.NewDesc(metricIfRxPacketsMulticast,
		"Interface RX packets multicast",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPSMulticast = prometheus.NewDesc(metricIfRxPPSMulticast,
		"Interface RX PPS multicast",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxLossPacketsMulticast = prometheus.NewDesc(metricIfRxLossPacketsMulticast,
		"Interface RX loss packets multicast",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPacketsSessionIPv4 = prometheus.NewDesc(metricIfTxPacketsSessionIPv4,
		"Interface TX packets session-traffic IPv4",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPSSessionIPv4 = prometheus.NewDesc(metricIfTxPPSSessionIPv4,
		"Interface TX PPS session-traffic IPv4",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPacketsSessionIPv4 = prometheus.NewDesc(metricIfRxPacketsSessionIPv4,
		"Interface RX packets session-traffic IPv4",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPSSessionIPv4 = prometheus.NewDesc(metricIfRxPPSSessionIPv4,
		"Interface RX PPS session-traffic IPv4",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxLossPacketsSessionIPv4 = prometheus.NewDesc(metricIfRxLossPacketsSessionIPv4,
		"Interface RX loss packets session-traffic IPv4",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPacketsSessionIPv6 = prometheus.NewDesc(metricIfTxPacketsSessionIPv6,
		"Interface TX packets session-traffic IPv6",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPSSessionIPv6 = prometheus.NewDesc(metricIfTxPPSSessionIPv6,
		"Interface TX PPS session-traffic IPv6",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPacketsSessionIPv6 = prometheus.NewDesc(metricIfRxPacketsSessionIPv6,
		"Interface RX packets session-traffic IPv6",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPSSessionIPv6 = prometheus.NewDesc(metricIfRxPPSSessionIPv6,
		"Interface RX PPS session-traffic IPv6",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxLossPacketsSessionIPv6 = prometheus.NewDesc(metricIfRxLossPacketsSessionIPv6,
		"Interface RX loss packets session-traffic IPv6",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPacketsSessionIPv6PD = prometheus.NewDesc(metricIfTxPacketsSessionIPv6PD,
		"Interface TX packets session-traffic IPv6PD",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPSSessionIPv6PD = prometheus.NewDesc(metricIfTxPPSSessionIPv6PD,
		"Interface TX PPS session-traffic IPv6PD",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPacketsSessionIPv6PD = prometheus.NewDesc(metricIfRxPacketsSessionIPv6PD,
		"Interface RX packets session-traffic IPv6PD",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPSSessionIPv6PD = prometheus.NewDesc(metricIfRxPPSSessionIPv6PD,
		"Interface RX PPS session-traffic IPv6PD",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxLossPacketsSessionIPv6PD = prometheus.NewDesc(metricIfRxLossPacketsSessionIPv6PD,
		"Interface RX loss packets session-traffic IPv6PD",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPacketsStreams = prometheus.NewDesc(metricIfTxPacketsStreams,
		"Interface TX packets stream-traffic",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfTxPPSStreams = prometheus.NewDesc(metricIfTxPPSStreams,
		"Interface TX PPS stream-traffic",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPacketsStreams = prometheus.NewDesc(metricIfRxPacketsStreams,
		"Interface RX packets stream-traffic",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxPPSStreams = prometheus.NewDesc(metricIfRxPPSStreams,
		"Interface RX PPS stream-traffic",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	p.IfRxLossPacketsStreams = prometheus.NewDesc(metricIfRxLossPacketsStreams,
		"Interface RX loss packets stream-traffic",
		[]string{labelInstanceName, labelInterfaceName, labelInterfaceType}, prometheus.Labels{labelHostname: hostname},
	)
	// Streams.
	p.StreamTxPackets = prometheus.NewDesc(metricStreamTxPackets,
		"Stream TX packets",
		[]string{labelInstanceName, labelFlowId, labelSessionId, labelStreamName, labelStreamDirection, labelStreamType, labelStreamSubType}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamTxBytes = prometheus.NewDesc(metricStreamTxBytes,
		"Stream TX bytes",
		[]string{labelInstanceName, labelFlowId, labelSessionId, labelStreamName, labelStreamDirection, labelStreamType, labelStreamSubType}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamRxPackets = prometheus.NewDesc(metricStreamRxPackets,
		"Stream RX packets",
		[]string{labelInstanceName, labelFlowId, labelSessionId, labelStreamName, labelStreamDirection, labelStreamType, labelStreamSubType}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamRxBytes = prometheus.NewDesc(metricStreamRxBytes,
		"Stream RX bytes",
		[]string{labelInstanceName, labelFlowId, labelSessionId, labelStreamName, labelStreamDirection, labelStreamType, labelStreamSubType}, prometheus.Labels{labelHostname: hostname},
	)
	p.StreamRxLoss = prometheus.NewDesc(metricStreamRxLoss,
		"Stream RX loss",
		[]string{labelInstanceName, labelFlowId, labelSessionId, labelStreamName, labelStreamDirection, labelStreamType, labelStreamSubType}, prometheus.Labels{labelHostname: hostname},
	)

	// Register all metrics and return.
	p.Registry.MustRegister(p)
	return p
}

// Describe all the metrics.
func (p *Prom) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.InstancesTotal
	ch <- p.InstancesRunning
	// Session counters.
	ch <- p.Sessions
	ch <- p.SessionsPPPoE
	ch <- p.SessionsIPoE
	ch <- p.SessionsEstablished
	ch <- p.SessionsEstablishedMax
	ch <- p.SessionsTerminated
	ch <- p.SessionsFlapped
	ch <- p.DHCPSessions
	ch <- p.DHCPSessionsEstablished
	ch <- p.DHCPSessionsEstablishedMax
	ch <- p.DHCPv6Sessions
	ch <- p.DHCPv6SessionsEstablished
	ch <- p.DHCPv6SessionsEstablishedMax
	ch <- p.SetupTime
	ch <- p.SetupRate
	ch <- p.SetupRateMin
	ch <- p.SetupRateAvg
	ch <- p.SetupRateMax
	ch <- p.SessionTrafficFlows
	ch <- p.SessionTrafficFlowsVerified
	ch <- p.StreamTrafficFlows
	ch <- p.StreamTrafficFlowsVerified
	// Interfaces.
	ch <- p.IfTxPackets
	ch <- p.IfTxBytes
	ch <- p.IfTxPPS
	ch <- p.IfTxKbps
	ch <- p.IfRxPackets
	ch <- p.IfRxBytes
	ch <- p.IfRxPPS
	ch <- p.IfRxKbps
	ch <- p.IfTxPacketsMulticast
	ch <- p.IfTxPPSMulticast
	ch <- p.IfRxPacketsMulticast
	ch <- p.IfRxPPSMulticast
	ch <- p.IfRxLossPacketsMulticast
	ch <- p.IfTxPacketsSessionIPv4
	ch <- p.IfTxPPSSessionIPv4
	ch <- p.IfRxPacketsSessionIPv4
	ch <- p.IfRxPPSSessionIPv4
	ch <- p.IfRxLossPacketsSessionIPv4
	ch <- p.IfTxPacketsSessionIPv6
	ch <- p.IfTxPPSSessionIPv6
	ch <- p.IfRxPacketsSessionIPv6
	ch <- p.IfRxPPSSessionIPv6
	ch <- p.IfRxLossPacketsSessionIPv6
	ch <- p.IfTxPacketsSessionIPv6PD
	ch <- p.IfTxPPSSessionIPv6PD
	ch <- p.IfRxPacketsSessionIPv6PD
	ch <- p.IfRxPPSSessionIPv6PD
	ch <- p.IfRxLossPacketsSessionIPv6PD
	ch <- p.IfTxPacketsStreams
	ch <- p.IfTxPPSStreams
	ch <- p.IfRxPacketsStreams
	ch <- p.IfRxPPSStreams
	ch <- p.IfRxLossPacketsStreams
	ch <- p.StreamTxPackets
	ch <- p.StreamTxBytes
	ch <- p.StreamRxPackets
	ch <- p.StreamRxBytes
	ch <- p.StreamRxLoss
}

// Collect implements required collect function for all metrics collectors.
func (p *Prom) Collect(ch chan<- prometheus.Metric) {
	total := float64(0)
	running := float64(0)

	var wg sync.WaitGroup

	instances, _ := os.ReadDir(p.repository.ConfigFolder())
	for _, instance := range instances {
		if instance.IsDir() {
			total++
			if p.repository.Running(instance.Name()) {
				running++
				wg.Add(1)
				go p.collectInstance(&wg, instance.Name(), ch)
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(p.InstancesTotal, prometheus.GaugeValue, total)
	ch <- prometheus.MustNewConstMetric(p.InstancesRunning, prometheus.GaugeValue, running)
	wg.Wait()
}

func (p *Prom) collectInstanceSessionCounters(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "session-counters",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute session-counters: %s", err.Error())
		return
	}
	// Decode response.
	var cr SessionCountersResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode session-counters: %s", err.Error())
		return
	}
	// Return Metrics.
	ch <- prometheus.MustNewConstMetric(p.Sessions, prometheus.CounterValue, float64(cr.SessionCounters.Sessions), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsPPPoE, prometheus.CounterValue, float64(cr.SessionCounters.SessionsPPPoE), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsIPoE, prometheus.CounterValue, float64(cr.SessionCounters.SessionsIPoE), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsEstablished, prometheus.GaugeValue, float64(cr.SessionCounters.SessionsEstablished), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsEstablishedMax, prometheus.CounterValue, float64(cr.SessionCounters.SessionsEstablishedMax), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsTerminated, prometheus.GaugeValue, float64(cr.SessionCounters.SessionsTerminated), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionsFlapped, prometheus.CounterValue, float64(cr.SessionCounters.SessionsFlapped), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPSessions, prometheus.CounterValue, float64(cr.SessionCounters.DHCPSessions), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPSessionsEstablished, prometheus.GaugeValue, float64(cr.SessionCounters.DHCPSessionsEstablished), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPSessionsEstablishedMax, prometheus.CounterValue, float64(cr.SessionCounters.DHCPSessionsEstablishedMax), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPv6Sessions, prometheus.CounterValue, float64(cr.SessionCounters.DHCPv6Sessions), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPv6SessionsEstablished, prometheus.GaugeValue, float64(cr.SessionCounters.DHCPv6SessionsEstablished), instance)
	ch <- prometheus.MustNewConstMetric(p.DHCPv6SessionsEstablishedMax, prometheus.CounterValue, float64(cr.SessionCounters.DHCPv6SessionsEstablishedMax), instance)
	ch <- prometheus.MustNewConstMetric(p.SetupTime, prometheus.GaugeValue, float64(cr.SessionCounters.SetupTime), instance)
	ch <- prometheus.MustNewConstMetric(p.SetupRate, prometheus.GaugeValue, cr.SessionCounters.SetupRate, instance)
	ch <- prometheus.MustNewConstMetric(p.SetupRateMin, prometheus.GaugeValue, cr.SessionCounters.SetupRateMin, instance)
	ch <- prometheus.MustNewConstMetric(p.SetupRateAvg, prometheus.GaugeValue, cr.SessionCounters.SetupRateAvg, instance)
	ch <- prometheus.MustNewConstMetric(p.SetupRateMax, prometheus.GaugeValue, cr.SessionCounters.SetupRateMax, instance)
	ch <- prometheus.MustNewConstMetric(p.SessionTrafficFlows, prometheus.GaugeValue, float64(cr.SessionCounters.SessionTrafficFlows), instance)
	ch <- prometheus.MustNewConstMetric(p.SessionTrafficFlowsVerified, prometheus.GaugeValue, float64(cr.SessionCounters.SessionTrafficFlowsVerified), instance)
	ch <- prometheus.MustNewConstMetric(p.StreamTrafficFlows, prometheus.GaugeValue, float64(cr.SessionCounters.StreamTrafficFlows), instance)
	ch <- prometheus.MustNewConstMetric(p.StreamTrafficFlowsVerified, prometheus.GaugeValue, float64(cr.SessionCounters.StreamTrafficFlowsVerified), instance)
}

func (p *Prom) collectInstanceInterfaces(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "interfaces",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute interfaces: %s", err.Error())
		return
	}
	// Decode response.
	var cr InterfacesResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode interfaces: %s", err.Error())
		return
	}
	// Return Metrics.
	for _, iface := range cr.Interfaces {
		ch <- prometheus.MustNewConstMetric(p.IfTxPackets, prometheus.CounterValue, float64(iface.TxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxBytes, prometheus.CounterValue, float64(iface.TxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPackets, prometheus.CounterValue, float64(iface.RxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxBytes, prometheus.CounterValue, float64(iface.RxBytes), instance, iface.Name, iface.Type)
	}
}

func (p *Prom) collectInstanceAccessInterfaces(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "access-interfaces",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute access-interfaces: %s", err.Error())
		return
	}
	// Decode response.
	var cr AccessInterfacesResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode access-interfaces: %s", err.Error())
		return
	}
	// Return Metrics.
	for _, iface := range cr.Interfaces {
		ch <- prometheus.MustNewConstMetric(p.IfTxPackets, prometheus.CounterValue, float64(iface.TxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxBytes, prometheus.CounterValue, float64(iface.TxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPS, prometheus.GaugeValue, float64(iface.TxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxKbps, prometheus.GaugeValue, float64(iface.TxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPackets, prometheus.CounterValue, float64(iface.RxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxBytes, prometheus.CounterValue, float64(iface.RxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPS, prometheus.GaugeValue, float64(iface.RxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxKbps, prometheus.GaugeValue, float64(iface.RxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsMulticast, prometheus.CounterValue, float64(iface.RxPacketsMulticast), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSMulticast, prometheus.GaugeValue, float64(iface.RxPPSMulticast), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsMulticast, prometheus.CounterValue, float64(iface.RxLossPacketsMulticast), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsStreams, prometheus.CounterValue, float64(iface.TxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSStreams, prometheus.GaugeValue, float64(iface.TxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsStreams, prometheus.CounterValue, float64(iface.RxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSStreams, prometheus.GaugeValue, float64(iface.RxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsStreams, prometheus.CounterValue, float64(iface.RxLossPacketsStreams), instance, iface.Name, iface.Type)
	}
}

func (p *Prom) collectInstanceNetworkInterfaces(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "network-interfaces",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute network-interfaces: %s", err.Error())
		return
	}
	// Decode response.
	var cr NetworkInterfacesResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode network-interfaces: %s", err.Error())
		return
	}
	// Return Metrics.
	for _, iface := range cr.Interfaces {
		ch <- prometheus.MustNewConstMetric(p.IfTxPackets, prometheus.CounterValue, float64(iface.TxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxBytes, prometheus.CounterValue, float64(iface.TxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPS, prometheus.GaugeValue, float64(iface.TxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxKbps, prometheus.GaugeValue, float64(iface.TxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPackets, prometheus.CounterValue, float64(iface.RxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxBytes, prometheus.CounterValue, float64(iface.RxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPS, prometheus.GaugeValue, float64(iface.RxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxKbps, prometheus.GaugeValue, float64(iface.RxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsMulticast, prometheus.CounterValue, float64(iface.TxPacketsMulticast), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSMulticast, prometheus.GaugeValue, float64(iface.TxPPSMulticast), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsStreams, prometheus.CounterValue, float64(iface.TxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSStreams, prometheus.GaugeValue, float64(iface.TxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsStreams, prometheus.CounterValue, float64(iface.RxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSStreams, prometheus.GaugeValue, float64(iface.RxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsStreams, prometheus.CounterValue, float64(iface.RxLossPacketsStreams), instance, iface.Name, iface.Type)
	}
}

func (p *Prom) collectInstanceA10nspInterfaces(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "a10nsp-interfaces",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute a10nsp-interfaces: %s", err.Error())
		return
	}
	// Decode response.
	var cr A10nspInterfacesResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode a10nsp-interfaces: %s", err.Error())
		return
	}
	// Return Metrics.
	for _, iface := range cr.Interfaces {
		ch <- prometheus.MustNewConstMetric(p.IfTxPackets, prometheus.CounterValue, float64(iface.TxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxBytes, prometheus.CounterValue, float64(iface.TxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPS, prometheus.GaugeValue, float64(iface.TxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxKbps, prometheus.GaugeValue, float64(iface.TxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPackets, prometheus.CounterValue, float64(iface.RxPackets), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxBytes, prometheus.CounterValue, float64(iface.RxBytes), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPS, prometheus.GaugeValue, float64(iface.RxPPS), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxKbps, prometheus.GaugeValue, float64(iface.RxKbps), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv4, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv4, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv4), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.TxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.TxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSSessionIPv6PD, prometheus.GaugeValue, float64(iface.RxPPSSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsSessionIPv6PD, prometheus.CounterValue, float64(iface.RxLossPacketsSessionIPv6PD), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPacketsStreams, prometheus.CounterValue, float64(iface.TxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfTxPPSStreams, prometheus.GaugeValue, float64(iface.TxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPacketsStreams, prometheus.CounterValue, float64(iface.RxPacketsStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxPPSStreams, prometheus.GaugeValue, float64(iface.RxPPSStreams), instance, iface.Name, iface.Type)
		ch <- prometheus.MustNewConstMetric(p.IfRxLossPacketsStreams, prometheus.CounterValue, float64(iface.RxLossPacketsStreams), instance, iface.Name, iface.Type)
	}
}

func (p *Prom) collectInstanceStreams(instance string, ch chan<- prometheus.Metric) {
	// Invoke command.
	command := SocketCommand{
		Command: "stream-summary",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		log.Warn().Msgf("failed to execute stream-summary: %s", err.Error())
		return
	}
	// Decode response.
	var cr StreamSummaryResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode stream-summary: %s", err.Error())
		return
	}
	// Return Metrics.
	for _, stream := range cr.Streams {
		fid := strconv.Itoa(stream.FlowId)
		sid := strconv.Itoa(stream.SessionId)
		ch <- prometheus.MustNewConstMetric(p.StreamTxPackets, prometheus.CounterValue, float64(stream.TxPackets), instance, fid, sid, stream.Name, stream.Direction, stream.Type, stream.SubType)
		ch <- prometheus.MustNewConstMetric(p.StreamTxBytes, prometheus.CounterValue, float64(stream.TxBytes), instance, fid, sid, stream.Name, stream.Direction, stream.Type, stream.SubType)
		ch <- prometheus.MustNewConstMetric(p.StreamRxPackets, prometheus.CounterValue, float64(stream.RxPackets), instance, fid, sid, stream.Name, stream.Direction, stream.Type, stream.SubType)
		ch <- prometheus.MustNewConstMetric(p.StreamRxBytes, prometheus.CounterValue, float64(stream.RxBytes), instance, fid, sid, stream.Name, stream.Direction, stream.Type, stream.SubType)
		ch <- prometheus.MustNewConstMetric(p.StreamRxLoss, prometheus.CounterValue, float64(stream.RxLoss), instance, fid, sid, stream.Name, stream.Direction, stream.Type, stream.SubType)
	}
}

func (p *Prom) collectInstance(wg *sync.WaitGroup, instance string, ch chan<- prometheus.Metric) {
	defer wg.Done()

	folder := path.Join(p.repository.ConfigFolder(), instance)
	path := path.Join(folder, RunConfigFilename)
	file, err := os.Open(path)
	if err != nil {
		log.Warn().Msgf("failed to open %s: %s", path, err.Error())
		fmt.Println(err)
		return
	}

	var runningConfig RunningConfig
	err = json.NewDecoder(file).Decode(&runningConfig)
	if err != nil {
		log.Warn().Msgf("failed to decode %s: %s", path, err.Error())
		return
	}

	for _, flag := range runningConfig.MetricFlags {
		switch flag {
		case "session_counters":
			p.collectInstanceSessionCounters(instance, ch)
		case "interfaces":
			p.collectInstanceInterfaces(instance, ch)
		case "access_interfaces":
			p.collectInstanceAccessInterfaces(instance, ch)
		case "network_interfaces":
			p.collectInstanceNetworkInterfaces(instance, ch)
		case "a10nsp_interfaces":
			p.collectInstanceA10nspInterfaces(instance, ch)
		case "streams":
			p.collectInstanceStreams(instance, ch)
		default:
			log.Warn().Msgf("unknown metrics flag: %s", flag)
		}
	}
}
