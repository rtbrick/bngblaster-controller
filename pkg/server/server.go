// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/host"
)

const (
	instanceNameParameter = "instance_name"
	errInstanceIsRunning  = "instance is running"
	contentType           = "Content-Type"
	applicationJSON       = "application/json"
)

func cleanPathVariable(instanceVariable string) string {
	instance := path.Clean(instanceVariable)
	instance = strings.ReplaceAll(instance, ".", "")
	instance = strings.ReplaceAll(instance, "/", "")
	return instance
}

// isUnsafeFileName reports whether name - already reduced to its base
// component via filepath.Base - could still escape the directory it is
// joined into. Base only strips leading directory components, so a
// filename that is itself "", ".", ".." or "/" (Base's results for those
// inputs) would otherwise resolve to the parent or instance directory
// itself instead of a file inside it.
func isUnsafeFileName(name string) bool {
	return name == "" || name == "." || name == ".." || name == string(filepath.Separator)
}

// clientIP returns the request's source IP, stripping the port from
// RemoteAddr. This is the direct TCP peer address rather than a
// client-supplied header (e.g. X-Forwarded-For), which cannot be trusted
// unless this server sits behind a specifically configured proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// availableDiskSpace returns the bytes available to the controller on the
// file system holding dir. ok is false if that cannot be determined, in
// which case the caller should not reject anything based on it.
func availableDiskSpace(dir string) (uint64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, false
	}
	return stat.Bavail * uint64(stat.Bsize), true //nolint:gosec // block size is never negative
}

// Server implementation for the rest api.
type Server struct {
	Version string
	router  *mux.Router
	// handler is the router wrapped into the access log.
	handler    http.Handler
	prom       *controller.Prom
	repository controller.Repository

	// enableUI toggles serving the embedded web UI on "/".
	enableUI bool
	// enableInterfaces toggles the "/api/v1/interfaces" endpoint.
	enableInterfaces bool
	// enableHost toggles the "/api/v1/host" endpoints.
	enableHost bool
	// hostCollector gathers the host information and keeps the CPU
	// usage sample between requests.
	hostCollector *host.Collector
	// nics keeps the network interface details, discovered at startup.
	nics     *host.NICInventory
	topology topologyCache
	// generators maps each enabled generator tool to its executable; empty
	// disables the generator endpoints.
	generators     map[string]string
	generatorProbe generatorInfoCache
	// schemaPath is the file system location of the bngblaster config schema.
	schemaPath string
	// allowedHosts restricts the Host header of incoming requests (see
	// hostAllowlistMiddleware). Empty accepts any host.
	allowedHosts []string
	// authMiddleware is invoked for every request. It is a no-op unless
	// WithAuthMiddleware is used, and is the extension point for plugging
	// in authentication/login later.
	authMiddleware AuthMiddleware
	// quietUI logs successful reads of the web UI at debug level only,
	// see accessLog.
	quietUI bool
	// autostart is the instance started at controller start, if any.
	autostart *controller.Autostart

	streamCache   *summaryCache[[]controller.StreamSummaryStream]
	sessionCache  *summaryCache[[]controller.SessionSummarySession]
	overviewCache *summaryCache[map[string]json.RawMessage]

	// assetVersion is appended to every embedded UI asset URL as a cache
	// busting query parameter, so a browser can cache them aggressively yet
	// never run a stale app.js against a newer controller. It is resolved
	// lazily because Version is assigned after NewServer returns.
	assetVersion     string
	assetVersionOnce sync.Once
}

// InterfaceInfo holds the information about a network interface.
type InterfaceInfo struct {
	Name  string   `json:"name"`
	MTU   int      `json:"mtu"`
	Flags []string `json:"flags"`
	Mac   string   `json:"mac"`
}

// VersionInfo holds controller version and the parsed output of the `bngblaster -v` command.
type VersionInfo struct {
	Version         string   `json:"bngblasterctrl-version"`
	BlasterVersion  string   `json:"bngblaster-version"`
	BlasterCompiler string   `json:"bngblaster-compiler"`
	BlasterIOModes  []string `json:"bngblaster-io-modes"`
}

