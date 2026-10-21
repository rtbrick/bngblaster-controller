// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rtbrick/bngblaster-controller/pkg/host"
)

const (
	// topologyCommand renders the hardware topology (hwloc). It is optional:
	// without it, the web UI hides the topology button.
	topologyCommand = "lstopo"
	// topologyTimeout bounds one lstopo run, which takes a few seconds on
	// large hosts.
	topologyTimeout = 30 * time.Second
	// topologyCacheTTL is how long a rendered topology is reused. The
	// hardware rarely changes, but CPUs can be taken offline.
	topologyCacheTTL = 10 * time.Minute
)

// hostInfo is the response of the host endpoint.
type hostInfo struct {
	host.Info
	// Topology reports whether the topology image is available.
	Topology bool `json:"topology"`
}

// topologyCache holds the last rendered topology image. Its mutex also
// serializes the lstopo runs, so concurrent viewers wait for one run
// instead of starting one each.
type topologyCache struct {
	mu      sync.Mutex
	png     []byte
	created time.Time
}

// hostInfo reports CPU, NUMA, memory and disk information of the host and
// which CPUs are pinned by bngblaster instances: GET /api/v1/host
//
// The web UI polls this every few seconds while the host view is open, so
// everything is read from /proc and /sys without running external commands.
func (s *Server) hostInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		info := hostInfo{Info: s.hostCollector.Collect()}
		s.resolvePinnedInstances(&info.Info)
		_, err := exec.LookPath(topologyCommand)
		info.Topology = err == nil

		noCache(w)
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	}
}

// resolvePinnedInstances names the controller instance of every pinned
// CPU. The controller starts each instance with "-C <config folder>/<name>/
// config.json", so a configuration file directly within an instance folder
// identifies the instance; anything else is a bngblaster started outside of
// this controller and is reported by its PID only.
func (s *Server) resolvePinnedInstances(info *host.Info) {
	folder, err := filepath.Abs(s.repository.ConfigFolder())
	if err != nil {
		return
	}
	for i := range info.CPUs {
		for j := range info.CPUs[i].Pins {
			pin := &info.CPUs[i].Pins[j]
			if pin.Config == "" {
				continue
			}
			// A relative path is relative to the working directory of
			// the controller, which started the instance with it.
			config, err := filepath.Abs(pin.Config)
			if err != nil {
				continue
			}
			instanceFolder := filepath.Dir(config)
			if filepath.Dir(instanceFolder) != folder {
				continue
			}
			if name := filepath.Base(instanceFolder); s.repository.Exists(name) {
				pin.Instance = name
			}
		}
	}
}

// hostInterfaces lists the network interfaces of the host with their
// details (driver, firmware, PCI device and link), discovered once per
// interface, and their current state (link, speed, queues):
// GET /api/v1/host/interfaces
//
// Unlike hostInfo, this runs ethtool for every interface, so the web UI
// requests it when the host view is opened rather than polling it.
func (s *Server) hostInterfaces() http.HandlerFunc {
	type response struct {
		// Ethtool reports whether ethtool is installed; without it,
		// firmware, port and channels are not available.
		Ethtool    bool       `json:"ethtool"`
		Interfaces []host.NIC `json:"interfaces"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resp := response{
			Ethtool:    host.EthtoolAvailable(),
			Interfaces: s.nics.Interfaces(r.Context()),
		}
		noCache(w)
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// hostTopology serves the hardware topology rendered by lstopo as PNG:
// GET /api/v1/host/topology.png
func (s *Server) hostTopology() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		executable, err := exec.LookPath(topologyCommand)
		if err != nil {
			JSONError(w, "lstopo (hwloc) is not installed", http.StatusNotFound)
			return
		}
		png, err := s.topology.get(r.Context(), executable)
		if err != nil {
			log.Warn().Err(err).Msg("failed to render host topology")
			JSONError(w, "not able to render host topology", http.StatusInternalServerError)
			return
		}
		disableWriteDeadline(w)
		w.Header().Set(contentType, "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(png)))
		noCache(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(png)
	}
}

func (c *topologyCache) get(ctx context.Context, executable string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.png != nil && time.Since(c.created) < topologyCacheTTL {
		return c.png, nil
	}
	ctx, cancel := context.WithTimeout(ctx, topologyTimeout)
	defer cancel()
	// "-" writes the image to stdout, so no temporary file is needed.
	cmd := exec.CommandContext(ctx, executable, "--of", "png", "-")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	c.png, c.created = stdout.Bytes(), time.Now()
	return c.png, nil
}
