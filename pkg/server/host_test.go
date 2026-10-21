// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/host"
)

func TestServer_hostDisabled(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	server := httptest.NewServer(NewServer(repository))
	defer server.Close()
	e := httpexpect.New(t, server.URL)
	e.GET("/api/v1/host").Expect().Status(http.StatusNotFound)
	e.GET("/api/v1/host/topology.png").Expect().Status(http.StatusNotFound)
	e.GET("/api/v1/host/interfaces").Expect().Status(http.StatusNotFound)
}

func TestServer_hostInterfaces(t *testing.T) {
	// One virtual interface; ethtool, if installed, fails for it, which
	// leaves the values read from sysfs.
	root := t.TempDir()
	for name, content := range map[string]string{
		"sys/class/net/bbltest0/ifindex":   "7\n",
		"sys/class/net/bbltest0/mtu":       "1500\n",
		"sys/class/net/bbltest0/operstate": "up\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
	}
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
	}
	s := NewServer(repository, WithHostAPI(true))
	s.nics = host.NewNICInventory(host.WithRoot(root))
	server := httptest.NewServer(s)
	defer server.Close()

	e := httpexpect.New(t, server.URL)
	body := e.GET("/api/v1/host/interfaces").Expect().Status(http.StatusOK).JSON().Object()
	body.Value("ethtool").Boolean()
	nics := body.Value("interfaces").Array()
	nics.Length().Equal(1)
	nic := nics.Element(0).Object()
	nic.Value("name").Equal("bbltest0")
	nic.Value("physical").Equal(false)
	nic.Value("mtu").Equal(1500)
	nic.Value("operstate").Equal("up")
	nic.Value("channels").Null()
	nic.Value("rings").Null()
}

func TestServer_hostInfo(t *testing.T) {
	// A host with two CPUs, where CPU 0 is pinned by the instance "a" of
	// this controller and CPU 1 by a bngblaster started manually.
	root := t.TempDir()
	locks := t.TempDir()
	folder := t.TempDir()
	files := map[string]string{
		filepath.Join(root, "proc/100/cmdline"):              "bngblaster\x00-C\x00" + filepath.Join(folder, "a", "config.json") + "\x00",
		filepath.Join(root, "proc/200/cmdline"):              "bngblaster\x00-C\x00/home/user/config.json\x00",
		filepath.Join(root, "sys/devices/system/cpu/online"): "0-1\n",
		filepath.Join(locks, "bngblaster_cpu0.100.lock"):     "100\n",
		filepath.Join(locks, "bngblaster_cpu1.200.lock"):     "200\n",
	}
	for name, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
	}

	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		ExistsFunc:       func(name string) bool { return name == "a" },
	}
	s := NewServer(repository, WithHostAPI(true))
	s.hostCollector = host.NewCollector(host.WithRoot(root), host.WithLockFolder(locks))
	server := httptest.NewServer(s)
	defer server.Close()

	e := httpexpect.New(t, server.URL)
	body := e.GET("/api/v1/host").Expect().Status(http.StatusOK).JSON().Object()
	body.Value("topology").Boolean()
	cpus := body.Value("cpus").Array()
	cpus.Length().Equal(2)
	cpus.Element(0).Object().Value("pins").Equal([]map[string]any{{"pid": 100, "instance": "a"}})
	cpus.Element(1).Object().Value("pins").Equal([]map[string]any{{"pid": 200}})
	body.Path("$.cpu.pinned").Equal(2)
}
