package main

import (
	"flag"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/rtbrick/bngblaster-controller/pkg/controller"
	"github.com/rtbrick/bngblaster-controller/pkg/daemonize"
	"github.com/rtbrick/bngblaster-controller/pkg/server"
)

var Version = "dev"

func main() {
	addr := flag.String("addr", ":8001", "HTTP network address")
	directory := flag.String("d", controller.DefaultConfigFolder, "config folder")
	executable := flag.String("e", controller.DefaultExecutable, "bngblaster executable")
	upload := flag.Bool("upload", true, "disable file upload")
	ui := flag.Bool("ui", true, "disable the embedded web UI")
	interfacesAPI := flag.Bool("interfaces-api", true, "disable the interfaces endpoint")
	schema := flag.String("schema", server.DefaultSchemaPath, "path to the bngblaster configuration JSON schema served on /api/v1/schema")

	// logging
	debug := flag.Bool("debug", false, "turn on debug logging")
	console := flag.Bool("console", true, "turn on pretty console logging")
	color := flag.Bool("color", false, "turn on color of color output")

	flag.Parse()

	// setup logging
	initializeLogger(*debug, *console, *color)

	repo := controller.NewDefaultRepository(
		controller.WithConfigFolder(*directory),
		controller.WithExecutable(*executable),
		controller.WithUpload(*upload))
	srv := server.NewServer(repo,
		server.WithUI(*ui),
		server.WithInterfacesAPI(*interfacesAPI),
		server.WithSchemaPath(*schema))
	srv.Version = Version
	serve(*addr, srv)
}

func serve(addr string, handler http.Handler) {
	const idleTimeout = time.Second * 80
	const writeTimeout = time.Second * 40
	const readHeaderTimeout = time.Second * 40
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	log.Info().Msgf("Starting server on %s\n", addr)
	sig, err := daemonize.Daemonize(func() error { return srv.ListenAndServe() })
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	log.Info().Msgf("Shutdown server on signal %s\n", sig)
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

	log.Logger = zerolog.New(levelSplitWriter{out: out, errOut: errOut}).With().Timestamp().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
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
