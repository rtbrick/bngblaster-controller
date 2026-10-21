// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package host

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Interfaces are reported in two parts. NICDetails (driver, firmware, PCI
// device and link) do not change while the controller runs and take a few
// ethtool runs and a pass over pci.ids to discover, so they are discovered
// once per interface. NICState (link, speed, queues) changes over time and
// is read whenever it is requested.
//
// ethtool is optional: without it, or if its output cannot be parsed, the
// values available in sysfs are reported and the others are left empty.

// NIC is a network interface with its details and current state.
type NIC struct {
	NICDetails
	NICState
}

// NICDetails are the static properties of a network interface.
type NICDetails struct {
	Name string `json:"name"`
	// Kind is "pci" for a PCI device, the bus of other devices, or the
	// type of a virtual interface (e.g. "veth", "bridge", "vlan").
	Kind          string   `json:"kind"`
	Physical      bool     `json:"physical"`
	Driver        string   `json:"driver,omitempty"`
	DriverVersion string   `json:"driver_version,omitempty"`
	Firmware      string   `json:"firmware,omitempty"`
	PermanentMAC  string   `json:"permanent_mac,omitempty"`
	PCI           *PCIInfo `json:"pci,omitempty"`

	// index tells a recreated interface of the same name apart.
	index int
}

// PCIInfo describes the PCI device of a network interface.
type PCIInfo struct {
	Address string `json:"address"`
	Vendor  string `json:"vendor"`
	Device  string `json:"device"`
	// VendorName and DeviceName are resolved from pci.ids, if installed.
	VendorName string `json:"vendor_name,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	// NUMANode is -1 if the platform does not report it.
	NUMANode int `json:"numa_node"`
	// Link speed per lane ("8.0 GT/s PCIe"), PCIe generation and number
	// of lanes; 0 if unknown (e.g. virtual functions or within VMs).
	LinkSpeed    string `json:"link_speed,omitempty"`
	LinkGen      int    `json:"link_gen,omitempty"`
	LinkWidth    int    `json:"link_width,omitempty"`
	MaxLinkSpeed string `json:"max_link_speed,omitempty"`
	MaxLinkGen   int    `json:"max_link_gen,omitempty"`
	MaxLinkWidth int    `json:"max_link_width,omitempty"`
	SRIOVTotal   int    `json:"sriov_total_vfs,omitempty"`
}

// NICState is the current state of a network interface.
type NICState struct {
	MAC       string `json:"mac"`
	MTU       int    `json:"mtu"`
	OperState string `json:"operstate"`
	// Carrier, Speed (Mb/s) and Duplex are nil or empty if unknown,
	// e.g. while the interface is down.
	Carrier *bool  `json:"carrier"`
	Speed   *int   `json:"speed"`
	Duplex  string `json:"duplex,omitempty"`
	Port    string `json:"port,omitempty"`
	AutoNeg string `json:"autoneg,omitempty"`
	// RXQueues and TXQueues are the queues in use, as listed in sysfs.
	RXQueues int `json:"rx_queues"`
	TXQueues int `json:"tx_queues"`
	// Channels are the configured and maximum channels (ethtool -l),
	// nil if not available.
	Channels *Channels `json:"channels"`
	// Rings are the configured and maximum ring sizes (ethtool -g), nil
	// if not available.
	Rings *Rings `json:"rings"`
}

// Channels are the queue counts reported by "ethtool -l". A nil value
// is "n/a" (not supported separately by the driver).
type Channels struct {
	Max     ChannelCounts `json:"max"`
	Current ChannelCounts `json:"current"`
}

// ChannelCounts are the counts per channel type.
type ChannelCounts struct {
	RX       *int `json:"rx"`
	TX       *int `json:"tx"`
	Other    *int `json:"other"`
	Combined *int `json:"combined"`
}

// Rings are the ring (descriptor) counts reported by "ethtool -g". A nil
// value is "n/a".
type Rings struct {
	Max     RingCounts `json:"max"`
	Current RingCounts `json:"current"`
}

// RingCounts are the counts per ring type.
type RingCounts struct {
	RX      *int `json:"rx"`
	RXMini  *int `json:"rx_mini"`
	RXJumbo *int `json:"rx_jumbo"`
	TX      *int `json:"tx"`
}

const (
	// ethtoolTimeout bounds one ethtool run; a driver that hangs must
	// not hang the request.
	ethtoolTimeout = 3 * time.Second
	// nicConcurrency bounds the interfaces read at the same time.
	nicConcurrency = 8
)

// pciIDsFiles are the usual locations of the PCI ID database.
var pciIDsFiles = []string{"/usr/share/misc/pci.ids", "/usr/share/hwdata/pci.ids", "/usr/share/pci.ids"}

// NICInventory reports the network interfaces of the host. It keeps the
// details of every interface it has seen and is safe for concurrent use.
type NICInventory struct {
	config
	ethtool func(ctx context.Context, args ...string) ([]byte, error)
	pciIDs  []string

	mu      sync.Mutex
	details map[string]NICDetails
}

// NewNICInventory creates a NICInventory; of the options, WithRoot applies.
func NewNICInventory(opts ...Option) *NICInventory {
	return &NICInventory{
		config:  newConfig(opts),
		ethtool: runEthtool,
		pciIDs:  pciIDsFiles,
		details: map[string]NICDetails{},
	}
}

// ErrNoEthtool is returned by the ethtool runner if it is not installed.
var ErrNoEthtool = errors.New("ethtool is not installed")

func runEthtool(ctx context.Context, args ...string) ([]byte, error) {
	executable, err := exec.LookPath("ethtool")
	if err != nil {
		return nil, ErrNoEthtool
	}
	ctx, cancel := context.WithTimeout(ctx, ethtoolTimeout)
	defer cancel()
	// ethtool fails if only some of the settings could be read, e.g.
	// with "netlink error: Operation not permitted" for those an
	// unprivileged user may not read, but still prints the others, which
	// the tolerant parsers make use of.
	out, err := exec.CommandContext(ctx, executable, args...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(out) > 0 {
		return out, nil
	}
	return out, err
}

// EthtoolAvailable reports whether ethtool is installed.
func EthtoolAvailable() bool {
	_, err := exec.LookPath("ethtool")
	return err == nil
}

// names lists the network interfaces, without loopback.
func (inv *NICInventory) names() []string {
	entries, err := os.ReadDir(inv.path("/sys/class/net"))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != "lo" {
			names = append(names, entry.Name())
		}
	}
	return names
}

// Discover reads the details of every interface not discovered yet. It is
// called at startup, and by Interfaces for interfaces created later.
func (inv *NICInventory) Discover(ctx context.Context) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	names := inv.names()
	var missing []string
	for _, name := range names {
		known, ok := inv.details[name]
		if !ok || known.index != atoi(inv.read("/sys/class/net/"+name+"/ifindex")) {
			missing = append(missing, name)
		}
	}
	// Forget removed interfaces, so a later one of the same name is
	// discovered anew.
	for name := range inv.details {
		if !slices.Contains(names, name) {
			delete(inv.details, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	found := make([]NICDetails, len(missing))
	parallel(len(missing), func(i int) { found[i] = inv.discover(ctx, missing[i]) })
	inv.resolvePCINames(found)
	for _, details := range found {
		inv.details[details.Name] = details
	}
}

// Interfaces returns every interface with its details and current state,
// physical interfaces first.
func (inv *NICInventory) Interfaces(ctx context.Context) []NIC {
	inv.Discover(ctx)
	inv.mu.Lock()
	details := make([]NICDetails, 0, len(inv.details))
	for _, d := range inv.details {
		details = append(details, d)
	}
	inv.mu.Unlock()

	slices.SortFunc(details, func(a, b NICDetails) int {
		if a.Physical != b.Physical {
			if a.Physical {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	nics := make([]NIC, len(details))
	parallel(len(details), func(i int) {
		nics[i] = NIC{NICDetails: details[i], NICState: inv.state(ctx, details[i].Name)}
	})
	return nics
}

// parallel runs fn for 0..n-1 with at most nicConcurrency at a time.
func parallel(n int, fn func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, nicConcurrency)
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}()
	}
	wg.Wait()
}

// pciAddressPattern matches a PCI address like "0000:01:00.0".
var pciAddressPattern = regexp.MustCompile(`^[0-9a-f]{4,}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)

func (inv *NICInventory) discover(ctx context.Context, name string) NICDetails {
	netDir := "/sys/class/net/" + name + "/"
	d := NICDetails{Name: name, index: atoi(inv.read(netDir + "ifindex"))}

	// A physical interface has a device. Its sysfs path ends with the PCI
	// address, or with a device on top of it (e.g. "virtio2" below the
	// PCI device of a virtio NIC).
	if device, err := filepath.EvalSymlinks(inv.path(netDir + "device")); err == nil {
		d.Physical = true
		d.Kind = filepath.Base(readLink(filepath.Join(device, "subsystem")))
		if driver := readLink(filepath.Join(device, "driver")); driver != "" {
			d.Driver = filepath.Base(driver)
		}
		for dir := device; dir != "/" && dir != "." && dir != inv.root; dir = filepath.Dir(dir) {
			if pciAddressPattern.MatchString(filepath.Base(dir)) {
				d.Kind = "pci"
				d.PCI = inv.pciInfo(filepath.Base(dir))
				break
			}
		}
		if d.Kind == "" {
			d.Kind = "device"
		}
	}

	// ethtool -i: "driver: i40e", "version: ...", "firmware-version: ...".
	if out, err := inv.ethtool(ctx, "-i", name); err == nil {
		values := parseKeyValues(out)
		if v := values["driver"]; v != "" {
			d.Driver = v
		}
		d.DriverVersion = values["version"]
		d.Firmware = values["firmware-version"]
	}
	if d.Physical {
		if out, err := inv.ethtool(ctx, "-P", name); err == nil {
			mac := parseKeyValues(out)["permanent address"]
			if mac != "" && mac != "00:00:00:00:00:00" {
				d.PermanentMAC = mac
			}
		}
	} else {
		d.Kind = inv.virtualKind(name, d.Driver)
	}
	return d
}

// virtualKind names the type of a virtual interface: the device type the
// kernel reports (bridge, vlan, bond, ...), else the driver (veth, tun,
// dummy, ...).
func (inv *NICInventory) virtualKind(name, driver string) string {
	for line := range strings.SplitSeq(inv.read("/sys/class/net/"+name+"/uevent"), "\n") {
		if v, ok := strings.CutPrefix(line, "DEVTYPE="); ok && v != "" {
			return v
		}
	}
	switch driver {
	case "":
		return "virtual"
	case "802.1Q VLAN Support":
		return "vlan"
	}
	return driver
}

func readLink(p string) string {
	target, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	return target
}

func (inv *NICInventory) pciInfo(address string) *PCIInfo {
	dir := "/sys/bus/pci/devices/" + address + "/"
	numa := -1
	if v, err := strconv.Atoi(inv.read(dir + "numa_node")); err == nil {
		numa = v
	}
	info := &PCIInfo{
		Address:      address,
		Vendor:       strings.TrimPrefix(inv.read(dir+"vendor"), "0x"),
		Device:       strings.TrimPrefix(inv.read(dir+"device"), "0x"),
		NUMANode:     numa,
		LinkSpeed:    knownLinkValue(inv.read(dir + "current_link_speed")),
		LinkWidth:    atoi(inv.read(dir + "current_link_width")),
		MaxLinkSpeed: knownLinkValue(inv.read(dir + "max_link_speed")),
		MaxLinkWidth: atoi(inv.read(dir + "max_link_width")),
		SRIOVTotal:   atoi(inv.read(dir + "sriov_totalvfs")),
	}
	info.LinkGen = PCIeGeneration(info.LinkSpeed)
	info.MaxLinkGen = PCIeGeneration(info.MaxLinkSpeed)
	return info
}

// knownLinkValue drops the placeholders sysfs reports for devices without
// a PCIe link (e.g. "Unknown" or "255" within VMs).
func knownLinkValue(v string) string {
	if strings.Contains(v, "GT/s") {
		return v
	}
	return ""
}

// PCIeGeneration maps the transfer rate per lane ("8.0 GT/s PCIe") to the
// PCIe generation, 0 if unknown.
func PCIeGeneration(speed string) int {
	fields := strings.Fields(speed)
	if len(fields) == 0 {
		return 0
	}
	rate, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	for gen, gts := range []float64{2.5, 5, 8, 16, 32, 64} {
		if rate == gts {
			return gen + 1
		}
	}
	return 0
}

// resolvePCINames fills in the vendor and device names from pci.ids. The
// file is read only up to the last vendor needed.
func (inv *NICInventory) resolvePCINames(details []NICDetails) {
	wanted := map[string]map[string]bool{}
	for _, d := range details {
		if d.PCI != nil && d.PCI.Vendor != "" {
			if wanted[d.PCI.Vendor] == nil {
				wanted[d.PCI.Vendor] = map[string]bool{}
			}
			wanted[d.PCI.Vendor][d.PCI.Device] = true
		}
	}
	if len(wanted) == 0 {
		return
	}
	vendors, devices := map[string]string{}, map[[2]string]string{}
	for _, file := range inv.pciIDs {
		f, err := os.Open(inv.path(file))
		if err != nil {
			continue
		}
		vendors, devices = parsePCIIDs(f, wanted)
		_ = f.Close()
		break
	}
	for i := range details {
		if pci := details[i].PCI; pci != nil {
			pci.VendorName = vendors[pci.Vendor]
			pci.DeviceName = devices[[2]string{pci.Vendor, pci.Device}]
		}
	}
}

// parsePCIIDs reads the names of the wanted vendors and devices from the
// pci.ids format: "vvvv  Vendor" lines, each followed by "\tdddd  Device"
// lines (and "\t\t" subsystem lines, which are not needed).
func parsePCIIDs(f *os.File, wanted map[string]map[string]bool) (map[string]string, map[[2]string]string) {
	vendors, devices := map[string]string{}, map[[2]string]string{}
	scanner := bufio.NewScanner(f)
	vendor := ""
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' || strings.HasPrefix(line, "\t\t") {
			continue
		}
		if line[0] == '\t' {
			id, name, ok := strings.Cut(strings.TrimPrefix(line, "\t"), "  ")
			if ok && vendor != "" && wanted[vendor][id] {
				devices[[2]string{vendor, id}] = strings.TrimSpace(name)
			}
			continue
		}
		// The device classes ("C 02  Network controller") follow
		// all vendors.
		if strings.HasPrefix(line, "C ") {
			break
		}
		id, name, ok := strings.Cut(line, "  ")
		vendor = ""
		if ok && wanted[id] != nil {
			vendor = id
			vendors[id] = strings.TrimSpace(name)
		}
	}
	return vendors, devices
}