// NewServer is a constructor function for Server.
func NewServer(repository controller.Repository, opts ...Option) *Server {
	r := &Server{
		Version:          "dev",
		router:           mux.NewRouter(),
		prom:             controller.NewProm(repository),
		repository:       repository,
		enableUI:         false,
		enableInterfaces: false,
		enableHost:       false,
		hostCollector:    host.NewCollector(),
		nics:             host.NewNICInventory(),
		schemaPath:       DefaultSchemaPath,
		authMiddleware:   noopAuthMiddleware,
		streamCache:      newSummaryCache[[]controller.StreamSummaryStream](),
		sessionCache:     newSummaryCache[[]controller.SessionSummarySession](),
		overviewCache:    newSummaryCache[map[string]json.RawMessage](),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.routes()
	r.handler = accessLog(r.router, r.quietUI)
	if r.enableHost {
		// Discover the interface details in the background, so neither
		// the startup nor the first request waits for them; a request
		// arriving earlier waits for the discovery to finish.
		go r.nics.Discover(context.Background())
	}
	return r
}

// ServeHTTP A Handler responds to an HTTP request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes() {
	const instanceURL = "/api/v1/instances/{instance_name}"
	s.router.Use(instanceLogMiddleware)
	// Hardening that does not depend on authentication; see hardening.go.
	// Security headers come first so that rejections carry them too.
	s.router.Use(securityHeadersMiddleware)
	s.router.Use(hostAllowlistMiddleware(s.allowedHosts))
	s.router.Use(crossOriginMiddleware())
	// authMiddleware is a no-op unless WithAuthMiddleware(...) was supplied.
	// It sits in front of both the UI and the API so a future login system
	// can be introduced here without touching individual handlers.
	s.router.Use(mux.MiddlewareFunc(s.authMiddleware))

	// Expose the registered metrics via HTTP.
	s.router.Path("/metrics").Methods(http.MethodGet).Handler(promhttp.HandlerFor(
		s.prom.Registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	))
	s.router.Path("/api/v1/version").Methods(http.MethodGet).Handler(s.version())
	s.router.Path("/api/v1/schema").Methods(http.MethodGet).Handler(s.schema())
	s.registerAPIDocsRoutes()
	if s.enableInterfaces {
		s.router.Path("/api/v1/interfaces").Methods(http.MethodGet).Handler(s.interfaces())
	}
	if s.enableHost {
		s.router.Path("/api/v1/host").Methods(http.MethodGet).Handler(s.hostInfo())
		s.router.Path("/api/v1/host/topology.png").Methods(http.MethodGet).Handler(s.hostTopology())
		s.router.Path("/api/v1/host/interfaces").Methods(http.MethodGet).Handler(s.hostInterfaces())
	}
	if len(s.generators) > 0 {
		s.router.Path("/api/v1/generators").Methods(http.MethodGet).Handler(s.generatorList())
		s.router.Path(instanceURL + "/_generate/{tool}").Methods(http.MethodPost).Handler(s.generate())
	}
	s.router.Path("/api/v1/instances").Methods(http.MethodGet).Handler(s.instances())
	s.router.Path(instanceURL + "/_overview").Methods(http.MethodGet).Handler(s.overview())
	s.router.Path(instanceURL + "/_streams").Methods(http.MethodGet).Handler(s.streams())
	s.router.Path(instanceURL + "/_sessions").Methods(http.MethodGet).Handler(s.sessions())
	s.router.Path(instanceURL + "/_logs").Methods(http.MethodGet).Handler(s.logs())
	s.router.Path(instanceURL + "/_files").Methods(http.MethodGet).Handler(s.files())
	s.router.Path(instanceURL + "/_files/{file_name}").Methods(http.MethodGet).Handler(s.fileDownload())
	s.router.
		Path(
			fmt.Sprintf("%s/{file_name:%s|%s|%s|%s|%s|%s|%s}",
				instanceURL,
				controller.ConfigFilename,
				controller.RunConfigFilename,
				controller.RunLogFilename,
				controller.RunReportFilename,
				controller.RunPcapFilename,
				controller.RunStdErr,
				controller.RunStdOut)).
		Methods(http.MethodGet).Handler(s.fileServing(s.repository.ConfigFolder()))

	s.router.Path(instanceURL).Methods(http.MethodGet).Handler(s.status())
	s.router.Path(instanceURL).Methods(http.MethodPut).Handler(s.create())
	s.router.Path(instanceURL).Methods(http.MethodDelete).Handler(s.delete())
	s.router.Path(instanceURL + "/_start").Methods(http.MethodPost).Handler(s.start())
	s.router.Path(instanceURL + "/_stop").Methods(http.MethodPost).Handler(s.stop())
	s.router.Path(instanceURL + "/_kill").Methods(http.MethodPost).Handler(s.kill())
	s.router.Path(instanceURL + "/_command").Methods(http.MethodPost).Handler(s.command())
	s.router.Path(instanceURL + "/_upload").Methods(http.MethodPost).Handler(s.uploadFile())

	if s.enableUI {
		s.registerUIRoutes()
	}
}

