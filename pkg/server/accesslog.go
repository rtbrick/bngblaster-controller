// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// ClientHeader marks the requests of the embedded web UI and of the
	// health check, see WithQuietUIRequests. It is set by the client and
	// thus only ever used to lower the log level of successful reads, never
	// of actions.
	ClientHeader  = "X-Bngblaster-Client"
	uiClientValue = "webui"
	// HealthClientValue marks the requests of "bngblasterctrl -health",
	// which a container runtime runs every few seconds; they are as quiet
	// as the polling of the web UI.
	HealthClientValue = "health"

	// maxLoggedErrorBody bounds how much of an error response is kept to
	// extract the message for the access log line.
	maxLoggedErrorBody = 512
)

// readOnlyCommands are the bngblaster control commands that only report
// state. A _command request carrying one of them is a read like a GET;
// every other command (including unknown ones) counts as an action and is
// always logged.
var readOnlyCommands = map[string]struct{}{
	"a10nsp-interfaces": {}, "access-interfaces": {}, "access-protocol-stats": {}, "arp-clients": {},
	"bgp-evpn-routes": {}, "bgp-l2vpn-routes": {}, "bgp-raw-update-list": {}, "bgp-routes": {},
	"bgp-routes-stats": {}, "bgp-sessions": {}, "commands": {}, "http-clients": {}, "icmp-clients": {},
	"igmp-info": {}, "interfaces": {}, "interface-topology": {}, "isis-adjacencies": {}, "isis-database": {},
	"l2tp-sessions": {}, "l2tp-tunnels": {}, "lag-info": {}, "ldp-adjacencies": {}, "ldp-raw-update-list": {},
	"ldp-sessions": {}, "li-flows": {}, "network-interfaces": {}, "ospf-database": {}, "ospf-interfaces": {},
	"ospf-neighbors": {}, "session-counters": {}, "session-info": {}, "sessions-pending": {},
	"session-streams": {}, "session-summary": {}, "session-traffic": {}, "stream-info": {},
	"streams-pending": {}, "stream-stats": {}, "stream-summary": {}, "test-info": {}, "zapping-stats": {},
}

// requestLog collects what handlers know about a request, so that the
// access log writes a single line per request - including its outcome -
// instead of handlers logging separate lines that can only be correlated
// by timestamp.
type requestLog struct {
	instance string
	action   string
	// readOnly marks a request that only reports state, see readOnlyCommands.
	readOnly bool
	fields   []func(*zerolog.Event)
}

type requestLogKey struct{}

// logFields returns the access log entry of the request. Outside of the
// access log middleware (e.g. a handler called directly in a test) it
// returns a detached entry, so callers never need to check.
func logFields(r *http.Request) *requestLog {
	if rl, ok := r.Context().Value(requestLogKey{}).(*requestLog); ok {
		return rl
	}
	return &requestLog{}
}

// audit records a state change on the request's log line. Without
// authentication the client address logged with it is the only record of
// who did what.
func audit(r *http.Request, instance, action string) *requestLog {
	rl := logFields(r)
	rl.instance = instance
	rl.action = action
	return rl
}

// Str adds a string field to the request's log line.
func (rl *requestLog) Str(key, value string) *requestLog {
	rl.fields = append(rl.fields, func(e *zerolog.Event) { e.Str(key, value) })
	return rl
}

// Strs adds a string list field to the request's log line.
func (rl *requestLog) Strs(key string, values []string) *requestLog {
	rl.fields = append(rl.fields, func(e *zerolog.Event) { e.Strs(key, values) })
	return rl
}

// Int adds an integer field to the request's log line.
func (rl *requestLog) Int(key string, value int) *requestLog {
	rl.fields = append(rl.fields, func(e *zerolog.Event) { e.Int(key, value) })
	return rl
}

// Int64 adds an integer field to the request's log line.
func (rl *requestLog) Int64(key string, value int64) *requestLog {
	rl.fields = append(rl.fields, func(e *zerolog.Event) { e.Int64(key, value) })
	return rl
}

// Err adds the cause of a failure to the request's log line. The response
// message is logged anyway; this is for details that the client does not
// get to see.
func (rl *requestLog) Err(err error) *requestLog {
	if err != nil {
		rl.Str("cause", err.Error())
	}
	return rl
}

