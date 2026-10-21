// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package host

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseCPUList(t *testing.T) {
	tests := []struct {
		list string
		want []int
	}{
		{"", nil},
		{"0", []int{0}},
		{"0-3", []int{0, 1, 2, 3}},
		{"0-1,8,10-11\n", []int{0, 1, 8, 10, 11}},
		{"4,0-1,1", []int{0, 1, 4}},
		{"x,2,3-1,5-y", []int{2}},
	}
	for _, tt := range tests {
		t.Run(tt.list, func(t *testing.T) {
			require.Equal(t, tt.want, ParseCPUList(tt.list))
		})
	}
}

func TestConfigArgument(t *testing.T) {
	require.Equal(t, "/var/bngblaster/a/config.json",
		configArgument([]byte("/usr/bin/bngblaster\x00-C\x00/var/bngblaster/a/config.json\x00-S\x00run.sock\x00")))
	require.Equal(t, "b.json", configArgument([]byte("bngblaster\x00--config=b.json\x00")))
	require.Empty(t, configArgument([]byte("bngblaster\x00-C\x00")))
}

func TestUnescapeMount(t *testing.T) {
	require.Equal(t, "/mnt/my disk", unescapeMount(`/mnt/my\040disk`))
	require.Equal(t, "/data", unescapeMount("/data"))
}

// writeFiles creates the files below root, creating folders as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	}
}

// fixtureHost builds a host with two sockets, one NUMA node per socket and
// two cores with two threads each per socket:
//
//	node 0: CPUs 0,1 (cores) and 4,5 (siblings)
//	node 1: CPUs 2,3 (cores) and 6,7 (siblings), CPU 7 offline
func fixtureHost(t *testing.T) (root, lockFolder string) {
	t.Helper()
	root = t.TempDir()
	lockFolder = t.TempDir()
	files := map[string]string{
		"/proc/sys/kernel/hostname":  "blaster\n",
		"/proc/sys/kernel/osrelease": "6.8.0\n",
		"/proc/uptime":               "3600.50 100.00\n",
		"/proc/loadavg":              "1.50 0.75 0.25 2/300 1234\n",
		"/proc/cpuinfo":              "processor\t: 0\nmodel name\t: AMD EPYC 9754 128-Core Processor\n",
		"/proc/meminfo": "MemTotal:       16384 kB\nMemFree:         4096 kB\nMemAvailable:    8192 kB\n" +
			"SwapTotal:          0 kB\nSwapFree:           0 kB\nHugePages_Total:     4\nHugePages_Free:      2\n" +
			"Hugepagesize:    2048 kB\n",
		"/proc/mounts": "/dev/sda1 / ext4 rw 0 0\n" +
			"/dev/sda1 /var/lib/docker ext4 rw 0 0\n" +
			"/dev/sdb1 /mnt/my\\040data xfs rw 0 0\n" +
			"/dev/loop0 /snap/core squashfs ro 0 0\n" +
			"tmpfs /run tmpfs rw 0 0\n" +
			"server:/export /nfs nfs4 rw 0 0\n",
		"/sys/devices/system/cpu/present":        "0-7\n",
		"/sys/devices/system/cpu/online":         "0-6\n",
		"/sys/devices/system/cpu/isolated":       "2-3,6\n",
		"/sys/devices/system/node/online":        "0-1\n",
		"/sys/devices/system/node/node0/cpulist": "0-1,4-5\n",
		"/sys/devices/system/node/node1/cpulist": "2-3,6-7\n",
		"/sys/devices/system/node/node0/meminfo": "Node 0 MemTotal:        8192 kB\nNode 0 MemFree:         2048 kB\n",
		"/sys/devices/system/node/node1/meminfo": "Node 1 MemTotal:        8192 kB\nNode 1 MemFree:         2048 kB\n",
		// Running instance 100 of the controller and a manually started
		// bngblaster 200; process 300 is gone.
		"/proc/100/cmdline": "/usr/bin/bngblaster\x00-C\x00/var/bngblaster/a/config.json\x00",
		"/proc/200/cmdline": "bngblaster\x00-C\x00/home/user/test.json\x00",
	}
	for cpu := range 8 {
		dir := "/sys/devices/system/cpu/cpu" + strconv.Itoa(cpu) + "/topology/"
		files[dir+"physical_package_id"] = strconv.Itoa(cpu % 4 / 2)
		files[dir+"core_id"] = strconv.Itoa(cpu % 2)
	}
	writeFiles(t, root, files)
	writeFiles(t, lockFolder, map[string]string{
		"bngblaster_cpu2.100.lock":   "100\n",
		"bngblaster_cpu3.100.lock":   "100\n",
		"bngblaster_cpu3.200.lock":   "200\n",
		"bngblaster_cpu6.300.lock":   "300\n",
		"bngblaster_startup.lock":    "100\n",
		"bngblaster_eth1.lock":       "100\n",
		"bngblaster_cpu5.100.lock.x": "100\n",
	})
	return root, lockFolder
}

