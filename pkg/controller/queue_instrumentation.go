package controller

import (
	"context"
	"sync"
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/priorityqueue"
)

// QueueMetricsRecorder records queue-related OTEL telemetry.
type QueueMetricsRecorder interface {
	AddQueueDepth(ctx context.Context, delta int64)
	RecordQueueOldestItemAge(ctx context.Context, d time.Duration)
}

// InstrumentedQueue wraps a priority queue to record OTEL metrics.
type InstrumentedQueue[T comparable] struct {
	queue      priorityqueue.PriorityQueue[T]
	recorder   QueueMetricsRecorder
	lock       sync.Mutex
	enqueuedAt map[T]time.Time
	done       chan struct{}
}

// NewInstrumentedQueue creates a queue that records OTEL metrics.
func NewInstrumentedQueue[T comparable](
	queue priorityqueue.PriorityQueue[T],
	recorder QueueMetricsRecorder,
) *InstrumentedQueue[T] {
	iq := &InstrumentedQueue[T]{
		queue:      queue,
		recorder:   recorder,
		enqueuedAt: make(map[T]time.Time),
		done:       make(chan struct{}),
	}

	// Start background routine to periodically record oldest item age
	if recorder != nil {
		go iq.recordOldestItemAgeLoop()
	}

	return iq
}

// recordOldestItemAgeLoop periodically records the age of the oldest item in the queue.
func (iq *InstrumentedQueue[T]) recordOldestItemAgeLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-iq.done:
			return
		case <-ticker.C:
			iq.recordOldestItemAge()
		}
	}
}

// recordOldestItemAge records the age of the oldest queued item.
func (iq *InstrumentedQueue[T]) recordOldestItemAge() {
	iq.lock.Lock()
	defer iq.lock.Unlock()

	var oldest time.Duration
	now := time.Now()
	for _, enqueuedTime := range iq.enqueuedAt {
		age := now.Sub(enqueuedTime)
		if age > oldest {
			oldest = age
		}
	}

	if oldest > 0 && iq.recorder != nil {
		iq.recorder.RecordQueueOldestItemAge(context.Background(), oldest)
	}
}

// Add adds an item to the queue and records queue depth.
func (iq *InstrumentedQueue[T]) Add(item T) {
	iq.recordEnqueueAt(item)
	iq.queue.Add(item)
}

// AddWithOpts adds items with options and records queue depth.
func (iq *InstrumentedQueue[T]) AddWithOpts(o priorityqueue.AddOpts, items ...T) {
	for _, item := range items {
		iq.recordEnqueueAt(item)
	}
	iq.queue.AddWithOpts(o, items...)
}

// AddAfter adds an item after a duration.
func (iq *InstrumentedQueue[T]) AddAfter(item T, duration time.Duration) {
	iq.recordEnqueueAt(item)
	iq.queue.AddAfter(item, duration)
}

// AddRateLimited adds an item with rate limiting.
func (iq *InstrumentedQueue[T]) AddRateLimited(item T) {
	iq.recordEnqueueAt(item)
	iq.queue.AddRateLimited(item)
}

// Get retrieves an item from the queue.
func (iq *InstrumentedQueue[T]) Get() (item T, shutdown bool) {
	item, shutdown = iq.queue.Get()
	if !shutdown {
		iq.recordDequeueAt(item)
	}
	return item, shutdown
}

// GetWithPriority retrieves an item with its priority.
func (iq *InstrumentedQueue[T]) GetWithPriority() (item T, priority int, shutdown bool) {
	item, priority, shutdown = iq.queue.GetWithPriority()
	if !shutdown {
		iq.recordDequeueAt(item)
	}
	return item, priority, shutdown
}

// Done marks an item as done.
func (iq *InstrumentedQueue[T]) Done(item T) {
	iq.queue.Done(item)
}

// ShutDown shuts down the queue.
func (iq *InstrumentedQueue[T]) ShutDown() {
	close(iq.done)
	iq.queue.ShutDown()
}

// ShutDownWithDrain shuts down the queue after draining it.
func (iq *InstrumentedQueue[T]) ShutDownWithDrain() {
	close(iq.done)
	iq.queue.ShutDownWithDrain()
}

// ShuttingDown returns true if the queue is shutting down.
func (iq *InstrumentedQueue[T]) ShuttingDown() bool {
	return iq.queue.ShuttingDown()
}

// Len returns the number of items in the queue.
func (iq *InstrumentedQueue[T]) Len() int {
	return iq.queue.Len()
}

// NumRequeues returns the number of requeues for an item.
func (iq *InstrumentedQueue[T]) NumRequeues(item T) int {
	return iq.queue.NumRequeues(item)
}

// Forget removes an item from retry tracking.
func (iq *InstrumentedQueue[T]) Forget(item T) {
	iq.queue.Forget(item)
}

// recordEnqueueAt records when an item is enqueued.
func (iq *InstrumentedQueue[T]) recordEnqueueAt(item T) {
	iq.lock.Lock()
	defer iq.lock.Unlock()

	if _, exists := iq.enqueuedAt[item]; !exists {
		iq.enqueuedAt[item] = time.Now()
		if iq.recorder != nil {
			iq.recorder.AddQueueDepth(context.Background(), 1)
		}
	}
}

// recordDequeueAt records when an item is dequeued.
func (iq *InstrumentedQueue[T]) recordDequeueAt(item T) {
	iq.lock.Lock()
	defer iq.lock.Unlock()

	if _, exists := iq.enqueuedAt[item]; exists {
		delete(iq.enqueuedAt, item)
		if iq.recorder != nil {
			iq.recorder.AddQueueDepth(context.Background(), -1)
		}
	}
}
