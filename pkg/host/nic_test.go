// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCIeGeneration(t *testing.T) {
	require.Equal(t, 1, PCIeGeneration("2.5 GT/s PCIe"))
	require.Equal(t, 3, PCIeGeneration("8.0 GT/s PCIe"))
	require.Equal(t, 4, PCIeGeneration("16.0 GT/s PCIe"))
	require.Equal(t, 5, PCIeGeneration("32.0 GT/s"))
	require.Equal(t, 0, PCIeGeneration(""))
	require.Equal(t, 0, PCIeGeneration("Unknown"))
	require.Equal(t, 0, PCIeGeneration("3.0 GT/s"))
}

const ethtoolChannels = `Channel parameters for enp1s0f0np0:
Pre-set maximums:
RX:		n/a
TX:		n/a
Other:		1
Combined:	63
Current hardware settings:
RX:		n/a
TX:		n/a
Other:		1
Combined:	8
`

func TestParseChannels(t *testing.T) {
	require.Equal(t, &Channels{
		Max:     ChannelCounts{Other: new(1), Combined: new(63)},
		Current: ChannelCounts{Other: new(1), Combined: new(8)},
	}, parseChannels([]byte(ethtoolChannels)))

	// Separate RX and TX channels, as some drivers report them.
	require.Equal(t, &Channels{
		Max:     ChannelCounts{RX: new(16), TX: new(16)},
		Current: ChannelCounts{RX: new(4), TX: new(2)},
	}, parseChannels([]byte("Pre-set maximums:\nRX: 16\nTX: 16\nCurrent hardware settings:\nRX: 4\nTX: 2\n")))

	// Not supported, or output of another ethtool version.
	require.Nil(t, parseChannels([]byte("netlink error: Operation not supported\n")))
	require.Nil(t, parseChannels(nil))
}

const ethtoolRings = `Ring parameters for enp1s0f0np0:
Pre-set maximums:
RX:		8160
RX Mini:	n/a
RX Jumbo:	n/a
TX:		8160
Current hardware settings:
RX:		4096
RX Mini:	n/a
RX Jumbo:	n/a
TX:		512
RX Buf Len:		n/a
CQE Size:		n/a
TX Push:	off
TCP data split:	n/a
`

func TestParseRings(t *testing.T) {
	require.Equal(t, &Rings{
		Max:     RingCounts{RX: new(8160), TX: new(8160)},
		Current: RingCounts{RX: new(4096), TX: new(512)},
	}, parseRings([]byte(ethtoolRings)))

	require.Equal(t, &Rings{
		Max:     RingCounts{RX: new(4096), RXMini: new(512), RXJumbo: new(2048), TX: new(4096)},
		Current: RingCounts{RX: new(1024), RXMini: new(128), RXJumbo: new(512), TX: new(1024)},
	}, parseRings([]byte("Pre-set maximums:\nRX: 4096\nRX Mini: 512\nRX Jumbo: 2048\nTX: 4096\n"+
		"Current hardware settings:\nRX: 1024\nRX Mini: 128\nRX Jumbo: 512\nTX: 1024\n")))

	require.Nil(t, parseRings([]byte("Cannot get device ring settings: Operation not supported\n")))
}

func TestParseKeyValues(t *testing.T) {
	values := parseKeyValues([]byte("Settings for eth0:\n\tSupported ports: [ FIBRE ]\n" +
		"\tSpeed: 25000Mb/s\n\tDuplex: Full\n\tPort: FIBRE\n\tAuto-negotiation: off\n\tLink detected: yes\n"))
	require.Equal(t, "25000Mb/s", values["speed"])
	require.Equal(t, "FIBRE", values["port"])
	require.Equal(t, "off", values["auto-negotiation"])
	require.Equal(t, "yes", values["link detected"])
}

func TestParsePCIIDs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pci.ids")
	require.NoError(t, os.WriteFile(file, []byte("# comment\n"+
		"15b3  Mellanox Technologies\n"+
		"\t101d  MT2892 Family [ConnectX-6 Dx]\n"+
		"\t\t15b3 0042  subsystem\n"+
		"8086  Intel Corporation\n"+
		"\t1572  Ethernet Controller X710 for 10GbE SFP+\n"+
		"\t1583  Ethernet Controller XL710 for 40GbE QSFP+\n"+
		"C 02  Network controller\n"+
		"\t00  Ethernet controller\n"), 0o600))
	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()

	vendors, devices := parsePCIIDs(f, map[string]map[string]bool{
		"8086": {"1572": true},
		"15b3": {"101d": true},
	})
	require.Equal(t, map[string]string{"8086": "Intel Corporation", "15b3": "Mellanox Technologies"}, vendors)
	require.Equal(t, map[[2]string]string{
		{"8086", "1572"}: "Ethernet Controller X710 for 10GbE SFP+",
		{"15b3", "101d"}: "MT2892 Family [ConnectX-6 Dx]",
	}, devices)
}

