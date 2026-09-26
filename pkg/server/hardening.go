// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// Request body limits. The controller runs as root without authentication,
// so no endpoint may let a single request exhaust memory or disk.
const (
	// maxConfigSize bounds an instance configuration (PUT). Generous, since
	// a config may carry thousands of inline stream definitions.
	maxConfigSize = 32 << 20
	// maxRequestSize bounds the small JSON bodies of _start and _command.
	maxRequestSize = 1 << 20
	// maxUploadSize bounds a single file upload, plus some headroom for the
	// multipart framing around it.
	maxUploadSize = 4000<<20 + 1<<20
)

// Content-Security-Policy values. defaultCSP is sent with every response
// and makes any document the API serves (a log, a report, config.json
// opened in a browser tab) inert: no scripts, no subresources, no framing.
// Those files can contain text an API caller controls, and would otherwise
// run with the controller's origin if a browser ever rendered them as HTML.
// The web UI and the API docs replace it with the policy they need.
const (
	defaultCSP = "default-src 'none'; frame-ancestors 'none'; sandbox"
	// uiCSP allows only the embedded assets. Inline styles remain allowed
	// since the markup uses style attributes; inline scripts do not.
	uiCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; " +
		"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	// apiDocsCSP allows the Swagger UI bundle loaded from unpkg and its
	// inline bootstrap script.
	apiDocsCSP = "default-src 'self'; script-src 'self' 'unsafe-inline' https://unpkg.com; " +
		"style-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https:; " +
		"connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
)

// securityHeadersMiddleware sets the headers every response carries.
// Handlers that serve an actual page overwrite Content-Security-Policy.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", defaultCSP)
		next.ServeHTTP(w, r)
	})
}

// crossOriginMiddleware rejects state-changing requests (anything but GET,
// HEAD and OPTIONS) that a browser sends on behalf of another site.
//
// Without authentication this is what stops a web page opened by anyone on
// the lab network from starting, killing or uploading into instances
// through that person's browser: a plain HTML form can POST cross-site
// without any CORS preflight. Browsers mark such requests via
// Sec-Fetch-Site or Origin; curl and other non-browser clients send
// neither header and are unaffected.
func crossOriginMiddleware() func(http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Warn().Str("remote_addr", clientIP(r)).Str("method", r.Method).Str("origin", r.Header.Get("Origin")).
			Str("sec_fetch_site", r.Header.Get("Sec-Fetch-Site")).Msg("cross-origin request rejected: " + r.RequestURI)
		JSONError(w, "cross-origin request rejected", http.StatusForbidden)
	}))
	return protection.Handler
}

// hostAllowlistMiddleware rejects requests whose Host header names a host
// that is not in allowed. It defends against DNS rebinding: a malicious
// site can re-point its own domain at the controller's address, after which
// the browser treats the controller as same-origin with that site and lets
// its scripts read every response. The Host header still carries the
// attacker's domain though, which is what is checked here.
//
// IP literals and localhost are always accepted, since a rebinding attack
// can only ever present a domain name the attacker controls. An empty
// allowed list disables the check.
func hostAllowlistMiddleware(allowed []string) func(http.Handler) http.Handler {
	hosts := make(map[string]struct{}, len(allowed))
	for _, host := range allowed {
		if host = normalizeHost(host); host != "" {
			hosts[host] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		if len(hosts) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = normalizeHost(host)
			if _, ok := hosts[host]; ok || host == "localhost" || net.ParseIP(host) != nil {
				next.ServeHTTP(w, r)
				return
			}
			log.Warn().Str("remote_addr", clientIP(r)).Str("host", r.Host).Msg("request rejected: host not allowed")
			JSONError(w, "host not allowed", http.StatusForbidden)
		})
	}
}

// normalizeHost lower-cases a host name and strips IPv6 brackets and a
// trailing root dot, so "Lab01.example.com." and "lab01.example.com" match.
func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	return strings.TrimSuffix(host, ".")
}

// isBodyTooLarge reports whether err stems from a body that exceeded its
// http.MaxBytesReader limit.
func isBodyTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}