// state reads the current state of an interface.
func (inv *NICInventory) state(ctx context.Context, name string) NICState {
	netDir := "/sys/class/net/" + name + "/"
	s := NICState{
		MAC:       inv.read(netDir + "address"),
		MTU:       atoi(inv.read(netDir + "mtu")),
		OperState: inv.read(netDir + "operstate"),
	}
	// carrier, speed and duplex fail to read while the interface is
	// down; speed is -1 if unknown.
	if v, err := strconv.Atoi(inv.read(netDir + "carrier")); err == nil {
		carrier := v == 1
		s.Carrier = &carrier
	}
	if v, err := strconv.Atoi(inv.read(netDir + "speed")); err == nil && v > 0 {
		s.Speed = &v
	}
	if v := inv.read(netDir + "duplex"); v != "" && v != "unknown" {
		s.Duplex = v
	}
	if entries, err := os.ReadDir(inv.path(netDir + "queues")); err == nil {
		for _, entry := range entries {
			switch {
			case strings.HasPrefix(entry.Name(), "rx-"):
				s.RXQueues++
			case strings.HasPrefix(entry.Name(), "tx-"):
				s.TXQueues++
			}
		}
	}

	// ethtool <name>: port and auto-negotiation are not in sysfs.
	if out, err := inv.ethtool(ctx, name); err == nil {
		values := parseKeyValues(out)
		if v := values["port"]; v != "" && v != "Other" {
			s.Port = v
		}
		if v := values["auto-negotiation"]; v == "on" || v == "off" {
			s.AutoNeg = v
		}
		if s.Speed == nil {
			if v, err := strconv.Atoi(strings.TrimSuffix(values["speed"], "Mb/s")); err == nil && v > 0 {
				s.Speed = &v
			}
		}
	}
	if out, err := inv.ethtool(ctx, "-l", name); err == nil {
		s.Channels = parseChannels(out)
	}
	if out, err := inv.ethtool(ctx, "-g", name); err == nil {
		s.Rings = parseRings(out)
	}
	return s
}