func (s *Server) fileServing(directory string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		file := mux.Vars(r)["file_name"]
		disableWriteDeadline(w)
		noCache(w)
		http.ServeFile(w, r, path.Join(directory, instance, file))
	}
}

// noCache makes browsers revalidate an instance file on every request.
// ServeFile sends Last-Modified but no Cache-Control, which lets browsers
// cache heuristically: the web UI then reopened a just-saved config.json
// from its cache and showed the previous version. Instance files change
// with every save and run, and revalidation stays cheap via
// If-Modified-Since.
func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache")
}

// Instance status values of the instance status and listing.
const (
	// StatusStarted: the bngblaster instance is running.
	StatusStarted = "started"
	// StatusStopped: the instance is not running.
	StatusStopped = "stopped"
)

// InstanceStatus is the response of GET /api/v1/instances/<name>, see
// status.
type InstanceStatus struct {
	// Status is StatusStarted or StatusStopped.
	Status string `json:"status"`
	// ExitCode is the exit code of the last run once stopped, if known.
	ExitCode *int `json:"exit_code,omitempty"`
	// Autostart is the state of the autostart, for its instance only.
	Autostart *controller.AutostartStatus `json:"autostart,omitempty"`
}

// instanceDetail is one entry of the detailed instance listing.
type instanceDetail struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// instances lists the configured instances. By default this is the plain
// array of names it has always been; "?detail=true" instead returns each
// name together with its status.
//
// The dashboard refreshes its table every few seconds and needs the status
// of every instance, which previously meant one request for the list plus
// one per instance on every refresh. The detailed form collapses that into
// a single request.
func (s *Server) instances() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instances := s.repository.Instances()
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("detail") != "true" {
			_ = json.NewEncoder(w).Encode(instances)
			return
		}
		details := make([]instanceDetail, 0, len(instances))
		for _, name := range instances {
			status := StatusStopped
			if s.repository.Running(name) {
				status = StatusStarted
			}
			details = append(details, instanceDetail{Name: name, Status: status})
		}
		_ = json.NewEncoder(w).Encode(details)
	}
}

// invalidateInstanceCaches drops every cached summary belonging to an
// instance. Called whenever its lifecycle changes so a start/stop/kill/delete
// is reflected immediately instead of after the cache period, and so a
// deleted instance leaves nothing cached behind it.
func (s *Server) invalidateInstanceCaches(instance string) {
	s.streamCache.invalidate(instance)
	s.sessionCache.invalidate(instance)
	s.overviewCache.invalidate(instance)
}

// getReadableInterfaceFlags converts interface flags to a readable format.
func getReadableInterfaceFlags(flags net.Flags) []string {
	var readableFlags []string
	flagMap := map[net.Flags]string{
		net.FlagUp:           "up",
		net.FlagBroadcast:    "broadcast",
		net.FlagLoopback:     "loopback",
		net.FlagPointToPoint: "point-to-point",
		net.FlagMulticast:    "multicast",
	}

	for flag, desc := range flagMap {
		if flags&flag != 0 {
			readableFlags = append(readableFlags, desc)
		}
	}

	return readableFlags
}

// getInterfaces returns a list of all network interfaces.
func getInterfaces() []InterfaceInfo {
	var interfacesInfo []InterfaceInfo

	// Get the list of network interfaces.
	interfaces, err := net.Interfaces()
	if err != nil {
		return interfacesInfo // Return the empty slice if there's an error.
	}

	// Populate InterfaceInfo for each interface.
	for _, iface := range interfaces {
		info := InterfaceInfo{
			Name:  iface.Name,
			MTU:   iface.MTU,
			Flags: getReadableInterfaceFlags(iface.Flags),
			Mac:   iface.HardwareAddr.String(),
		}
		interfacesInfo = append(interfacesInfo, info)
	}

	return interfacesInfo
}

func (s *Server) interfaces() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		interfaces := getInterfaces()
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(interfaces)
	}
}

