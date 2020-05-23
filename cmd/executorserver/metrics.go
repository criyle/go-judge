package main

import (
	"time"

	"github.com/criyle/go-judge/worker"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "executorserver"
)

var (
	// 1ms -> 10s
	timeBuckets = []float64{
		0.001, 0.002, 0.005, 0.008, 0.010, 0.025, 0.050, 0.075, 0.1, 0.2,
		0.4, 0.6, 0.8, 1.0, 1.5, 2, 5, 10,
	}

	// 4k (1<<12) -> 1g (1<<30)
	memoryBucket = prometheus.ExponentialBuckets(1<<12, 2, 19)

	metricsSummaryQuantile = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}

	execErrorCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "error",
		Help:      "Number of exec query returns error",
	})

	execTimeHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Name:      "time_seconds",
		Help:      "Histogram for the running time",
		Buckets:   timeBuckets,
	}, []string{"status"})

	execTimeSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Name:       "time",
		Help:       "Summary for the running time",
		Objectives: metricsSummaryQuantile,
	}, []string{"status"})

	execMemHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Name:      "memory_bytes",
		Help:      "Histgram for the memory",
		Buckets:   memoryBucket,
	}, []string{"status"})

	execMemSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Name:       "memory",
		Help:       "Summary for the memory",
		Objectives: metricsSummaryQuantile,
	}, []string{"status"})
)

func init() {
	prometheus.MustRegister(execErrorCount)
	prometheus.MustRegister(execTimeHist, execTimeSummary)
	prometheus.MustRegister(execMemHist, execMemSummary)
}

func execObserve(res worker.Response) {
	if res.Error != nil {
		execErrorCount.Inc()
	}
	for _, r := range res.Results {
		status := r.Status.String()
		d := time.Duration(r.Time)
		ob := d.Seconds()
		mob := float64(r.Memory)
		execTimeHist.WithLabelValues(status).Observe(ob)
		execTimeSummary.WithLabelValues(status).Observe(ob)
		execMemHist.WithLabelValues(status).Observe(mob)
		execMemSummary.WithLabelValues(status).Observe(mob)
	}
}
