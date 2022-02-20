package main

import (
	"os"
	"sync"

	"github.com/criyle/go-judge/env/pool"
	"github.com/criyle/go-judge/envexec"
	"github.com/criyle/go-judge/filestore"
	"github.com/criyle/go-judge/worker"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace     = "executorserver"
	execSubsystem        = "exec"
	filestoreSubsystem   = "file"
	environmentSubsystem = "environment"
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

	execErrorCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: execSubsystem,
		Name:      "error_count",
		Help:      "Number of exec query returns error",
	})

	execTimeHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: execSubsystem,
		Name:      "time_seconds",
		Help:      "Histogram for the command execution time",
		Buckets:   timeBuckets,
	}, []string{"status"})

	execMemHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: execSubsystem,
		Name:      "memory_bytes",
		Help:      "Histgram for the command execution max memory",
		Buckets:   memoryBucket,
	}, []string{"status"})

	fsSizeHist = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: filestoreSubsystem,
		Name:      "size_bytes",
		Help:      "Histgram for the file size created in the file store",
		Buckets:   fileSizeBucket,
	})

	fsCurrentTotalCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: filestoreSubsystem,
		Name:      "current_bytes_count",
		Help:      "Total number of current files in the file store",
	})

	fsCurrentTotalSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: filestoreSubsystem,
		Name:      "current_bytes_sum",
		Help:      "Total size of current files in the file store",
	})

	envCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: environmentSubsystem,
		Name:      "count",
		Help:      "Total number of environment build by environment builder",
	})

	envInUse = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: environmentSubsystem,
		Name:      "current_count",
		Help:      "Total number of environment currently in use",
	})
)

func init() {
	prometheus.MustRegister(execErrorCount)
	prometheus.MustRegister(execTimeHist)
	prometheus.MustRegister(execMemHist)
	prometheus.MustRegister(fsSizeHist, fsCurrentTotalCount, fsCurrentTotalSize)
	prometheus.MustRegister(envCreated, envInUse)
}

func execObserve(res worker.Response) {
	if res.Error != nil {
		execErrorCount.Inc()
	}
	for _, r := range res.Results {
		status := r.Status.String()
		time := r.Time.Seconds()
		memory := float64(r.Memory)

		execTimeHist.WithLabelValues(status).Observe(time)
		execMemHist.WithLabelValues(status).Observe(memory)
	}
}

var _ filestore.FileStore = &metricsFileStore{}

type metricsFileStore struct {
	mu sync.Mutex
	filestore.FileStore
	fileSize map[string]int64
}

func newMetricsFileStore(fs filestore.FileStore) filestore.FileStore {
	return &metricsFileStore{
		FileStore: fs,
		fileSize:  make(map[string]int64),
	}
}

func (m *metricsFileStore) Add(name, path string) (string, error) {
	id, err := m.FileStore.Add(name, path)
	if err != nil {
		return "", err
	}

	fi, err := os.Stat(path)
	if err != nil {
		return id, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	s := fi.Size()
	m.fileSize[id] = s

	sf := float64(s)
	fsSizeHist.Observe(sf)
	fsCurrentTotalSize.Add(sf)
	fsCurrentTotalCount.Inc()

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
	delete(m.fileSize, id)

	sf := float64(s)
	fsCurrentTotalSize.Sub(sf)
	fsCurrentTotalCount.Dec()

	return success
}

var _ pool.EnvBuilder = &metriceEnvBuilder{}

type metriceEnvBuilder struct {
	pool.EnvBuilder
}

func (b *metriceEnvBuilder) Build() (pool.Environment, error) {
	e, err := b.EnvBuilder.Build()
	if err != nil {
		return nil, err
	}
	envCreated.Inc()
	return e, nil
}

var _ worker.EnvironmentPool = &metricsEnvPool{}

type metricsEnvPool struct {
	worker.EnvironmentPool
}

func (p *metricsEnvPool) Get() (envexec.Environment, error) {
	e, err := p.EnvironmentPool.Get()
	if err != nil {
		return nil, err
	}
	envInUse.Inc()
	return e, nil
}

func (p *metricsEnvPool) Put(env envexec.Environment) {
	p.EnvironmentPool.Put(env)
	envInUse.Dec()
}