// getVersion returns server and bngblaster version informations.
func getVersion(s *Server) VersionInfo {
	versionInfo := VersionInfo{
		Version:         s.Version,
		BlasterVersion:  "NA",
		BlasterCompiler: "NA",
	}

	cmd := exec.Command(s.repository.Executable(), "-v")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return versionInfo
	}

	lines := strings.SplitSeq(out.String(), "\n")
	for line := range lines {
		if after, ok := strings.CutPrefix(line, "Version:"); ok {
			versionInfo.BlasterVersion = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "Compiler:"); ok {
			versionInfo.BlasterCompiler = strings.TrimSpace(after)
		} else if strings.HasPrefix(line, "IO Modes:") {
			ioModes := strings.TrimSpace(strings.TrimPrefix(strings.Replace(line, " (default)", "", 1), "IO Modes:"))
			versionInfo.BlasterIOModes = strings.Split(ioModes, ", ")
		}
	}

	return versionInfo
}

func (s *Server) version() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		version := getVersion(s)
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(version)
	}
}

// schema serves the bngblaster configuration JSON schema used by the web UI
// to render and validate the "New Instance" config editor.
func (s *Server) schema() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		content, err := os.ReadFile(s.schemaPath)
		if err != nil {
			JSONError(w, "schema not available", http.StatusNotFound)
			return
		}
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

func (s *Server) create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxConfigSize))
		if isBodyTooLarge(err) {
			http.Error(w, "config too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil || len(content) == 0 {
			http.Error(w, "body not readable", http.StatusBadRequest)
			return
		}
		status := http.StatusCreated
		if s.repository.Exists(instance) {
			status = http.StatusNoContent
		}
		err = s.repository.Create(instance, content)
		if errors.Is(err, controller.ErrBlasterRunning) {
			http.Error(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			http.Error(w, "not able to create instance", http.StatusInternalServerError)
			return
		}
		audit(r, instance, "configured")
		w.WriteHeader(status)
	}
}

// status reports whether an instance is running ("started") or not
// ("stopped"), the exit code of its last run once stopped, if known, and
// for the autostart instance the state of the autostart. The autostart
// instance has a status even before it exists, so that a health check can
// tell waiting for interfaces and a failed start from an unknown instance.
func (s *Server) status() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		var resp InstanceStatus
		if s.autostart != nil && s.autostart.Instance() == instance {
			autostart := s.autostart.Status()
			resp.Autostart = &autostart
		}
		if resp.Autostart == nil && !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		resp.Status = StatusStopped
		if s.repository.Running(instance) {
			resp.Status = StatusStarted
		} else if code, ok := s.repository.ExitCode(instance); ok {
			resp.ExitCode = &code
		}
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(&resp)
	}
}

