// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2024, RtBrick, Inc.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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

// Server implementation for the rest api.
type Server struct {
	Version    string
	router     *mux.Router
	prom       *controller.Prom
	repository controller.Repository
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
func NewServer(repository controller.Repository) *Server {
	r := &Server{
		Version:    "dev",
		router:     mux.NewRouter(),
		prom:       controller.NewProm(repository),
		repository: repository,
	}
	r.routes()
	return r
}

// ServeHTTP A Handler responds to an HTTP request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do stuff here.
		log.Info().Str("method", r.Method).Msg(r.RequestURI)
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	const instanceURL = "/api/v1/instances/{instance_name}"
	s.router.Use(loggingMiddleware)
	// Expose the registered metrics via HTTP.
	s.router.Path("/metrics").Methods(http.MethodGet).Handler(promhttp.HandlerFor(
		s.prom.Registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	))
	s.router.Path("/api/v1/version").Methods(http.MethodGet).Handler(s.version())
	s.router.Path("/api/v1/interfaces").Methods(http.MethodGet).Handler(s.interfaces())
	s.router.Path("/api/v1/instances").Methods(http.MethodGet).Handler(s.instances())
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
}

func (s *Server) fileServing(directory string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		file := mux.Vars(r)["file_name"]
		http.ServeFile(w, r, path.Join(directory, instance, file))
	}
}

func (s *Server) instances() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instances := s.repository.Instances()
		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(instances)
	}
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

	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version:") {
			versionInfo.BlasterVersion = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		} else if strings.HasPrefix(line, "Compiler:") {
			versionInfo.BlasterCompiler = strings.TrimSpace(strings.TrimPrefix(line, "Compiler:"))
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

func (s *Server) create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		content, err := io.ReadAll(r.Body)
		if err != nil || len(content) == 0 {
			http.Error(w, "body not readable", http.StatusBadRequest)
			return
		}
		status := http.StatusCreated
		if s.repository.Exists(instance) {
			status = http.StatusNoContent
		}
		err = s.repository.Create(instance, content)
		if err == controller.ErrBlasterRunning {
			http.Error(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			http.Error(w, "not able to create instance", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(status)
	}
}

func (s *Server) status() http.HandlerFunc {
	type response struct {
		Status string `json:"status"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		if !s.repository.Exists(instance) {
			JSONNotFound(w, r)
			return
		}

		w.Header().Set(contentType, applicationJSON)
		w.WriteHeader(http.StatusOK)
		if s.repository.Running(instance) {
			_ = json.NewEncoder(w).Encode(&response{Status: "started"})
		} else {
			_ = json.NewEncoder(w).Encode(&response{Status: "stopped"})
		}
	}
}

func (s *Server) delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusNoContent
		err := s.repository.Delete(instance)
		if err == controller.ErrBlasterRunning {
			JSONError(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			JSONError(w, "not able to delete instance", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(status)
	}
}

func (s *Server) start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		var runningConfig controller.RunningConfig
		err := json.NewDecoder(r.Body).Decode(&runningConfig)
		if err != nil {
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		status := http.StatusNoContent

		err = s.repository.Start(instance, runningConfig)
		if err == controller.ErrBlasterNotExists {
			JSONNotFound(w, r)
			return
		}
		if err == controller.ErrBlasterRunning {
			JSONError(w, errInstanceIsRunning, http.StatusPreconditionFailed)
			return
		}
		if err != nil {
			JSONError(w, "not able to start", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(status)
	}
}

func (s *Server) stop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusAccepted
		s.repository.Stop(instance)
		w.WriteHeader(status)
	}
}

func (s *Server) kill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceVariable := mux.Vars(r)[instanceNameParameter]
		instance := cleanPathVariable(instanceVariable)
		status := http.StatusAccepted
		s.repository.Kill(instance)
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
		err := json.NewDecoder(r.Body).Decode(&command)
		if err != nil {
			JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		status := http.StatusOK

		result, err := s.repository.Command(instance, command)
		if err == controller.ErrBlasterNotExists {
			JSONNotFound(w, r)
			return
		}
		if err == controller.ErrBlasterNotRunning {
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

		err := r.ParseMultipartForm(4000 << 20) // Max upload size set to 4000 MB
		if err != nil {
			http.Error(w, "error parsing multipart form", http.StatusRequestEntityTooLarge)
			return
		}

		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "error retrieving file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		filePath := filepath.Join(s.repository.ConfigFolder(), instance, handler.Filename)

		destFile, err := os.Create(filePath)
		if err != nil {
			http.Error(w, "failed to create file", http.StatusInternalServerError)
			return
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, file)
		if err != nil {
			http.Error(w, "failed to save file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

type message struct {
	Message interface{} `json:"message"`
}

// JSONError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
// The error message should be plain text.
func JSONError(w http.ResponseWriter, err interface{}, code int) {
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
