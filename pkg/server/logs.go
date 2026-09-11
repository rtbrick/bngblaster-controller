// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2025, RtBrick, Inc.
package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"syscall"

	"github.com/gorilla/mux"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
)

const (
	defaultLogReadLimit = 64 * 1024
	maxLogReadLimit     = 1 << 20
)

// logsResponse is returned by the log tail endpoint. NextOffset should be
// passed back as the "offset" query parameter on the following poll so the
// viewer only ever receives newly appended log lines.
//
// Generation identifies the log file itself (its inode), not its contents.
// Starting an instance deletes and recreates run.log, so an offset carried
// over from a previous run points into a file that no longer exists: if the
// new log has already grown past that offset, a plain size comparison cannot
// detect the rotation and everything written before it is silently skipped.
// A client must therefore reset its offset to 0 whenever Generation changes.
type logsResponse struct {
	Generation uint64   `json:"generation"`
	Offset     int64    `json:"offset"`
	NextOffset int64    `json:"next_offset"`
	EOF        bool     `json:"eof"`
	Lines      []string `json:"lines"`
}

// generationPrefixLen is how many bytes from the start of a log file are
// hashed into its generation. Enough to cover the first log line, which
// carries a timestamp and therefore differs between runs.
const generationPrefixLen = 256

// logGeneration returns an identifier that changes whenever the log file a
// client is reading is replaced by a different one.
//
// File metadata cannot answer this. Starting an instance deletes run.log and
// immediately recreates it, which on ext4 reuses the just-freed inode - and
// with it the recorded birth time - so neither identifies the new file as
// distinct. What reliably differs is the content: the first log line carries
// a timestamp from the run that wrote it. Hashing the file's leading bytes
// together with its inode therefore answers the question actually being
// asked, which is "is this still the file whose offset I am holding?".
func logGeneration(f *os.File, info os.FileInfo) uint64 {
	prefix := make([]byte, generationPrefixLen)
	n, err := f.ReadAt(prefix, 0)
	if err != nil && err != io.EOF {
		n = 0
	}
	prefix = prefix[:n]

	// FNV-1a over the inode followed by the content prefix.
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	hash := uint64(offset64)
	mix := func(b byte) {
		hash ^= uint64(b)
		hash *= prime64
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		for shift := 0; shift < 64; shift += 8 {
			mix(byte(stat.Ino >> shift))
		}
	}
	for _, b := range prefix {
		mix(b)
	}
	return hash
}

// logs is a STUB handler for the instance log viewer.
//
// bngblaster does not currently expose a socket command to stream log
// messages, so this implementation tails the run.log file that the
// bngblaster process writes to when started with logging enabled
// (see RunningConfig.Logging). The UI polls this endpoint with the
// "offset" it last received, which keeps the request cheap regardless of
// how large the log file grows.
//
// Once bngblaster gains a native "log" (or similar) socket command capable
// of streaming structured log messages, this handler should be replaced
// with one that forwards to repository.Command the same way s.streams()
// does, without requiring any change to the frontend's polling contract
// (offset/next_offset/eof/lines).
func (s *Server) logs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		offset := int64(parseNonNegativeIntQuery(r, "offset", 0))
		limit := parseNonNegativeIntQuery(r, "limit", defaultLogReadLimit)
		if limit <= 0 || limit > maxLogReadLimit {
			limit = defaultLogReadLimit
		}

		file := path.Join(s.repository.ConfigFolder(), instance, controller.RunLogFilename)
		resp, err := tailLogFile(file, offset, limit)
		if err != nil {
			// No log file yet (e.g. instance never started with logging
			// enabled) is not an error from the UI's perspective.
			if os.IsNotExist(err) {
				// No log file yet: generation 0 tells the client to reset,
				// so a stale offset from a previous run cannot survive.
				w.Header().Set(contentType, applicationJSON)
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(logsResponse{Offset: 0, NextOffset: 0, EOF: true, Lines: []string{}})
				return
			}
			JSONError(w, "not able to read log", http.StatusInternalServerError)
			return
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func tailLogFile(file string, offset int64, limit int) (logsResponse, error) {
	f, err := os.Open(file)
	if err != nil {
		return logsResponse{}, err
	}
	defer func() {
		_ = f.Close()
	}()

	info, err := f.Stat()
	if err != nil {
		return logsResponse{}, err
	}
	size := info.Size()
	generation := logGeneration(f, info)
	if offset > size {
		// File was truncated/rotated since the last poll; restart from 0.
		offset = 0
	}

	toRead := size - offset
	if toRead > int64(limit) {
		toRead = int64(limit)
	}
	if toRead < 0 {
		toRead = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return logsResponse{}, err
	}

	buf := make([]byte, toRead)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return logsResponse{}, err
	}
	buf = buf[:n]
	nextOffset := offset + int64(n)

	// Only emit complete lines; keep any trailing partial line for the next
	// poll by not advancing nextOffset past the last newline.
	lastNewline := bytes.LastIndexByte(buf, '\n')
	complete := buf
	if lastNewline == -1 {
		complete = nil
	} else {
		complete = buf[:lastNewline+1]
		nextOffset = offset + int64(lastNewline+1)
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(complete))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if lines == nil {
		lines = []string{}
	}

	return logsResponse{
		Generation: generation,
		Offset:     offset,
		NextOffset: nextOffset,
		EOF:        nextOffset >= size,
		Lines:      lines,
	}, nil
}