func (s *Server) delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusNoContent
		s.invalidateInstanceCaches(instance)
		err := s.repository.Delete(instance)
		if errors.Is(err, controller.ErrBlasterRunning) {
			JSONError(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if errors.Is(err, controller.ErrGeneratorBusy) {
			JSONError(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			JSONError(w, "not able to delete instance", http.StatusInternalServerError)
			return
		}
		audit(r, instance, "deleted")
		w.WriteHeader(status)
	}
}

func (s *Server) start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		var runningConfig controller.RunningConfig
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestSize)).Decode(&runningConfig)
		if isBodyTooLarge(err) {
			JSONError(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		status := http.StatusNoContent

		// Start waits until the instance has come up or failed, which has
		// no upper bound (e.g. while waiting for the startup of other
		// instances); the instance is aborted via _stop or _kill instead.
		disableWriteDeadline(w)
		s.invalidateInstanceCaches(instance)
		err = s.repository.Start(r.Context(), instance, runningConfig)
		if errors.Is(err, controller.ErrBlasterNotExists) {
			JSONNotFound(w, r)
			return
		}
		if errors.Is(err, controller.ErrBlasterRunning) {
			JSONError(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if errors.Is(err, controller.ErrBlasterStartupAborted) {
			audit(r, instance, "start aborted")
			JSONError(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, controller.ErrInvalidStreamConfig) {
			audit(r, instance, "start rejected").Str("stream_config", runningConfig.StreamConfig)
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err != nil {
			audit(r, instance, "start failed")
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		audit(r, instance, "started")
		w.WriteHeader(status)
	}
}

func (s *Server) stop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusAccepted
		s.repository.Stop(instance)
		s.invalidateInstanceCaches(instance)
		audit(r, instance, "stop requested")
		w.WriteHeader(status)
	}
}

func (s *Server) kill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusAccepted
		s.repository.Kill(instance)
		s.invalidateInstanceCaches(instance)
		audit(r, instance, "kill requested")
		w.WriteHeader(status)
	}
}

func (s *Server) command() http.HandlerFunc {
	type commandResponse struct {
		Code int `json:"code"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		var command controller.SocketCommand
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestSize)).Decode(&command)
		if isBodyTooLarge(err) {
			JSONError(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		// The web UI issues read-only commands on every poll; those count as
		// reads in the access log, every other command as an action.
		rl := logFields(r).Str("command", command.Command)
		if _, ok := readOnlyCommands[command.Command]; ok {
			rl.readOnly = true
		} else {
			rl.action = "command"
		}

		status := http.StatusOK

		result, err := s.repository.Command(instance, command)
		if errors.Is(err, controller.ErrBlasterNotExists) {
			JSONNotFound(w, r)
			return
		}
		if errors.Is(err, controller.ErrBlasterNotRunning) {
			JSONError(w, "instance is not running", http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			JSONError(w, "not able to send command", http.StatusInternalServerError)
			return
		}
		var cr commandResponse
		err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if cr.Code != 0 {
			status = cr.Code
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(status)
		_, _ = w.Write(result)
	}
}

func (s *Server) uploadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			http.Error(w, "instance not found", http.StatusNotFound)
			return
		}

		if !s.repository.AllowUpload() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Reading a multi-GB body counts against the write timeout too.
		disableWriteDeadline(w)
		instanceFolder := filepath.Join(s.repository.ConfigFolder(), instance)
		if r.ContentLength > 0 {
			if available, ok := availableDiskSpace(instanceFolder); ok && uint64(r.ContentLength) > available {
				logFields(r).Int64("size", r.ContentLength).Int64("available", int64(available)) //nolint:gosec // disk sizes fit
				http.Error(w, "insufficient disk space", http.StatusInsufficientStorage)
				return
			}
		}

		// The multipart body is streamed straight into the instance folder
		// rather than parsed with ParseMultipartForm, which would spool it
		// through os.TempDir (often a small tmpfs) first and does not bound
		// the total size at all. MaxBytesReader does.
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		part, err := uploadFilePart(r)
		if isBodyTooLarge(err) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			http.Error(w, "error retrieving file", http.StatusBadRequest)
			return
		}
		defer part.Close()

		// Only ever the base name. Part.FileName already strips directory
		// components (RFC 7578 requires it), so this is belt and braces -
		// but the guarantee that an upload cannot escape the instance folder
		// is worth stating at the point where the path is built rather than
		// relying on a library's behavior. Base's own degenerate outputs
		// ("", ".", "..", "/") are rejected outright since joining any of
		// them would land outside the instance folder.
		name := filepath.Base(part.FileName())
		if isUnsafeFileName(name) {
			logFields(r).Str("file", part.FileName())
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}
		if controller.IsReservedFile(name) {
			logFields(r).Str("file", name)
			http.Error(w, "reserved filename", http.StatusBadRequest)
			return
		}

		err = saveUpload(instanceFolder, name, part)
		switch {
		case isBodyTooLarge(err):
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		case errors.Is(err, syscall.ENOSPC):
			logFields(r).Str("file", name)
			http.Error(w, "insufficient disk space", http.StatusInsufficientStorage)
			return
		case err != nil:
			http.Error(w, "failed to save file", http.StatusInternalServerError)
			return
		}

		audit(r, instance, "uploaded").Str("file", name)

		w.WriteHeader(http.StatusOK)
	}
}

// uploadFilePart returns the multipart part carrying the uploaded file (the
// "file" field), skipping any other form fields before it.
func uploadFilePart(r *http.Request) (*multipart.Part, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" && part.FileName() != "" {
			return part, nil
		}
	}
}

// saveUpload stores src as folder/name. It writes to a temporary file next
// to the target and renames it into place, so an aborted, oversized or
// disk-full upload never leaves a truncated file behind or clobbers the
// previous version.
func saveUpload(folder, name string, src io.Reader) error {
	tmpFile, err := os.CreateTemp(folder, ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name()) // no-op once renamed
	_, err = io.Copy(tmpFile, src)
	if closeErr := tmpFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpFile.Name(), filepath.Join(folder, name))
}

type message struct {
	Message any `json:"message"`
}

// JSONError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
// The error message should be plain text.
func JSONError(w http.ResponseWriter, err any, code int) {
	m := &message{Message: err}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(m)
}

// JSONNotFound replies to the request with an HTTP 404 not found error.
func JSONNotFound(w http.ResponseWriter, _ *http.Request) {
	JSONError(w, "404 page not found", http.StatusNotFound)
}
