// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// disableWriteDeadline lifts the server-wide WriteTimeout for the current
// request. That timeout protects the JSON endpoints against stuck clients,
// but it is measured from the end of the request headers, so it would also
// abort any pcap or log download (and any upload response) that simply
// takes longer than the timeout over a slow link.
func disableWriteDeadline(w http.ResponseWriter) {
	// An error only means the writer does not support deadlines (e.g. in
	// tests); the server-wide timeout then keeps applying, which is safe.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}

// files lists the downloadable files present in an instance's config
// folder, used by the web UI's "Download" view.
func (s *Server) files() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		files, err := s.repository.Files(instance)
		if err != nil {
			JSONError(w, "not able to list files", http.StatusInternalServerError)
			return
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(files)
	}
}

// fileDownload serves a single file out of an instance's config folder.
// Unlike the fixed-name route registered for the well-known result files,
// this accepts any file name (e.g. user-uploaded files) since it only ever
// downloads names the files() endpoint itself just listed - path traversal
// is prevented by only ever taking the base component of the requested name.
//
// The folder holds arbitrary user-uploaded content, so the response is
// forced to a download: without "Content-Disposition: attachment" plus
// "X-Content-Type-Options: nosniff", an uploaded .html or .svg file would be
// served inline and execute script in the controller's own origin - a stored
// cross-site scripting vector against every other user of this UI.
func (s *Server) fileDownload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := clientIP(r)
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}
		// Base's own degenerate outputs ("", ".", "..", "/") are rejected
		// outright since joining any of them would land outside the
		// instance folder - see isUnsafeFileName.
		file := filepath.Base(mux.Vars(r)["file_name"])
		if isUnsafeFileName(file) {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}
		log.Info().Str("remote_addr", remoteAddr).Str("instance", instance).Str("file", file).Msg("file downloaded")
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(file))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(contentType, "application/octet-stream")
		disableWriteDeadline(w)
		http.ServeFile(w, r, filepath.Join(s.repository.ConfigFolder(), instance, file))
	}
}