// fixtureNICs builds a host with the PCI NIC eth0 (degraded PCIe link),
// the virtio NIC eth1, the SR-IOV virtual function eth2 of eth0 and the
// virtual interface veth0.
func fixtureNICs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pci := "/sys/devices/pci0000:00/0000:00:01.0/0000:01:00.0"
	virtioPCI := "/sys/devices/pci0000:00/0000:00:02.0/0000:02:00.0"
	virtio := virtioPCI + "/virtio1"
	vfPCI := "/sys/devices/pci0000:00/0000:00:01.0/0000:01:02.5"
	writeFiles(t, root, map[string]string{
		"/sys/class/net/lo/ifindex": "1\n",

		"/sys/class/net/eth0/ifindex":       "2\n",
		"/sys/class/net/eth0/address":       "aa:bb:cc:00:00:01\n",
		"/sys/class/net/eth0/mtu":           "9000\n",
		"/sys/class/net/eth0/operstate":     "up\n",
		"/sys/class/net/eth0/carrier":       "1\n",
		"/sys/class/net/eth0/speed":         "100000\n",
		"/sys/class/net/eth0/duplex":        "full\n",
		"/sys/class/net/eth0/queues/rx-0/x": "",
		"/sys/class/net/eth0/queues/rx-1/x": "",
		"/sys/class/net/eth0/queues/tx-0/x": "",
		pci + "/vendor":                     "0x15b3\n",
		pci + "/device":                     "0x101d\n",
		pci + "/numa_node":                  "1\n",
		pci + "/current_link_speed":         "8.0 GT/s PCIe\n",
		pci + "/current_link_width":         "8\n",
		pci + "/max_link_speed":             "16.0 GT/s PCIe\n",
		pci + "/max_link_width":             "16\n",
		pci + "/sriov_totalvfs":             "8\n",

		"/sys/class/net/eth1/ifindex":     "3\n",
		"/sys/class/net/eth1/operstate":   "down\n",
		"/sys/class/net/eth1/speed":       "-1\n",
		virtioPCI + "/vendor":             "0x1af4\n",
		virtioPCI + "/device":             "0x1041\n",
		virtioPCI + "/numa_node":          "-1\n",
		virtioPCI + "/current_link_speed": "Unknown\n",
		virtioPCI + "/current_link_width": "0\n",
		virtio + "/features":              "",

		// eth2 is an SR-IOV virtual function of eth0.
		"/sys/class/net/eth2/ifindex":   "5\n",
		"/sys/class/net/eth2/operstate": "up\n",
		vfPCI + "/vendor":               "0x8086\n",
		vfPCI + "/device":               "0x154c\n",
		pci + "/net/eth0/ifindex":       "2\n",

		"/sys/class/net/veth0/ifindex":   "4\n",
		"/sys/class/net/veth0/operstate": "up\n",
		"/sys/class/net/veth0/carrier":   "1\n",
		"/sys/class/net/veth0/speed":     "10000\n",
		"/sys/class/net/veth0/uevent":    "INTERFACE=veth0\n",

		"/sys/bus/pci/drivers/mlx5_core/bind": "",
		"/sys/bus/virtio/drivers_probe":       "",
		"/usr/share/misc/pci.ids":             "15b3  Mellanox Technologies\n\t101d  MT2892 Family [ConnectX-6 Dx]\n",
	})
	link := func(target, name string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(root, target), filepath.Join(root, name)))
	}
	link(pci, "/sys/class/net/eth0/device")
	link("/sys/bus/pci", pci+"/subsystem")
	link("/sys/bus/pci/drivers/mlx5_core", pci+"/driver")
	link(pci, "/sys/bus/pci/devices/0000:01:00.0")
	// The NIC device of virtio is below its PCI device.
	link(virtio, "/sys/class/net/eth1/device")
	link("/sys/bus/virtio", virtio+"/subsystem")
	link(virtioPCI, "/sys/bus/pci/devices/0000:02:00.0")
	link(vfPCI, "/sys/class/net/eth2/device")
	link("/sys/bus/pci", vfPCI+"/subsystem")
	link(pci, vfPCI+"/physfn")
	link(vfPCI, "/sys/bus/pci/devices/0000:01:02.5")
	return root
}

// fakeEthtool answers like ethtool for eth0 and veth0, and fails otherwise.
// Discover queries the interfaces in parallel, so recording the calls is
// guarded.
func fakeEthtool(calls *[]string) func(context.Context, ...string) ([]byte, error) {
	var mu sync.Mutex
	return func(_ context.Context, args ...string) ([]byte, error) {
		mu.Lock()
		*calls = append(*calls, strings.Join(args, " "))
		mu.Unlock()
		switch strings.Join(args, " ") {
		case "-i eth0":
			return []byte("driver: mlx5_core\nversion: 6.8.0\nfirmware-version: 22.36.1010 (MT_0000000359)\nbus-info: 0000:01:00.0\n"), nil
		case "-P eth0":
			return []byte("Permanent address: aa:bb:cc:00:00:01\n"), nil
		case "eth0":
			return []byte("Settings for eth0:\n\tSpeed: 100000Mb/s\n\tPort: Direct Attach Copper\n\tAuto-negotiation: on\n"), nil
		case "-l eth0":
			return []byte(ethtoolChannels), nil
		case "-g eth0":
			return []byte(ethtoolRings), nil
		case "-i veth0":
			return []byte("driver: veth\nversion: 1.0\n"), nil
		}
		return nil, errors.New("exit status 1")
	}
}

