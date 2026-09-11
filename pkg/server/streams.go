// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

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

func parseOptionalIntQuery(r *http.Request, name string) *int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &v
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
func (f streamFilters) arguments() map[string]interface{} {
	args := map[string]interface{}{}
	if f.SessionID != nil {
		args["session-id"] = *f.SessionID
	}
	if f.SessionGroupID != nil {
		args["session-group-id"] = *f.SessionGroupID
	}
	if f.FlowID != nil {
		args["flow-id"] = *f.FlowID
	}
	if f.FlowIDMin != nil {
		args["flow-id-min"] = *f.FlowIDMin
	}
	if f.FlowIDMax != nil {
		args["flow-id-max"] = *f.FlowIDMax
	}
	if f.Name != "" {
		args["name"] = f.Name
	}
	if f.Interface != "" {
		args["interface"] = f.Interface
	}
	if f.Direction != "" {
		args["direction"] = f.Direction
	}
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
// This is the "floating range" contract used by the virtual scrolling stream
// table: the client only ever asks for the slice of rows currently in (or
// near) the viewport instead of downloading the entire stream list.
type streamsResponse struct {
	Total  int                              `json:"total"`
	Offset int                              `json:"offset"`
	Limit  int                              `json:"limit"`
	Items  []controller.StreamSummaryStream `json:"items"`
}

// streams implements the "floating range" pagination endpoint backing the
// virtual-scrolling stream table: GET .../_streams?offset=&limit=
func (s *Server) streams() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		offset := parseNonNegativeIntQuery(r, "offset", 0)
		limit := parseNonNegativeIntQuery(r, "limit", defaultStreamPageSize)
		if limit <= 0 {
			limit = defaultStreamPageSize
		}
		if limit > maxStreamPageSize {
			limit = maxStreamPageSize
		}

		filters := parseStreamFilters(r)
		// "window=1" marks a flow-id range the UI's virtual scroller derived
		// from its scroll position rather than one the user typed into the
		// filter panel. The two need different pagination semantics (see
		// below), and only the client knows which is which.
		windowed := r.URL.Query().Get("window") == "1" && filters.FlowIDMin != nil && filters.FlowIDMax != nil
		cacheKey := instance + filters.cacheKey()

		streamsData, err := s.streamCache.get(cacheKey, func() ([]controller.StreamSummaryStream, error) {
			result, err := s.repository.Command(instance, controller.SocketCommand{
				Command:   "stream-summary",
				Arguments: filters.arguments(),
			})
			if err != nil {
				return nil, err
			}
			var parsed controller.StreamSummaryResponse
			if err := json.Unmarshal(result, &parsed); err != nil {
				return nil, err
			}
			return parsed.Streams, nil
		})
		if err == controller.ErrBlasterNotRunning {
			JSONError(w, "instance is not running", http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			JSONError(w, "not able to fetch stream summary", http.StatusInternalServerError)
			return
		}

		var resp streamsResponse
		if windowed {
			// The flow-id range was generated by the UI's virtual-scroll
			// window, not typed by a user: it asked bngblaster for exactly the
			// slice of streams it is about to render, so that slice is returned
			// as-is. Offset is the absolute row index the slice starts at,
			// which for a sequentially assigned flow-id chain is FlowIDMin-1.
			resp = streamsResponse{
				Total:  len(streamsData),
				Offset: *filters.FlowIDMin - 1,
				Limit:  limit,
				Items:  streamsData,
			}
		} else {
			// Everything else - including a user-entered flow-id range - is
			// plain offset/limit pagination over the filtered result. Offset
			// is a row index into that result and Total is its full length, so
			// the client can size a scrollbar for the filtered list correctly.
			total := len(streamsData)
			start := offset
			if start > total {
				start = total
			}
			end := start + limit
			if end > total {
				end = total
			}
			resp = streamsResponse{
				Total:  total,
				Offset: start,
				Limit:  limit,
				Items:  streamsData[start:end],
			}
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func parseNonNegativeIntQuery(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return def
	}
	return v
}
