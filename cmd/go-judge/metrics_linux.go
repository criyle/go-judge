package main

import (
	"path/filepath"
	"time"

	"github.com/criyle/go-judge/cmd/go-judge/config"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	"github.com/prometheus/client_golang/prometheus"
)

const cgroupSubsystem = "cgroup"

var _ prometheus.Collector = &cgroupMetrics{}

type cgroupMetrics struct {
	cgroup          cgroup.Cgroup
	cgroupCPU       *prometheus.Desc
	cgroupMemory    *prometheus.Desc
	cgroupMaxMemory *prometheus.Desc
}

// Collect implements prometheus.Collector.
func (c *cgroupMetrics) Collect(ch chan<- prometheus.Metric) {
	if u, err := c.cgroup.CPUUsage(); err == nil {
		ch <- prometheus.MustNewConstMetric(
			c.cgroupCPU, prometheus.CounterValue, time.Duration(u).Seconds(),
		)
	}
	if m, err := c.cgroup.MemoryUsage(); err == nil {
		ch <- prometheus.MustNewConstMetric(
			c.cgroupMemory, prometheus.GaugeValue, float64(m),
		)
	}
	if m, err := c.cgroup.MemoryMaxUsage(); err == nil {
		ch <- prometheus.MustNewConstMetric(
			c.cgroupMaxMemory, prometheus.GaugeValue, float64(m),
		)
	}
}

// Describe implements prometheus.Collector.
func (c *cgroupMetrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func newCgroupMetrics(cg cgroup.Cgroup, label string) *cgroupMetrics {
	cgroupCPU := prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, cgroupSubsystem, "cpu_seconds"),
		"CPU usage of the cgroup", nil, prometheus.Labels{"type": label},
	)
	cgroupMemory := prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, cgroupSubsystem, "memory_bytes"),
		"Memory usage of the cgroup", nil, prometheus.Labels{"type": label},
	)
	cgroupMaxMemory := prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, cgroupSubsystem, "memory_max_bytes"),
		"Maximum memory usage of the cgroup", nil, prometheus.Labels{"type": label},
	)
	rt := &cgroupMetrics{
		cgroup:          cg,
		cgroupCPU:       cgroupCPU,
		cgroupMemory:    cgroupMemory,
		cgroupMaxMemory: cgroupMaxMemory,
	}
	prometheus.MustRegister(rt)
	return rt
}

func initCgroupMetrics(conf *config.Config, param map[string]any) {
	if !conf.EnableMetrics {
		return
	}
	t, ok := param["cgroupType"]
	if !ok {
		return
	}
	ct, ok := t.(int)
	if !ok {
		return
	}
	if ct != cgroup.TypeV1 && ct != cgroup.TypeV2 {
		return
	}

	prefix, err := cgroup.GetCurrentCgroupPrefix()
	if err != nil {
		return
	}

	// current cgroup is xxx/api, get the dir
	prefix = filepath.Dir(prefix)
	control, err := cgroup.GetAvailableControllerWithPrefix(prefix)
	if err != nil {
		return
	}

	cg, err := cgroup.New(prefix, control)
	if err != nil {
		return
	}
	newCgroupMetrics(cg, "all")

	apiCg, err := cg.New("api")
	if err != nil {
		return
	}
	newCgroupMetrics(apiCg, "controller")

	containersCg, err := cgroup.New(filepath.Join(prefix, "containers"), control)
	if err != nil {
		return
	}
	newCgroupMetrics(containersCg, "containers")
}