func TestNICInventory(t *testing.T) {
	root := fixtureNICs(t)
	var calls []string
	inv := NewNICInventory(WithRoot(root))
	inv.ethtool = fakeEthtool(&calls)

	nics := inv.Interfaces(context.Background())
	require.Len(t, nics, 4)

	eth0 := nics[0]
	require.Equal(t, NICDetails{
		Name: "eth0", Kind: "pci", Physical: true, Driver: "mlx5_core", DriverVersion: "6.8.0",
		Firmware: "22.36.1010 (MT_0000000359)", PermanentMAC: "aa:bb:cc:00:00:01",
		PCI: &PCIInfo{
			Address: "0000:01:00.0", Vendor: "15b3", Device: "101d",
			VendorName: "Mellanox Technologies", DeviceName: "MT2892 Family [ConnectX-6 Dx]",
			NUMANode: 1, LinkSpeed: "8.0 GT/s PCIe", LinkGen: 3, LinkWidth: 8,
			MaxLinkSpeed: "16.0 GT/s PCIe", MaxLinkGen: 4, MaxLinkWidth: 16, SRIOVTotal: 8,
		},
		index: 2,
	}, eth0.NICDetails)
	carrier := true
	require.Equal(t, NICState{
		MAC: "aa:bb:cc:00:00:01", MTU: 9000, OperState: "up", Carrier: &carrier, Speed: new(100000),
		Duplex: "full", Port: "Direct Attach Copper", AutoNeg: "on", RXQueues: 2, TXQueues: 1,
		Channels: parseChannels([]byte(ethtoolChannels)),
		Rings:    parseRings([]byte(ethtoolRings)),
	}, eth0.NICState)

	// ethtool fails for eth1: the sysfs values remain.
	eth1 := nics[1]
	require.Equal(t, "eth1", eth1.Name)
	require.Equal(t, "pci", eth1.Kind)
	require.True(t, eth1.Physical)
	require.Equal(t, &PCIInfo{Address: "0000:02:00.0", Vendor: "1af4", Device: "1041", NUMANode: -1}, eth1.PCI)
	require.Equal(t, "down", eth1.OperState)
	require.Nil(t, eth1.Carrier)
	require.Nil(t, eth1.Speed)
	require.Nil(t, eth1.Channels)
	require.Nil(t, eth1.Rings)

	// Virtual functions follow the physical NICs.
	eth2 := nics[2]
	require.Equal(t, "eth2", eth2.Name)
	require.True(t, eth2.Physical)
	require.True(t, eth2.VirtualFunction())
	require.Equal(t, "0000:01:00.0", eth2.PCI.PhysicalFunction)
	require.Equal(t, "eth0", eth2.PCI.PhysicalFunctionInterface)
	require.False(t, eth0.VirtualFunction())

	veth0 := nics[3]
	require.Equal(t, "veth0", veth0.Name)
	require.Equal(t, "veth", veth0.Kind)
	require.False(t, veth0.Physical)
	require.Nil(t, veth0.PCI)

	// The details are discovered once; only the state is read again.
	calls = nil
	inv.Interfaces(context.Background())
	for _, call := range calls {
		require.False(t, strings.HasPrefix(call, "-i ") || strings.HasPrefix(call, "-P "), call)
	}

	// A new interface is discovered, a removed one forgotten.
	writeFiles(t, root, map[string]string{"/sys/class/net/veth1/ifindex": "5\n"})
	require.NoError(t, os.RemoveAll(filepath.Join(root, "/sys/class/net/veth0")))
	names := []string{}
	for _, nic := range inv.Interfaces(context.Background()) {
		names = append(names, nic.Name)
	}
	require.Equal(t, []string{"eth0", "eth1", "eth2", "veth1"}, names)
}

func TestNICInventory_withoutEthtool(t *testing.T) {
	root := fixtureNICs(t)
	inv := NewNICInventory(WithRoot(root))
	inv.ethtool = func(context.Context, ...string) ([]byte, error) { return nil, ErrNoEthtool }

	nics := inv.Interfaces(context.Background())
	require.Len(t, nics, 4)
	// Driver from sysfs, PCI and state from sysfs, nothing from ethtool.
	require.Equal(t, "mlx5_core", nics[0].Driver)
	require.Empty(t, nics[0].Firmware)
	require.Equal(t, 3, nics[0].PCI.LinkGen)
	require.Equal(t, new(100000), nics[0].Speed)
	require.Equal(t, 2, nics[0].RXQueues)
	require.Nil(t, nics[0].Channels)
	require.Nil(t, nics[0].Rings)
	require.Empty(t, nics[0].Port)
	require.Equal(t, "virtual", nics[3].Kind)
}