func TestCollector_Collect(t *testing.T) {
	root, lockFolder := fixtureHost(t)
	c := NewCollector(WithRoot(root), WithLockFolder(lockFolder))
	c.statfs = func(path string, buf *syscall.Statfs_t) error {
		buf.Bsize = 4096
		buf.Blocks = 1000
		buf.Bfree = 400
		buf.Bavail = 300
		return nil
	}
	c.sleep = func(time.Duration) {}

	info := c.Collect()
	require.Equal(t, "blaster", info.Hostname)
	require.Equal(t, "6.8.0", info.Kernel)
	require.InDelta(t, 3600.5, info.Uptime, 0.001)
	require.Equal(t, []float64{1.5, 0.75, 0.25}, info.LoadAverage)

	require.Equal(t, CPUSummary{
		Model:    "AMD EPYC 9754 128-Core Processor",
		Sockets:  2,
		Cores:    4,
		Threads:  7,
		Isolated: 3,
		Pinned:   2,
	}, info.CPU)

	require.Equal(t, Memory{
		Total:          16384 * 1024,
		Available:      8192 * 1024,
		Used:           8192 * 1024,
		HugePagesTotal: 4,
		HugePagesFree:  2,
		HugePageSize:   2048 * 1024,
	}, info.Memory)

	require.Equal(t, []NUMANode{
		{ID: 0, CPUs: []int{0, 1, 4, 5}, MemoryTotal: 8192 * 1024, MemoryFree: 2048 * 1024},
		{ID: 1, CPUs: []int{2, 3, 6, 7}, MemoryTotal: 8192 * 1024, MemoryFree: 2048 * 1024},
	}, info.NUMANodes)

	require.Len(t, info.CPUs, 8)
	require.Equal(t, CPU{ID: 0, Online: true, Node: 0, Socket: 0, Core: 0}, info.CPUs[0])
	require.Equal(t, CPU{
		ID: 3, Online: true, Node: 1, Socket: 1, Core: 1, Isolated: true,
		Pins: []Pin{{PID: 100, Config: "/var/bngblaster/a/config.json"}, {PID: 200, Config: "/home/user/test.json"}},
	}, info.CPUs[3])
	// The lock file of the gone process 300 does not count.
	require.Equal(t, CPU{ID: 6, Online: true, Node: 1, Socket: 1, Core: 0, Isolated: true}, info.CPUs[6])
	require.False(t, info.CPUs[7].Online)

	// One entry per device, without loop devices, tmpfs and NFS.
	require.Equal(t, []Disk{
		{Mount: "/", Device: "/dev/sda1", FSType: "ext4", Total: 4096000, Used: 2457600, Available: 1228800},
		{Mount: "/mnt/my data", Device: "/dev/sdb1", FSType: "xfs", Total: 4096000, Used: 2457600, Available: 1228800},
	}, info.Disks)
}

func TestCollector_cpuUsage(t *testing.T) {
	root := t.TempDir()
	stat := func(cpu0Busy, cpu0Idle int) {
		writeFiles(t, root, map[string]string{
			"/proc/stat": "cpu  " + strconv.Itoa(cpu0Busy) + " 0 0 " + strconv.Itoa(cpu0Idle) + " 0 0 0 0 0 0\n" +
				"cpu0 " + strconv.Itoa(cpu0Busy) + " 0 0 " + strconv.Itoa(cpu0Idle) + " 0 0 0 0 0 0\n",
		})
	}
	now := time.Unix(1000, 0)
	c := NewCollector(WithRoot(root))
	c.now = func() time.Time { return now }
	// Without a previous sample, a second one is taken after a short wait.
	stat(100, 100)
	c.sleep = func(d time.Duration) {
		now = now.Add(d)
		stat(125, 175) // +25 busy, +75 idle
	}
	usage, total := c.cpuUsage()
	require.InDelta(t, 25, total, 0.001)
	require.InDelta(t, 25, usage[0], 0.001)

	// Later calls use the previous sample, without waiting.
	c.sleep = func(time.Duration) { t.Fatal("unexpected sample wait") }
	now = now.Add(2 * time.Second)
	stat(175, 225) // +50 busy, +50 idle
	_, total = c.cpuUsage()
	require.InDelta(t, 50, total, 0.001)

	// Calls in quick succession share the last result.
	now = now.Add(100 * time.Millisecond)
	stat(275, 225)
	_, total = c.cpuUsage()
	require.InDelta(t, 50, total, 0.001)
}
