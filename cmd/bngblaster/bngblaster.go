package main

import (
	"flag"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/rs/zerolog/log"
)

func echoServer(c net.Conn) {
	for {
		buf := make([]byte, 512)
		nr, err := c.Read(buf)
		if err != nil {
			return
		}

		data := buf[0:nr]
		log.Info().Msgf("Server got: %s", string(data))
		_, err = c.Write(data)
		if err != nil {
			log.Fatal().Msgf("Writing client error: %v", err)
		}
		_ = c.Close()
	}
}

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, " ")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}
func main() {
	initializeLogger(true, true, false)

	//Flags
	var logging arrayFlags
	config := flag.String("C", "", "config")
	logfile := flag.String("L", "", "log-file")
	flag.Var(&logging, "l", "logging")
	pcap := flag.String("P", "", "pcap-capture")
	socket := flag.String("S", "", "control socket")

	flag.Parse()

	log.Info().Str("C", *config).Msgf("got")
	log.Info().Str("L", *logfile).Msgf("got")
	log.Info().Strs("l", logging).Msgf("got")
	log.Info().Str("P", *pcap).Msgf("got")
	log.Info().Str("S", *socket).Msgf("got")

	log.Info().Msg("Starting echo server")
	ln, err := net.Listen("unix", *socket)
	if err != nil {
		log.Fatal().Msgf("Listen error: %v", err)
	}
	log.Info().Msgf("PID: %d", os.Getpid())
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM, os.Kill)
	go func(ln net.Listener, c chan os.Signal) {
		sig := <-c
		log.Info().Msgf("Caught signal %s: shutting down.", sig)
		_ = ln.Close()
		os.Exit(0)
	}(ln, sigc)

	for {
		fd, err := ln.Accept()
		if err != nil {
			log.Fatal().Msgf("Accept error: %v", err)
		}

		go echoServer(fd)
	}
}

func initializeLogger(debug, console bool, color bool) {
	var w io.Writer
	w = os.Stdout
	if console {
		w = zerolog.ConsoleWriter{
			Out:        os.Stdout,
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
