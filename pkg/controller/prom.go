package controller

import (
	"io/ioutil"
	"os"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricInstancesTotal   = "instances_total"
	metricInstancesRunning = "instances_running"
	labelHostName          = "hostname"
	labelInstanceName      = "instance_name"
)

// Prom prometheus exports for this package
type Prom struct {
	Registry   *prometheus.Registry
	repository Repository

	// metrics
	instancesTotal   *prometheus.Desc
	instancesRunning *prometheus.Desc
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
		nil, prometheus.Labels{"hostname": hostname},
	)
	p.instancesRunning = prometheus.NewDesc(metricInstancesRunning,
		"The number of running instances",
		nil, prometheus.Labels{"hostname": hostname},
	)

	// Register metrics and return.
	p.Registry.MustRegister(p)
	return p
}

// Describe to describe all the metrics.
func (p *Prom) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.instancesTotal
	ch <- p.instancesRunning
}

// Collect implements required collect function for all metrics collectors.
func (p *Prom) Collect(ch chan<- prometheus.Metric) {
	instances := float64(0)
	running := float64(0)

	items, _ := ioutil.ReadDir(p.repository.ConfigFolder())
	for _, item := range items {
		if item.IsDir() {
			instances++
			if p.repository.Running(item.Name()) {
				running++
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(p.instancesTotal, prometheus.GaugeValue, instances)
	ch <- prometheus.MustNewConstMetric(p.instancesRunning, prometheus.GaugeValue, running)
}