// parseKeyValues parses "key: value" lines into a map with lower case keys.
// Lines without a colon (headings, link mode lists) are skipped.
func parseKeyValues(out []byte) map[string]string {
	values := map[string]string{}
	for line := range strings.SplitSeq(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, seen := values[key]; !seen {
			values[key] = strings.TrimSpace(value)
		}
	}
	return values
}

// parseMaxCurrent parses the output of "ethtool -l" and "ethtool -g", which
// list "key: count" lines under "Pre-set maximums:" and "Current hardware
// settings:", into one map per section with lower case keys. A count of
// "n/a" is nil. Newer ethtool versions add settings that are no counts
// (e.g. "TX push: off") after these, which are skipped.
func parseMaxCurrent(out []byte) (maxCounts, current map[string]*int, ok bool) {
	maxCounts, current = map[string]*int{}, map[string]*int{}
	var section map[string]*int
	for line := range strings.SplitSeq(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Pre-set maximums"):
			section, ok = maxCounts, true
			continue
		case strings.HasPrefix(trimmed, "Current hardware settings"):
			section, ok = current, true
			continue
		}
		if section == nil {
			continue
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if v, err := strconv.Atoi(value); err == nil {
			section[strings.ToLower(key)] = &v
		} else if value == "n/a" {
			section[strings.ToLower(key)] = nil
		}
	}
	return maxCounts, current, ok
}

// parseChannels parses the output of "ethtool -l", nil if not found.
func parseChannels(out []byte) *Channels {
	maxCounts, current, ok := parseMaxCurrent(out)
	if !ok {
		return nil
	}
	counts := func(m map[string]*int) ChannelCounts {
		return ChannelCounts{RX: m["rx"], TX: m["tx"], Other: m["other"], Combined: m["combined"]}
	}
	return &Channels{Max: counts(maxCounts), Current: counts(current)}
}

// parseRings parses the output of "ethtool -g", nil if not found.
func parseRings(out []byte) *Rings {
	maxCounts, current, ok := parseMaxCurrent(out)
	if !ok {
		return nil
	}
	counts := func(m map[string]*int) RingCounts {
		return RingCounts{RX: m["rx"], RXMini: m["rx mini"], RXJumbo: m["rx jumbo"], TX: m["tx"]}
	}
	return &Rings{Max: counts(maxCounts), Current: counts(current)}
}
