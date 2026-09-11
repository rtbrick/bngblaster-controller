// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

// overviewCommands are the control socket commands aggregated by the
// instance overview endpoint. The key of each entry in the response is the
// command name, which is also the key bngblaster wraps its payload in.
var overviewCommands = []string{
	"session-counters",
	"network-interfaces",
	"access-interfaces",
	"a10nsp-interfaces",
	"test-info",
}

// overview aggregates every control socket command the instance detail view
// polls into a single cached response: GET .../_overview
//
// The Session Overview tab previously issued one request per command every
// two seconds, and the header duration badge a fifth, so a single open
// browser tab meant five uncached unix socket round-trips every two seconds
// and N tabs meant 5*N. Serving them from one endpoint behind the shared
// summary cache collapses that to one round-trip per command per cache
// period regardless of how many viewers are watching.
//
// A command that fails individually (unsupported by this bngblaster build,
// or simply not applicable) yields a null value for its key rather than
// failing the whole response - the UI hides the corresponding section.
func (s *Server) overview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		result, err := s.overviewCache.get(instance, func() (map[string]json.RawMessage, error) {
			out := map[string]json.RawMessage{}
			var firstErr error
			for _, command := range overviewCommands {
				payload, err := s.repository.Command(instance, controller.SocketCommand{Command: command})
				if err != nil {
					// ErrBlasterNotRunning applies to every command equally, so
					// remember it and report it once the loop is done; anything
					// else is treated as "this command is unavailable".
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				var envelope map[string]json.RawMessage
				if err := json.Unmarshal(payload, &envelope); err != nil {
					continue
				}
				if value, ok := envelope[command]; ok {
					out[command] = value
				}
			}
			if len(out) == 0 && firstErr != nil {
				return nil, firstErr
			}
			return out, nil
		})
		if err == controller.ErrBlasterNotRunning {
			JSONError(w, "instance is not running", http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			JSONError(w, "not able to fetch instance overview", http.StatusInternalServerError)
			return
		}

		// Always emit every key so the client can tell "not reported" from
		// "not requested" without knowing the command list itself.
		response := map[string]json.RawMessage{}
		for _, command := range overviewCommands {
			response[command] = result[command]
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}
}
