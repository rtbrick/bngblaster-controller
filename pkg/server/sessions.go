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
	defaultSessionPageSize = 50
	maxSessionPageSize     = 500
)

// sessionFilters mirrors the filter arguments accepted by the bngblaster
// "session-summary" control socket command.
type sessionFilters struct {
	SessionID      *int
	SessionGroupID *int
	SessionIDMin   *int
	SessionIDMax   *int
}

func parseSessionFilters(r *http.Request) sessionFilters {
	f := sessionFilters{}
	f.SessionID = parseOptionalIntQuery(r, "session-id")
	f.SessionGroupID = parseOptionalIntQuery(r, "session-group-id")
	f.SessionIDMin = parseOptionalIntQuery(r, "session-id-min")
	f.SessionIDMax = parseOptionalIntQuery(r, "session-id-max")
	return f
}

// cacheKey is a stable string encoding of the filter set, used as (part of)
// the session-summary cache key.
func (f sessionFilters) cacheKey() string {
	key := ""
	if f.SessionID != nil {
		key += fmt.Sprintf("|session-id=%d", *f.SessionID)
	}
	if f.SessionGroupID != nil {
		key += fmt.Sprintf("|session-group-id=%d", *f.SessionGroupID)
	}
	if f.SessionIDMin != nil {
		key += fmt.Sprintf("|session-id-min=%d", *f.SessionIDMin)
	}
	if f.SessionIDMax != nil {
		key += fmt.Sprintf("|session-id-max=%d", *f.SessionIDMax)
	}
	return key
}

// arguments builds the "arguments" object sent alongside the
// "session-summary" socket command.
func (f sessionFilters) arguments() map[string]any {
	args := map[string]any{}
	setIntArgument(args, "session-id", f.SessionID)
	setIntArgument(args, "session-group-id", f.SessionGroupID)
	setIntArgument(args, "session-id-min", f.SessionIDMin)
	setIntArgument(args, "session-id-max", f.SessionIDMax)
	return args
}

// sessionsResponse is the paginated view of session-summary returned to the
// UI, sharing the stream table's "floating range" contract.
type sessionsResponse = summaryPage[controller.SessionSummarySession]

// sessions implements the "floating range" pagination endpoint backing the
// virtual-scrolling session table (GET .../_sessions?offset=&limit=).
func (s *Server) sessions() http.HandlerFunc {
	return serveSummaryPage(s, summaryEndpoint[controller.SessionSummarySession]{
		command:      "session-summary",
		errorMessage: "not able to fetch session summary",
		defaultLimit: defaultSessionPageSize,
		maxLimit:     maxSessionPageSize,
		cache:        s.sessionCache,
		query: func(r *http.Request) (map[string]any, string, *int) {
			filters := parseSessionFilters(r)
			return filters.arguments(), filters.cacheKey(), windowStart(r, filters.SessionIDMin, filters.SessionIDMax)
		},
		items: func(payload []byte) ([]controller.SessionSummarySession, error) {
			var parsed controller.SessionSummaryResponse
			if err := json.Unmarshal(payload, &parsed); err != nil {
				return nil, err
			}
			return parsed.Sessions, nil
		},
	})
}
