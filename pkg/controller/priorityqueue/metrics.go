package priorityqueue

import (
	"sync"
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/workqueue/metrics"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
)

type queueMetrics[T comparable] interface {
	add(item T, priority int)
	get(item T, priority int)
	updateDepthWithPriorityMetric(oldPriority, newPriority int)
	done(item T, priority int)
	updateUnfinishedWork()
	retry()
}

func newQueueMetrics[T comparable](mp workqueue.MetricsProvider, name string, clock clock.Clock) queueMetrics[T] {
	if len(name) == 0 {
		return noMetrics[T]{}
	}

	dqm := &defaultQueueMetrics[T]{
		clock:                   clock,
		adds:                    mp.NewAddsMetric(name),
		latency:                 mp.NewLatencyMetric(name),
		workDuration:            mp.NewWorkDurationMetric(name),
		unfinishedWorkSeconds:   mp.NewUnfinishedWorkSecondsMetric(name),
		longestRunningProcessor: mp.NewLongestRunningProcessorSecondsMetric(name),
		addTimes:                map[T]time.Time{},
		processingStartTimes:    map[T]time.Time{},
		retries:                 mp.NewRetriesMetric(name),
	}

	if mpp, ok := mp.(metrics.MetricsProviderWithPriority); ok {
		dqm.depthWithPriority = mpp.NewDepthMetricWithPriority(name)
		dqm.latencyWithPriority = mpp.NewLatencyMetricWithPriority(name)
		dqm.workDurationWithPriority = mpp.NewWorkDurationMetricWithPriority(name)
		dqm.totalDurationWithPriority = mpp.NewTotalDurationMetricWithPriority(name)
	} else {
		dqm.depth = mp.NewDepthMetric(name)
	}
	return dqm
}

// defaultQueueMetrics expects the caller to lock before setting any metrics.
type defaultQueueMetrics[T comparable] struct {
	clock clock.Clock

	// current depth of a workqueue
	depth             workqueue.GaugeMetric
	depthWithPriority metrics.DepthMetricWithPriority
	// total number of adds handled by a workqueue
	adds workqueue.CounterMetric
	// how long an item stays in a workqueue
	latency             workqueue.HistogramMetric
	latencyWithPriority metrics.LatencyMetricWithPriority
	// how long processing an item from a workqueue takes
	workDuration              workqueue.HistogramMetric
	workDurationWithPriority  metrics.WorkDurationMetricWithPriority
	totalDurationWithPriority metrics.TotalDurationMetricWithPriority

	mapLock              sync.RWMutex
	addTimes             map[T]time.Time
	processingStartTimes map[T]time.Time

	// how long have current threads been working?
	unfinishedWorkSeconds   workqueue.SettableGaugeMetric
	longestRunningProcessor workqueue.SettableGaugeMetric

	retries workqueue.CounterMetric
}

// add is called for ready items only
func (m *defaultQueueMetrics[T]) add(item T, priority int) {
	if m == nil {
		return
	}

	m.adds.Inc()
	if m.depthWithPriority != nil {
		m.depthWithPriority.Inc(priority)
	} else {
		m.depth.Inc()
	}

	m.mapLock.Lock()
	defer m.mapLock.Unlock()

	if _, exists := m.addTimes[item]; !exists {
		m.addTimes[item] = m.clock.Now()
	}
}

func (m *defaultQueueMetrics[T]) get(item T, priority int) {
	if m == nil {
		return
	}

	if m.depthWithPriority != nil {
		m.depthWithPriority.Dec(priority)
	} else {
		m.depth.Dec()
	}

	m.mapLock.Lock()
	defer m.mapLock.Unlock()

	m.processingStartTimes[item] = m.clock.Now()
	if startTime, exists := m.addTimes[item]; exists {
		if m.latencyWithPriority != nil {
			m.latencyWithPriority.Observe(priority, m.sinceInSeconds(startTime))
		} else {
			m.latency.Observe(m.sinceInSeconds(startTime))
		}
		delete(m.addTimes, item)
	}
}

func (m *defaultQueueMetrics[T]) updateDepthWithPriorityMetric(oldPriority, newPriority int) {
	if m.depthWithPriority != nil {
		m.depthWithPriority.Dec(oldPriority)
		m.depthWithPriority.Inc(newPriority)
	}
}

func (m *defaultQueueMetrics[T]) done(item T, priority int) {
	if m == nil {
		return
	}

	m.mapLock.Lock()
	defer m.mapLock.Unlock()

	if startTime, exists := m.processingStartTimes[item]; exists {
		processingDuration := m.sinceInSeconds(startTime)
		m.workDuration.Observe(processingDuration)

		if m.workDurationWithPriority != nil {
			m.workDurationWithPriority.Observe(priority, processingDuration)
		}

		delete(m.processingStartTimes, item)
	}

	if addTime, exists := m.addTimes[item]; exists {
		totalDuration := m.sinceInSeconds(addTime)
		if m.totalDurationWithPriority != nil {
			m.totalDurationWithPriority.Observe(priority, totalDuration)
		}
	}
}

func (m *defaultQueueMetrics[T]) updateUnfinishedWork() {
	m.mapLock.RLock()
	defer m.mapLock.RUnlock()
	// Note that a summary metric would be better for this, but prometheus
	// doesn't seem to have non-hacky ways to reset the summary metrics.
	var total float64
	var oldest float64
	for _, t := range m.processingStartTimes {
		age := m.sinceInSeconds(t)
		total += age
		if age > oldest {
			oldest = age
		}
	}
	m.unfinishedWorkSeconds.Set(total)
	m.longestRunningProcessor.Set(oldest)
}

// Gets the time since the specified start in seconds.
func (m *defaultQueueMetrics[T]) sinceInSeconds(start time.Time) float64 {
	return m.clock.Since(start).Seconds()
}

func (m *defaultQueueMetrics[T]) retry() {
	m.retries.Inc()
}

type noMetrics[T any] struct{}

func (noMetrics[T]) add(item T, priority int)                                   {}
func (noMetrics[T]) get(item T, priority int)                                   {}
func (noMetrics[T]) updateDepthWithPriorityMetric(oldPriority, newPriority int) {}
func (noMetrics[T]) done(item T, priority int)                                  {}
func (noMetrics[T]) updateUnfinishedWork()                                      {}
func (noMetrics[T]) retry()                                                     {}
