// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.

// Package host reports information about the host the controller runs on:
// CPU, NUMA and memory layout, live CPU, memory and disk usage, and which
// CPUs are pinned by bngblaster instances.
//
// Everything is read from /proc and /sys (Linux only), relative to a root
// folder so that tests can run against a fixture tree.
package host

import (
	"bufio"
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultLockFolder is where bngblaster creates its lock files.
const DefaultLockFolder = "/run/lock"

// Info is the host information reported by Collector.Collect.
type Info struct {
	Hostname    string     `json:"hostname"`
	Kernel      string     `json:"kernel"`
	Uptime      float64    `json:"uptime"`
	LoadAverage []float64  `json:"load_average"`
	CPU         CPUSummary `json:"cpu"`
	Memory      Memory     `json:"memory"`
	NUMANodes   []NUMANode `json:"numa_nodes"`
	CPUs        []CPU      `json:"cpus"`
	Disks       []Disk     `json:"disks"`
}

// CPUSummary describes the processors of the host.
type CPUSummary struct {
	Model   string `json:"model"`
	Sockets int    `json:"sockets"`
	Cores   int    `json:"cores"`
	// Threads is the number of online logical CPUs.
	Threads int `json:"threads"`
	// Usage is the total CPU usage in percent over all online CPUs.
	Usage    float64 `json:"usage"`
	Isolated int     `json:"isolated"`
	// Pinned is the number of CPUs pinned by at least one instance.
	Pinned int `json:"pinned"`
}

// CPU is one logical CPU.
type CPU struct {
	ID       int  `json:"id"`
	Online   bool `json:"online"`
	Node     int  `json:"node"`
	Socket   int  `json:"socket"`
	Core     int  `json:"core"`
	Isolated bool `json:"isolated"`
	// Usage in percent since the previous sample.
	Usage float64 `json:"usage"`
	// Pins lists the bngblaster processes holding a lock on this CPU.
	Pins []Pin `json:"pins,omitempty"`
}

// Pin is a bngblaster process using a CPU, as recorded by its CPU lock
// file "bngblaster_cpu<N>.<PID>.lock".
type Pin struct {
	PID int `json:"pid"`
	// Config is the configuration file passed with -C, which identifies
	// the instance.
	Config string `json:"-"`
	// Instance is the controller instance of the process, empty for a
	// bngblaster started outside of this controller.
	Instance string `json:"instance,omitempty"`
}

// Memory in bytes, from /proc/meminfo.
type Memory struct {
	Total          uint64 `json:"total"`
	Available      uint64 `json:"available"`
	Used           uint64 `json:"used"`
	SwapTotal      uint64 `json:"swap_total"`
	SwapFree       uint64 `json:"swap_free"`
	HugePagesTotal uint64 `json:"hugepages_total"`
	HugePagesFree  uint64 `json:"hugepages_free"`
	HugePageSize   uint64 `json:"hugepage_size"`
}

// NUMANode is one NUMA node with its CPUs and memory in bytes.
type NUMANode struct {
	ID          int    `json:"id"`
	CPUs        []int  `json:"cpus"`
	MemoryTotal uint64 `json:"memory_total"`
	MemoryFree  uint64 `json:"memory_free"`
}

// Disk is a mounted block device file system, sizes in bytes as reported
// by df: Used + Available is less than Total by the reserved blocks.
type Disk struct {
	Mount     string `json:"mount"`
	Device    string `json:"device"`
	FSType    string `json:"fstype"`
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

// Collector collects Info. It keeps the previous CPU time sample, from
// which the usage is derived, and is safe for concurrent use.
type Collector struct {
	root       string
	lockFolder string
	statfs     func(path string, buf *syscall.Statfs_t) error
	now        func() time.Time
	sleep      func(time.Duration)

	mu       sync.Mutex
	prev     map[int]cpuTimes
	prevAt   time.Time
	usage    map[int]float64
	total    float64
	usageSet bool
}

// Option configures a Collector.
type Option func(*Collector)

// WithRoot reads /proc and /sys below root instead of "/".
func WithRoot(root string) Option {
	return func(c *Collector) { c.root = root }
}

// WithLockFolder sets the folder of the bngblaster lock files,
// DefaultLockFolder by default.
func WithLockFolder(folder string) Option {
	return func(c *Collector) { c.lockFolder = folder }
}

// NewCollector creates a Collector.
func NewCollector(opts ...Option) *Collector {
	c := &Collector{
		root:       "/",
		lockFolder: DefaultLockFolder,
		statfs:     syscall.Statfs,
		now:        time.Now,
		sleep:      time.Sleep,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Collector) path(p string) string {
	return filepath.Join(c.root, p)
}

func (c *Collector) read(p string) string {
	data, err := os.ReadFile(c.path(p))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Collect gathers the current host information.
func (c *Collector) Collect() Info {
	info := Info{
		Hostname: c.read("/proc/sys/kernel/hostname"),
		Kernel:   c.read("/proc/sys/kernel/osrelease"),
		Memory:   c.memory(),
		Disks:    c.disks(),
	}
	if fields := strings.Fields(c.read("/proc/uptime")); len(fields) > 0 {
		info.Uptime, _ = strconv.ParseFloat(fields[0], 64)
	}
	if fields := strings.Fields(c.read("/proc/loadavg")); len(fields) >= 3 {
		for _, f := range fields[:3] {
			v, _ := strconv.ParseFloat(f, 64)
			info.LoadAverage = append(info.LoadAverage, v)
		}
	}
	info.CPUs, info.CPU = c.cpus()
	info.NUMANodes = c.numaNodes(info.CPUs, info.Memory)
	return info
}

// cpus reads the CPU topology, usage and pinning.
func (c *Collector) cpus() ([]CPU, CPUSummary) {
	var summary CPUSummary
	summary.Model = c.cpuModel()

	present := ParseCPUList(c.read("/sys/devices/system/cpu/present"))
	online := toSet(ParseCPUList(c.read("/sys/devices/system/cpu/online")))
	isolated := toSet(ParseCPUList(c.read("/sys/devices/system/cpu/isolated")))
	if len(present) == 0 {
		present = slices.Sorted(maps.Keys(online))
	}
	nodeOf := c.cpuNodes()
	usage, total := c.cpuUsage()
	pins := c.pins()

	sockets := map[int]bool{}
	cores := map[[2]int]bool{}
	cpus := make([]CPU, 0, len(present))
	for _, id := range present {
		dir := "/sys/devices/system/cpu/cpu" + strconv.Itoa(id) + "/topology/"
		cpu := CPU{
			ID:       id,
			Online:   online[id],
			Node:     nodeOf[id],
			Socket:   atoi(c.read(dir + "physical_package_id")),
			Core:     atoi(c.read(dir + "core_id")),
			Isolated: isolated[id],
			Usage:    usage[id],
			Pins:     pins[id],
		}
		if cpu.Online {
			summary.Threads++
			sockets[cpu.Socket] = true
			cores[[2]int{cpu.Socket, cpu.Core}] = true
		}
		if cpu.Isolated {
			summary.Isolated++
		}
		if len(cpu.Pins) > 0 {
			summary.Pinned++
		}
		cpus = append(cpus, cpu)
	}
	summary.Sockets = len(sockets)
	summary.Cores = len(cores)
	summary.Usage = total
	return cpus, summary
}

func (c *Collector) cpuModel() string {
	data, err := os.ReadFile(c.path("/proc/cpuinfo"))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "model name", "Model", "cpu model":
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// cpuNodes maps each CPU to its NUMA node; CPUs of a host without NUMA
// information are all on node 0.
func (c *Collector) cpuNodes() map[int]int {
	nodeOf := map[int]int{}
	for _, node := range c.nodeIDs() {
		for _, cpu := range ParseCPUList(c.read(nodeDir(node) + "cpulist")) {
			nodeOf[cpu] = node
		}
	}
	return nodeOf
}

func nodeDir(node int) string {
	return "/sys/devices/system/node/node" + strconv.Itoa(node) + "/"
}

func (c *Collector) nodeIDs() []int {
	return ParseCPUList(c.read("/sys/devices/system/node/online"))
}

func (c *Collector) numaNodes(cpus []CPU, memory Memory) []NUMANode {
	ids := c.nodeIDs()
	if len(ids) == 0 {
		// Kernel without NUMA support: one node with everything.
		node := NUMANode{MemoryTotal: memory.Total, MemoryFree: memory.Available}
		for _, cpu := range cpus {
			node.CPUs = append(node.CPUs, cpu.ID)
		}
		return []NUMANode{node}
	}
	nodes := make([]NUMANode, 0, len(ids))
	for _, id := range ids {
		node := NUMANode{ID: id, CPUs: ParseCPUList(c.read(nodeDir(id) + "cpulist"))}
		// "Node 0 MemTotal:       24609312 kB"
		for _, line := range strings.Split(c.read(nodeDir(id)+"meminfo"), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			value, _ := strconv.ParseUint(fields[3], 10, 64)
			switch fields[2] {
			case "MemTotal:":
				node.MemoryTotal = value * 1024
			case "MemFree:":
				node.MemoryFree = value * 1024
			}
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func (c *Collector) memory() Memory {
	values := map[string]uint64{}
	for _, line := range strings.Split(c.read("/proc/meminfo"), "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, _ := strconv.ParseUint(fields[0], 10, 64)
		if len(fields) > 1 && fields[1] == "kB" {
			value *= 1024
		}
		values[key] = value
	}
	m := Memory{
		Total:          values["MemTotal"],
		Available:      values["MemAvailable"],
		SwapTotal:      values["SwapTotal"],
		SwapFree:       values["SwapFree"],
		HugePagesTotal: values["HugePages_Total"],
		HugePagesFree:  values["HugePages_Free"],
		HugePageSize:   values["Hugepagesize"],
	}
	if m.Available == 0 {
		// Kernels before 3.14 do not report MemAvailable.
		m.Available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if m.Total > m.Available {
		m.Used = m.Total - m.Available
	}
	return m
}

// cpuTimes are the busy and total jiffies of one CPU from /proc/stat.
type cpuTimes struct {
	busy, total uint64
}

// totalCPU is the key of the aggregated "cpu" line of /proc/stat.
const totalCPU = -1

func (c *Collector) readCPUTimes() map[int]cpuTimes {
	times := map[int]cpuTimes{}
	for _, line := range strings.Split(c.read("/proc/stat"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		id := totalCPU
		if fields[0] != "cpu" {
			n, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu"))
			if err != nil {
				continue
			}
			id = n
		}
		var t cpuTimes
		// user nice system idle iowait irq softirq steal; guest and
		// guest_nice are already included in user and nice.
		for i, f := range fields[1:min(len(fields), 9)] {
			v, _ := strconv.ParseUint(f, 10, 64)
			t.total += v
			if i != 3 && i != 4 { // idle, iowait
				t.busy += v
			}
		}
		times[id] = t
	}
	return times
}

const (
	// usageMinInterval is the shortest interval a usage is derived from;
	// requests in between (several viewers) share the last result, as
	// a few jiffies are too few for a meaningful percentage.
	usageMinInterval = 500 * time.Millisecond
	// usageMaxAge is the oldest previous sample still used; after that,
	// two fresh samples usageSampleInterval apart are taken instead of
	// reporting the average since the last viewer has left.
	usageMaxAge         = 30 * time.Second
	usageSampleInterval = 250 * time.Millisecond
)

// cpuUsage returns the usage in percent per CPU and in total, derived
// from the CPU times since the previous call.
func (c *Collector) cpuUsage() (map[int]float64, float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	age := now.Sub(c.prevAt)
	if c.usageSet && age < usageMinInterval {
		return c.usage, c.total
	}
	if c.prev == nil || age > usageMaxAge {
		c.prev = c.readCPUTimes()
		c.sleep(usageSampleInterval)
		now = c.now()
	}
	current := c.readCPUTimes()
	usage := map[int]float64{}
	for id, cur := range current {
		prev, ok := c.prev[id]
		if !ok || cur.total <= prev.total {
			continue
		}
		busy := float64(cur.busy-prev.busy) / float64(cur.total-prev.total) * 100
		usage[id] = min(100, max(0, busy))
	}
	c.prev, c.prevAt = current, now
	c.usage, c.total, c.usageSet = usage, usage[totalCPU], true
	return c.usage, c.total
}

// cpuLockPattern matches "bngblaster_cpu<N>.<PID>.lock".
var cpuLockPattern = regexp.MustCompile(`^bngblaster_cpu(\d+)\.(\d+)\.lock$`)

// pins maps CPUs to the bngblaster processes using them, based on their
// CPU lock files.
//
// A lock file is left behind if its instance was killed. Whether the lock
// is still held is deliberately not tested with flock(): even a shared
// lock held for a moment makes an instance locking the same file at that
// time fail. Instead, a file counts only if its process still exists;
// bngblaster removes stale files on its next startup.
func (c *Collector) pins() map[int][]Pin {
	entries, err := os.ReadDir(c.lockFolder)
	if err != nil {
		return nil
	}
	type process struct {
		alive  bool
		config string
	}
	processes := map[int]process{}
	pins := map[int][]Pin{}
	for _, entry := range entries {
		match := cpuLockPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		cpu, _ := strconv.Atoi(match[1])
		pid, _ := strconv.Atoi(match[2])
		proc, ok := processes[pid]
		if !ok {
			cmdline, err := os.ReadFile(c.path("/proc/" + strconv.Itoa(pid) + "/cmdline"))
			proc = process{alive: err == nil, config: configArgument(cmdline)}
			processes[pid] = proc
		}
		if proc.alive {
			pins[cpu] = append(pins[cpu], Pin{PID: pid, Config: proc.config})
		}
	}
	for _, list := range pins {
		sort.Slice(list, func(i, j int) bool { return list[i].PID < list[j].PID })
	}
	return pins
}

// configArgument returns the value of -C from a NUL separated command line.
func configArgument(cmdline []byte) string {
	args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
	for i, arg := range args {
		if arg == "-C" && i+1 < len(args) {
			return args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "--config="); ok {
			return value
		}
	}
	return ""
}

// pseudoDevices are file systems with a /dev device that are no disks.
var pseudoDevices = []string{"/dev/loop", "/dev/ram", "/dev/zram"}

// disks lists the file systems of block devices, one per device.
//
// Only devices below /dev are reported: tmpfs, overlay and the like are
// no disks, and statfs() on a network file system can block for a long
// time when its server is unreachable. Loop devices are mostly read-only
// squashfs images (snaps) that are always full.
func (c *Collector) disks() []Disk {
	data, err := os.ReadFile(c.path("/proc/mounts"))
	if err != nil {
		return nil
	}
	byDevice := map[string]Disk{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], unescapeMount(fields[1]), fields[2]
		if !strings.HasPrefix(device, "/dev/") || fstype == "squashfs" {
			continue
		}
		if slices.ContainsFunc(pseudoDevices, func(p string) bool { return strings.HasPrefix(device, p) }) {
			continue
		}
		// The same device mounted several times (bind mounts, btrfs
		// subvolumes): report the shortest mount point only.
		if prev, ok := byDevice[device]; ok && len(prev.Mount) <= len(mount) {
			continue
		}
		var st syscall.Statfs_t
		if err := c.statfs(mount, &st); err != nil || st.Blocks == 0 {
			continue
		}
		size := uint64(st.Bsize) //nolint:gosec // block size is positive
		byDevice[device] = Disk{
			Mount:     mount,
			Device:    device,
			FSType:    fstype,
			Total:     st.Blocks * size,
			Used:      (st.Blocks - st.Bfree) * size,
			Available: st.Bavail * size,
		}
	}
	disks := make([]Disk, 0, len(byDevice))
	for _, disk := range byDevice {
		disks = append(disks, disk)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Mount < disks[j].Mount })
	return disks
}

// unescapeMount decodes the octal escapes (e.g. "\040" for a space) of a
// mount point in /proc/mounts.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ParseCPUList parses a kernel CPU list like "0-3,8,10-11" into its
// sorted CPU numbers. Invalid parts are skipped.
func ParseCPUList(list string) []int {
	var cpus []int
	for part := range strings.SplitSeq(strings.TrimSpace(list), ",") {
		if part == "" {
			continue
		}
		from, to, isRange := strings.Cut(part, "-")
		first, err := strconv.Atoi(from)
		if err != nil {
			continue
		}
		last := first
		if isRange {
			if last, err = strconv.Atoi(to); err != nil || last < first {
				continue
			}
		}
		for cpu := first; cpu <= last; cpu++ {
			cpus = append(cpus, cpu)
		}
	}
	slices.Sort(cpus)
	return slices.Compact(cpus)
}

func toSet(list []int) map[int]bool {
	set := make(map[int]bool, len(list))
	for _, v := range list {
		set[v] = true
	}
	return set
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
