// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

func TestServer_files(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return true },
		FilesFunc: func(name string) ([]controller.InstanceFile, error) {
			return []controller.InstanceFile{{Name: "run_report.json", Size: 12}}, nil
		},
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_files")
	require.Equal(t, http.StatusOK, recorder.Code)

	var files []controller.InstanceFile
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &files))
	require.Equal(t, []controller.InstanceFile{{Name: "run_report.json", Size: 12}}, files)
}

func TestServer_fileDownload_isForcedToADownload(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))
	// The instance folder holds arbitrary uploaded content. Served inline,
	// this would execute script in the controller's own origin.
	payload := "<script>alert(document.domain)</script>"
	require.NoError(t, os.WriteFile(filepath.Join(folder, "test", "evil.html"), []byte(payload), 0o600))

	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		ExistsFunc:       func(name string) bool { return true },
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_files/evil.html")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, payload, recorder.Body.String())
	require.Equal(t, `attachment; filename="evil.html"`, recorder.Header().Get("Content-Disposition"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"),
		"the browser must never be told this is renderable HTML")
}

func TestServer_fileDownload_missingInstance(t *testing.T) {
	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return configFolder },
		ExistsFunc:       func(name string) bool { return false },
	}
	handler := NewServer(repository)

	recorder := doGet(t, handler, "/api/v1/instances/test/_files/whatever.json")
	require.Equal(t, http.StatusNotFound, recorder.Code)
}

// uploadRequest builds a multipart upload carrying the given (possibly
// hostile) filename.
func uploadRequest(t *testing.T, instance, filename, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/api/v1/instances/"+instance+"/_upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestServer_uploadFile_cannotEscapeInstanceFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "configs")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))

	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		AllowUploadFunc:  func() bool { return true },
		ExistsFunc:       func(name string) bool { return true },
	}
	handler := NewServer(repository)

	// net/http strips the directory components itself, and the handler takes
	// the base name again on top of that. This pins the resulting guarantee:
	// whatever a client puts in the multipart filename, the upload lands
	// inside the instance folder and nowhere else.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, uploadRequest(t, "test", "../../pwned.txt", "payload"))
	require.Equal(t, http.StatusOK, recorder.Code)

	_, err := os.Stat(filepath.Join(root, "pwned.txt"))
	require.True(t, os.IsNotExist(err), "upload must not escape the instance folder")
	_, err = os.Stat(filepath.Join(folder, "pwned.txt"))
	require.True(t, os.IsNotExist(err), "upload must not escape the instance folder")

	// It lands under its base name inside the instance folder instead.
	written, err := os.ReadFile(filepath.Join(folder, "test", "pwned.txt"))
	require.NoError(t, err)
	require.Equal(t, "payload", string(written))
}

func TestServer_uploadFile_storesPlainNameUnchanged(t *testing.T) {
	folder := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "test"), 0o700))

	repository := &controller.RepositoryMock{
		ConfigFolderFunc: func() string { return folder },
		AllowUploadFunc:  func() bool { return true },
		ExistsFunc:       func(name string) bool { return true },
	}
	handler := NewServer(repository)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, uploadRequest(t, "test", "streams.json", `{"streams":[]}`))
	require.Equal(t, http.StatusOK, recorder.Code)

	written, err := os.ReadFile(filepath.Join(folder, "test", "streams.json"))
	require.NoError(t, err)
	require.Equal(t, `{"streams":[]}`, string(written))
}
