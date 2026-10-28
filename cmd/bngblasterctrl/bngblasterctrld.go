// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2026, RtBrick, Inc.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/daemonize"
	"github.com/rtbrick/bngblaster-controller/pkg/generator"
	"github.com/rtbrick/bngblaster-controller/pkg/server"
)

var Version = "dev"

func main() {
	addr := flag.String("addr", ":8001", "HTTP network address")
	directory := flag.String("d", controller.DefaultConfigFolder, "config folder")
	executable := flag.String("e", controller.DefaultExecutable, "bngblaster executable")
	upload := flag.Bool("upload", true, "enable file upload (disable with -upload=false)")
	ui := flag.Bool("ui", true, "enable the embedded web UI (experimental, disable with -ui=false)")
	interfacesAPI := flag.Bool("interfaces-api", true, "enable the interfaces endpoint (disable with -interfaces-api=false)")
	hostAPI := flag.Bool("host-api", true, "enable the host information endpoints (disable with -host-api=false)")
	generators := flag.Bool("generators", true, "enable the lspgen, bgpupdate and ldpupdate generator endpoints (disable with -generators=false)")
	lspgen := flag.String("lspgen", defaultGeneratorExecutable(generator.LSPGen), "lspgen executable")
	bgpupdate := flag.String("bgpupdate", defaultGeneratorExecutable(generator.BGPUpdate), "bgpupdate executable")
	ldpupdate := flag.String("ldpupdate", defaultGeneratorExecutable(generator.LDPUpdate), "ldpupdate executable")
	schema := flag.String("schema", server.DefaultSchemaPath, "path to the bngblaster configuration JSON schema served on /api/v1/schema")
	allowedHosts := flag.String("allowed-hosts", "",
		"comma-separated host names clients may use to reach the controller, against DNS rebinding "+
			"(IP addresses and localhost are always allowed; empty allows any host)")

	// logging
	debug := flag.Bool("debug", false, "turn on debug logging")
	console := flag.Bool("console", true, "turn on pretty console logging")
	color := flag.Bool("color", false, "turn on color of color output")
	logUI := flag.Bool("log-ui", false,
		"also log successful read requests of the web UI (its polling) at info level; "+
			"failed requests and actions are always logged")

	flag.Parse()

	// setup logging
	initializeLogger(*debug, *console, *color)

	repo := controller.NewDefaultRepository(
		controller.WithConfigFolder(*directory),
		controller.WithExecutable(*executable),
		controller.WithUpload(*upload))
	var generatorExecutables map[string]string
	if *generators {
		generatorExecutables = map[string]string{
			generator.LSPGen:    *lspgen,
			generator.BGPUpdate: *bgpupdate,
			generator.LDPUpdate: *ldpupdate,
		}
		repo.RemoveGeneratorTempFiles()
	}
	srv := server.NewServer(repo,
		server.WithGenerators(generatorExecutables),
		server.WithUI(*ui),
		server.WithQuietUIRequests(!*logUI),
		server.WithInterfacesAPI(*interfacesAPI),
		server.WithHostAPI(*hostAPI),
		server.WithSchemaPath(*schema),
		server.WithAllowedHosts(splitList(*allowedHosts)))
	srv.Version = Version
	serve(*addr, srv)
}

// defaultGeneratorExecutable returns the default executable of a generator tool.
func defaultGeneratorExecutable(name string) string {
	tool, _ := generator.Lookup(name)
	return tool.DefaultExecutable
}

// splitList splits a comma-separated flag value, dropping empty entries.
func splitList(value string) []string {
	var items []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func serve(addr string, handler http.Handler) {
	const idleTimeout = time.Second * 80
	const writeTimeout = time.Second * 40
	const readHeaderTimeout = time.Second * 40
	const shutdownTimeout = time.Second * 30
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	log.Info().Msgf("Starting server on %s\n", addr)
	sig, err := daemonize.Daemonize(func() error { return srv.ListenAndServe() })
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal().Err(err).Send()
	}
	log.Info().Msgf("Shutdown server on signal %s\n", sig)

	// Let in-flight requests (e.g. a start/stop call or a running download)
	// finish instead of cutting them off; systemd's default stop timeout is
	// 90s, so stay well below it.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("graceful shutdown incomplete")
	}
}

func initializeLogger(debug, console bool, color bool) {
	var out, errOut io.Writer = os.Stdout, os.Stderr
	if console {
		out = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    !color,
			TimeFormat: "2006-01-02 15:04:05 MST",
		}
		errOut = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			NoColor:    !color,
			TimeFormat: "2006-01-02 15:04:05 MST",
		}
	}

	// The source location only helps when debugging; in normal operation
	// it is noise on every line.
	logCtx := zerolog.New(levelSplitWriter{out: out, errOut: errOut}).With().Timestamp()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.CallerMarshalFunc = shortCaller
		logCtx = logCtx.Caller()
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = logCtx.Logger()
}

// shortCaller reduces the source location to package folder, file and
// line ("server/accesslog.go:210"): the module path in front of it is the
// same on every line.
func shortCaller(file string, line int) string {
	dir, name := filepath.Split(file)
	return filepath.Join(filepath.Base(dir), name) + ":" + strconv.Itoa(line)
}

// levelSplitWriter routes warn/error/fatal/panic records to errOut and
// everything below (info/debug/trace) to out, so the systemd unit's separate
// stdout/stderr log files actually separate normal activity from problems
// instead of funneling every record into one of them.
type levelSplitWriter struct {
	out    io.Writer
	errOut io.Writer
}

func (w levelSplitWriter) Write(p []byte) (int, error) {
	return w.out.Write(p)
}

func (w levelSplitWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level >= zerolog.WarnLevel {
		return w.errOut.Write(p)
	}
	return w.out.Write(p)
}
