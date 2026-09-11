// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gorilla/mux"
)

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
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}
		file := filepath.Base(mux.Vars(r)["file_name"])
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(file))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(contentType, "application/octet-stream")
		http.ServeFile(w, r, filepath.Join(s.repository.ConfigFolder(), instance, file))
	}
}
