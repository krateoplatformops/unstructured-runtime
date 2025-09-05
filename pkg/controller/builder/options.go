package builder

import (
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	metricsserver "github.com/krateoplatformops/unstructured-runtime/pkg/metrics/server"
	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"golang.org/x/time/rate"
	"k8s.io/client-go/util/workqueue"
)

type options struct {
	pluralizer        pluralizer.PluralizerInterface
	namespace         string
	resyncInterval    time.Duration
	logger            logging.Logger
	listWatcher       controller.ListWatcherConfiguration
	globalRateLimiter workqueue.TypedRateLimiter[any]
	metrics           metricsserver.Options
	// MaxRetries is the maximum number of retries for a failed reconciliation.
	// If the value is less than or equal to 0, the default value of 5 will be used.
	// If the value is greater than 0, it will be used as the maximum number of retries.
	// After the maximum number of retries is reached, the event will be dropped from the queue.
	// Default: 5
	maxRetries int

	// WachAnnotations is a map of annotations to watch for changes.
	// The key is the annotation name, and the value is the event to trigger.
	// If an annotation is not present, no event will be triggered.
	// If an annotation is present, the corresponding event will be triggered.
	// This is useful for triggering events based on annotations in the resource.
	watchAnnotations ctrlevent.AnnotationEvents
}

func defaultOptions() options {
	return options{
		resyncInterval: 3 * time.Minute,
		logger:         logging.NewNopLogger(),
		pluralizer:     pluralizer.New(),
		listWatcher:    controller.ListWatcherConfiguration{},
		metrics: metricsserver.Options{
			BindAddress: "0",
		},
		maxRetries:       5,
		watchAnnotations: nil,
		namespace:        "",
		globalRateLimiter: workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[any](3*time.Second, 180*time.Second),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.TypedBucketRateLimiter[any]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	}
}

func WithPluralizer(p pluralizer.PluralizerInterface) func(o *options) {
	return func(o *options) {
		o.pluralizer = p
	}
}

func WithNamespace(ns string) func(o *options) {
	return func(o *options) {
		o.namespace = ns
	}
}

func WithResyncInterval(d time.Duration) func(o *options) {
	return func(o *options) {
		o.resyncInterval = d
	}
}

func WithLogger(l logging.Logger) func(o *options) {
	return func(o *options) {
		o.logger = l
	}
}

func WithListWatcher(lw controller.ListWatcherConfiguration) func(o *options) {
	return func(o *options) {
		o.listWatcher = lw
	}
}

func WithGlobalRateLimiter(rl workqueue.TypedRateLimiter[any]) func(o *options) {
	return func(o *options) {
		o.globalRateLimiter = rl
	}
}

func WithMetrics(m metricsserver.Options) func(o *options) {
	return func(o *options) {
		o.metrics = m
	}
}

func WithMaxRetries(r int) func(o *options) {
	return func(o *options) {
		o.maxRetries = r
	}
}

func WithWatchAnnotations(a ctrlevent.AnnotationEvents) func(o *options) {
	return func(o *options) {
		o.watchAnnotations = a
	}
}
