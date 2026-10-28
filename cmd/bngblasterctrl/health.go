// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/server"
)

// healthTimeout bounds the whole health check, well below the timeouts
// container runtimes use for a health check command (docker: 30s).
const healthTimeout = 2 * time.Second

// healthClient talks to the controller directly: lab containers often have
// HTTP_PROXY set for downloads, which must not apply to an -addr bound to a
// non-loopback address (loopback is never proxied anyway).
var healthClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

// checkHealth checks the controller listening on addr (the -addr value)
// and, if instance is not empty, the autostart of that instance. It
// returns a description of the healthy state, or the reason why it is not
// healthy.
func checkHealth(ctx context.Context, addr, instance string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	base := "http://" + localAddr(addr)
	if instance == "" {
		if _, err := healthGet(ctx, base+"/api/v1/instances"); err != nil {
			return "", err
		}
		return "controller running", nil
	}
	body, err := healthGet(ctx, base+"/api/v1/instances/"+url.PathEscape(instance))
	if err != nil {
		return "", err
	}
	var st server.InstanceStatus
	if err := json.Unmarshal(body, &st); err != nil {
		return "", fmt.Errorf("invalid status response: %w", err)
	}
	return instanceHealth(instance, st)
}

// instanceHealth decides whether the autostart instance is healthy: it
// is while it runs, and once it has terminated with exit code 0. Waiting
// for interfaces and starting are not healthy yet - the start period of
// the health check covers them.
func instanceHealth(instance string, st server.InstanceStatus) (string, error) {
	if st.Autostart != nil {
		switch st.Autostart.State {
		case controller.AutostartWaitingInterfaces:
			return "", errors.New("waiting for interfaces")
		case controller.AutostartStarting:
			return "", fmt.Errorf("instance %s is starting", instance)
		case controller.AutostartFailed:
			return "", fmt.Errorf("autostart of instance %s failed: %s", instance, st.Autostart.Error)
		}
	}
	switch {
	case st.Status == server.StatusStarted:
		return fmt.Sprintf("instance %s running", instance), nil
	case st.ExitCode == nil:
		return "", fmt.Errorf("instance %s stopped, exit code unknown", instance)
	case *st.ExitCode != 0:
		return "", fmt.Errorf("instance %s terminated with exit code %d", instance, *st.ExitCode)
	default:
		return fmt.Sprintf("instance %s terminated with exit code 0", instance), nil
	}
}

// healthGet returns the body of a successful GET of u.
func healthGet(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set(server.ClientHeader, server.HealthClientValue)
	resp, err := healthClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("controller not reachable: %w", err)
	}
	defer resp.Body.Close()
	var body json.RawMessage
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return body, nil
}

// localAddr turns the listen address addr into one to connect to from
// the same host: a wildcard or missing host becomes the loopback address.
func localAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return net.JoinHostPort(host, port)
}
