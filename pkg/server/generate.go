// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/generator"
)

const (
	// generatorTimeout bounds a generator run. bgpupdate is Python and
	// takes minutes for millions of prefixes; anything beyond this is
	// rather a mistake than a test setup.
	generatorTimeout = 10 * time.Minute

	// generatorInfoTTL is how long the probe results of the generators
	// endpoint are reused. Probing starts Python twice, which takes about
	// a second, while an installation changes rarely.
	generatorInfoTTL = time.Minute
)

// generatorInfoCache keeps the result of probing the generator tools.
type generatorInfoCache struct {
	mu      sync.Mutex
	infos   []generator.Info
	expires time.Time
}

// generatorInfos returns the enabled generator tools with their options
// and whether they are usable. The mutex is held while probing, so
// concurrent requests share one probe instead of starting their own.
func (s *Server) generatorInfos(ctx context.Context) []generator.Info {
	c := &s.generatorProbe
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.infos != nil && time.Now().Before(c.expires) {
		return c.infos
	}
	// A client going away must not end the probe: its failure would be
	// cached as the tools being unavailable.
	ctx = context.WithoutCancel(ctx)
	var tools []generator.Tool
	for _, tool := range generator.Tools {
		if _, ok := s.generators[tool.Name]; ok {
			tools = append(tools, tool)
		}
	}
	infos := make([]generator.Info, len(tools))
	var wg sync.WaitGroup
	for i, tool := range tools {
		wg.Go(func() {
			infos[i] = generator.Probe(ctx, tool, s.generators[tool.Name])
		})
	}
	wg.Wait()
	c.infos = infos
	c.expires = time.Now().Add(generatorInfoTTL)
	return infos
}

// generatorList lists the enabled generator tools, whether they are
// available and their options, which is what the web UI renders its
// generator forms from.
func (s *Server) generatorList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		infos := s.generatorInfos(r.Context())
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(infos)
	}
}

// generateResponse is the result of a run. Message is set if the run did
// not succeed; the tool's output is reported either way, since that is
// where the reason is.
type generateResponse struct {
	generator.Result

	Message string `json:"message,omitempty"`
}

// generate runs a generator tool in an instance folder. The request is
// validated against the tool's options before anything runs, see
// generator.Build.
func (s *Server) generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		tool := mux.Vars(r)["tool"]
		executable, ok := s.generators[tool]
		if !ok || !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestSize))
		if isBodyTooLarge(err) {
			JSONError(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			JSONError(w, "body not readable", http.StatusBadRequest)
			return
		}
		job, err := generator.Build(tool, body)
		if err != nil {
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// A run can take minutes, see generatorTimeout.
		disableWriteDeadline(w)
		ctx, cancel := context.WithTimeout(r.Context(), generatorTimeout)
		defer cancel()
		result, err := s.repository.Generate(ctx, instance, executable, job)

		status := generateStatus(err)
		if status == http.StatusNotFound {
			// The instance was deleted before the run could start.
			JSONNotFound(w, r)
			return
		}
		event := auditLog(r, instance).Str("tool", tool).Strs("files", result.Files).
			Int("exit_code", result.ExitCode).Int64("duration_ms", result.DurationMS)
		response := generateResponse{Result: result}
		if err != nil {
			event = event.Err(err)
			response.Message = err.Error()
		}
		event.Msg("generator run")
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}
}

// generateStatus maps the error of a run to its HTTP status. A canceled
// run has no one left to read the status, so it falls under the default.
func generateStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, controller.ErrBlasterNotExists):
		return http.StatusNotFound
	case errors.Is(err, controller.ErrGeneratorBusy), errors.Is(err, generator.ErrOutputExists):
		return http.StatusConflict
	case errors.Is(err, generator.ErrInputMissing):
		return http.StatusBadRequest
	case errors.Is(err, generator.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, generator.ErrFailed):
		return http.StatusUnprocessableEntity
	case errors.Is(err, generator.ErrTimeout):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
