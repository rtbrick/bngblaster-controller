package controller

import (
	"io/ioutil"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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

func (p *Prom) collectInstance(wg *sync.WaitGroup, instance string, ch chan<- prometheus.Metric) {
	defer wg.Done()

	ch <- prometheus.MustNewConstMetric(p.sessions, prometheus.GaugeValue, 1337, instance)
}
