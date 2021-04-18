package main

import (
	"sync"
	"time"

	"github.com/criyle/go-judge/filestore"
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

	// 4k (1<<12) -> 4g (1<<32)
	memoryBucket = prometheus.ExponentialBuckets(1<<12, 2, 21)
	// 256 byte (1<<8) -> 256m (1<<28)
	fileSizeBucket = prometheus.ExponentialBuckets(1<<8, 2, 20)

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

	fsSizeHist = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Name:      "file_size_bytes",
		Help:      "Histgram for the file size in the file store",
		Buckets:   fileSizeBucket,
	})

	fsSizeSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Name:       "file_size",
		Help:       "Summary for the file size in the file store",
		Objectives: metricsSummaryQuantile,
	})

	fsTotalCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "file_current_total",
		Help:      "Total number of current files in the file store",
	})

	fsTotalSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "file_size_current_total",
		Help:      "Total size of current files in the file store",
	})
)

func init() {
	prometheus.MustRegister(execErrorCount)
	prometheus.MustRegister(execTimeHist, execTimeSummary)
	prometheus.MustRegister(execMemHist, execMemSummary)
	prometheus.MustRegister(fsSizeHist, fsSizeSummary, fsTotalSize)
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

var _ filestore.FileStore = &metricsFileStore{}

type metricsFileStore struct {
	mu sync.Mutex
	filestore.FileStore
	fileSize map[string]int
}

func newMetricsFileStore(fs filestore.FileStore) filestore.FileStore {
	return &metricsFileStore{
		FileStore: fs,
		fileSize:  make(map[string]int),
	}
}

func (m *metricsFileStore) Add(name string, content []byte) (string, error) {
	id, err := m.FileStore.Add(name, content)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	s := len(content)
	m.fileSize[id] = s

	sf := float64(s)
	fsSizeHist.Observe(sf)
	fsSizeSummary.Observe(sf)
	fsTotalSize.Add(sf)
	fsTotalCount.Inc()

	return id, nil
}

func (m *metricsFileStore) Remove(id string) bool {
	success := m.FileStore.Remove(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.fileSize[id]
	if !ok {
		return success
	}

	sf := float64(s)
	fsTotalSize.Sub(sf)
	fsTotalCount.Dec()

	return success
}
