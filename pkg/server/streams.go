// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

const (
	defaultStreamPageSize = 50
	maxStreamPageSize     = 500
)

// streamFilters mirrors the filter arguments accepted by the bngblaster
// "stream-summary" control socket command, allowing the stream table to
// narrow down the (potentially large) stream list server-side instead of
// downloading everything and filtering in the browser.
type streamFilters struct {
	SessionID      *int
	SessionGroupID *int
	FlowID         *int
	FlowIDMin      *int
	FlowIDMax      *int
	Name           string
	Interface      string
	Direction      string
	// State is one of "verified", "bidirectional-verified", "pending" or ""
	// (any), matching the mutually exclusive verified-only /
	// bidirectional-verified-only / pending-only socket command arguments.
	State string
}

func parseStreamFilters(r *http.Request) streamFilters {
	q := r.URL.Query()
	f := streamFilters{
		Name:      q.Get("name"),
		Interface: q.Get("interface"),
		Direction: q.Get("direction"),
		State:     q.Get("state"),
	}
	f.SessionID = parseOptionalIntQuery(r, "session-id")
	f.SessionGroupID = parseOptionalIntQuery(r, "session-group-id")
	f.FlowID = parseOptionalIntQuery(r, "flow-id")
	f.FlowIDMin = parseOptionalIntQuery(r, "flow-id-min")
	f.FlowIDMax = parseOptionalIntQuery(r, "flow-id-max")
	return f
}

// cacheKey is a stable string encoding of the filter set, used as (part of)
// the stream-summary cache key.
func (f streamFilters) cacheKey() string {
	key := ""
	if f.SessionID != nil {
		key += fmt.Sprintf("|session-id=%d", *f.SessionID)
	}
	if f.SessionGroupID != nil {
		key += fmt.Sprintf("|session-group-id=%d", *f.SessionGroupID)
	}
	if f.FlowID != nil {
		key += fmt.Sprintf("|flow-id=%d", *f.FlowID)
	}
	if f.FlowIDMin != nil {
		key += fmt.Sprintf("|flow-id-min=%d", *f.FlowIDMin)
	}
	if f.FlowIDMax != nil {
		key += fmt.Sprintf("|flow-id-max=%d", *f.FlowIDMax)
	}
	if f.Name != "" {
		key += "|name=" + f.Name
	}
	if f.Interface != "" {
		key += "|interface=" + f.Interface
	}
	if f.Direction != "" {
		key += "|direction=" + f.Direction
	}
	if f.State != "" {
		key += "|state=" + f.State
	}
	return key
}

// arguments builds the "arguments" object sent alongside the
// "stream-summary" socket command.
func (f streamFilters) arguments() map[string]any {
	args := map[string]any{}
	setIntArgument(args, "session-id", f.SessionID)
	setIntArgument(args, "session-group-id", f.SessionGroupID)
	setIntArgument(args, "flow-id", f.FlowID)
	setIntArgument(args, "flow-id-min", f.FlowIDMin)
	setIntArgument(args, "flow-id-max", f.FlowIDMax)
	setStringArgument(args, "name", f.Name)
	setStringArgument(args, "interface", f.Interface)
	setStringArgument(args, "direction", f.Direction)
	switch f.State {
	case "verified":
		args["verified-only"] = true
	case "bidirectional-verified":
		args["bidirectional-verified-only"] = true
	case "pending":
		args["pending-only"] = true
	}
	return args
}

// streamsResponse is the paginated view of stream-summary returned to the UI.
type streamsResponse = summaryPage[controller.StreamSummaryStream]

// streams implements the "floating range" pagination endpoint backing the
// virtual-scrolling stream table (GET .../_streams?offset=&limit=).
func (s *Server) streams() http.HandlerFunc {
	return serveSummaryPage(s, summaryEndpoint[controller.StreamSummaryStream]{
		command:      "stream-summary",
		errorMessage: "not able to fetch stream summary",
		defaultLimit: defaultStreamPageSize,
		maxLimit:     maxStreamPageSize,
		cache:        s.streamCache,
		query: func(r *http.Request) (map[string]any, string, *int) {
			filters := parseStreamFilters(r)
			return filters.arguments(), filters.cacheKey(), windowStart(r, filters.FlowIDMin, filters.FlowIDMax)
		},
		items: func(payload []byte) ([]controller.StreamSummaryStream, error) {
			var parsed controller.StreamSummaryResponse
			if err := json.Unmarshal(payload, &parsed); err != nil {
				return nil, err
			}
			return parsed.Streams, nil
		},
	})
}