// accessRecorder captures the status code and, for error responses, the
// start of the body, which holds the message logged with the request.
type accessRecorder struct {
	http.ResponseWriter

	status  int
	errBody bytes.Buffer
}

func (w *accessRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *accessRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.status >= http.StatusBadRequest && w.errBody.Len() < maxLoggedErrorBody {
		w.errBody.Write(p[:min(len(p), maxLoggedErrorBody-w.errBody.Len())])
	}
	return w.ResponseWriter.Write(p)
}

// Flush keeps streaming responses working through the recorder.
func (w *accessRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection, which
// disableWriteDeadline relies on.
func (w *accessRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// errorMessage extracts the message of an error response: the "message"
// field of a JSON body (JSONError, bngblaster command errors), or the
// plain text written by http.Error.
func (w *accessRecorder) errorMessage() string {
	body := bytes.TrimSpace(w.errBody.Bytes())
	if len(body) == 0 {
		return ""
	}
	if body[0] == '{' {
		var m struct {
			Message any `json:"message"`
		}
		if json.Unmarshal(body, &m) == nil && m.Message != nil {
			if s, ok := m.Message.(string); ok {
				return s
			}
			b, _ := json.Marshal(m.Message)
			return string(b)
		}
	}
	return strings.TrimSpace(string(body))
}

// accessLog writes one line per request once it has been handled:
// method, URI, status and duration, plus what the handler recorded via
// logFields (instance, action, ...) and the message of an error response.
//
// The level follows the outcome - error for 5xx, warning for 4xx, info
// otherwise - so the stderr log (warn+) lists every failed request. With
// quietUI, successful reads of the web UI (its polling, which otherwise
// floods the log every second) and of the health check are logged at
// debug level only.
func accessLog(next http.Handler, quietUI bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &requestLog{}
		rec := &accessRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), requestLogKey{}, rl)))

		status := rec.status
		if status == 0 {
			// Nothing written at all, which net/http answers with 200.
			status = http.StatusOK
		}
		client := knownClient(r)
		read := rl.readOnly || r.Method == http.MethodGet || r.Method == http.MethodHead
		level := accessLevel(status, quietUI && client != "" && read)
		if level < zerolog.GlobalLevel() {
			return
		}
		event := log.WithLevel(level).Str("remote_addr", clientIP(r))
		rl.addTo(event, client)
		if status >= http.StatusBadRequest {
			if msg := rec.errorMessage(); msg != "" {
				event.Str("error", msg)
			}
		}
		event.Msg(fmt.Sprintf("%s %s %d %s", r.Method, r.RequestURI, status, roundDuration(time.Since(start))))
	})
}

// knownClient returns the client marked by ClientHeader, if it is one that
// polls (the web UI or the health check), and "" otherwise.
func knownClient(r *http.Request) string {
	switch client := r.Header.Get(ClientHeader); client {
	case uiClientValue, HealthClientValue:
		return client
	default:
		return ""
	}
}

// accessLevel returns the log level of a request by its outcome; quiet
// demotes a successful request to debug.
func accessLevel(status int, quiet bool) zerolog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return zerolog.ErrorLevel
	case status >= http.StatusBadRequest:
		return zerolog.WarnLevel
	case quiet:
		return zerolog.DebugLevel
	default:
		return zerolog.InfoLevel
	}
}

// addTo adds the recorded fields to the request's log line.
func (rl *requestLog) addTo(event *zerolog.Event, client string) {
	if rl.instance != "" {
		event.Str("instance", rl.instance)
	}
	if client != "" {
		event.Str("client", client)
	}
	if rl.action != "" {
		event.Str("action", rl.action)
	}
	for _, field := range rl.fields {
		field(event)
	}
}

// instanceLogMiddleware records the instance of the matched route on the
// request's log line. It runs as a router middleware, where the path
// variables are known, while accessLog wraps the router to also log
// requests that match no route.
func instanceLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name, ok := mux.Vars(r)[instanceNameParameter]; ok {
			logFields(r).instance = cleanPathVariable(name)
		}
		next.ServeHTTP(w, r)
	})
}

// roundDuration keeps the logged duration readable: milliseconds for
// anything that takes at least one, microseconds below.
func roundDuration(d time.Duration) time.Duration {
	if d >= time.Millisecond {
		return d.Round(time.Millisecond)
	}
	return d.Round(time.Microsecond)
}
