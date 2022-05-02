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

func main() {
	addr := flag.String("addr", ":8001", "HTTP network address")
	directory := flag.String("d", controller.DefaultConfigFolder, "config folder")
	executable := flag.String("e", controller.DefaultExecutable, "bngblaster executable")

	// logging
	debug := flag.Bool("debug", false, "turn on debug logging")
	console := flag.Bool("console", true, "turn on pretty console logging")
	color := flag.Bool("color", false, "turn on color of color output")

	flag.Parse()

	// setup logging
	initializeLogger(*debug, *console, *color)

	repo := controller.NewDefaultRepository(
		controller.WithConfigFolder(*directory),
		controller.WithExecutable(*executable))
	srv := server.NewServer(repo)
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
	var w io.Writer
	w = os.Stderr
	if console {
		w = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			NoColor:    !color,
			TimeFormat: "2006-01-02 15:04:05 MST",
		}
	}

	log.Logger = zerolog.New(w).With().Timestamp().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}
