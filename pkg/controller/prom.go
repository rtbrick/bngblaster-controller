package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

const (
	metricInstancesTotal   = "instances_total"
	metricInstancesRunning = "instances_running"

	metricSessions = "sessions"

	labelHostname     = "hostname"
	labelInstanceName = "instance_name"
)

// Prom prometheus exports for this package
type Prom struct {
	Registry   *prometheus.Registry
	repository Repository

	// metrics
	instancesTotal   *prometheus.Desc
	instancesRunning *prometheus.Desc
	sessions         *prometheus.Desc
}

func NewProm(repository Repository) *Prom {
	p := &Prom{
		Registry:   prometheus.NewRegistry(),
		repository: repository,
	}
	// Get system hostname.
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	// Init metrics.
	p.instancesTotal = prometheus.NewDesc(metricInstancesTotal,
		"The total number of instances",
		nil, prometheus.Labels{labelHostname: hostname},
	)
	p.instancesRunning = prometheus.NewDesc(metricInstancesRunning,
		"The number of running instances",
		nil, prometheus.Labels{labelHostname: hostname},
	)
	p.sessions = prometheus.NewDesc(metricSessions,
		"The number of sessions",
		[]string{labelInstanceName}, prometheus.Labels{labelHostname: hostname},
	)

	// Register metrics and return.
	p.Registry.MustRegister(p)
	return p
}

// Describe to describe all the metrics.
func (p *Prom) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.instancesTotal
	ch <- p.instancesRunning
	ch <- p.sessions
}

// Collect implements required collect function for all metrics collectors.
func (p *Prom) Collect(ch chan<- prometheus.Metric) {
	total := float64(0)
	running := float64(0)

	var wg sync.WaitGroup

	instances, _ := ioutil.ReadDir(p.repository.ConfigFolder())
	for _, instance := range instances {
		if instance.IsDir() {
			total++
			if p.repository.Running(instance.Name()) {
				running++
				wg.Add(1)
				go p.collectInstance(&wg, instance.Name(), ch)
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(p.instancesTotal, prometheus.GaugeValue, total)
	ch <- prometheus.MustNewConstMetric(p.instancesRunning, prometheus.GaugeValue, running)
	wg.Wait()
}

func (p *Prom) collectInstanceSessionCounters(instance string, ch chan<- prometheus.Metric) {
	type commandResponse struct {
		Code            int `json:"code"`
		SessionCounters struct {
			Sessions               int `json:"sessions"`
			SessionsPPPoE          int `json:"sessions-pppoe"`
			SessionsIPoE           int `json:"sessions-ipoe"`
			SessionsEstablished    int `json:"sessions-established"`
			SessionsEstablishedMax int `json:"sessions-established-max"`
			SessionsTerminated     int `json:"sessions-terminated"`
			SessionsFlapped        int `json:"sessions-flapped"`
		} `json:"session-counters"`
	}

	command := SocketCommand{
		Command: "session-counters",
	}
	result, err := p.repository.Command(instance, command)
	if err != nil {
		return
	}

	var cr commandResponse
	err = json.NewDecoder(strings.NewReader(string(result))).Decode(&cr)
	if err != nil {
		log.Warn().Msgf("failed to decode session-counters: %s", err.Error())
		return
	}

	ch <- prometheus.MustNewConstMetric(p.sessions, prometheus.GaugeValue, float64(cr.SessionCounters.Sessions), instance)
}

func (p *Prom) collectInstance(wg *sync.WaitGroup, instance string, ch chan<- prometheus.Metric) {
	defer wg.Done()

	folder := path.Join(p.repository.ConfigFolder(), instance)
	path := path.Join(folder, RunConfigFilename)
	file, err := os.Open(path)
	if err != nil {
		log.Warn().Msgf("failed to open %s: %s", path, err.Error())
		fmt.Println(err)
		return
	}

	var runningConfig RunningConfig
	err = json.NewDecoder(file).Decode(&runningConfig)
	if err != nil {
		log.Warn().Msgf("failed to decode %s: %s", path, err.Error())
		return
	}

	for _, flag := range runningConfig.MetricFlags {
		switch flag {
		case "session_counters":
			p.collectInstanceSessionCounters(instance, ch)
		default:
			log.Warn().Msgf("unknown metrics flag: %s", flag)
		}
	}
}
