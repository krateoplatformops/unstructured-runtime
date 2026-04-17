package telemetry

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	defaultServiceName    = "unstructured-runtime"
	defaultExportInterval = 30 * time.Second
)

// Config controls OpenTelemetry metrics export.
type Config struct {
	Enabled        bool
	ServiceName    string
	ExportInterval time.Duration
	DeploymentName string // Deployment name for stable resource identification
}

// Metrics exposes a small set of low-cardinality runtime metrics for
// unstructured-runtime consumers.
type Metrics struct {
	log            logging.Logger
	deploymentName string // For tracking instance identity in exported metrics

	startupSuccess metric.Int64Counter
	startupFailure metric.Int64Counter

	getDuration metric.Float64Histogram
	getFailure  metric.Int64Counter

	connectDuration metric.Float64Histogram
	connectFailure  metric.Int64Counter

	reconcileDuration      metric.Float64Histogram
	queueDepth             metric.Int64UpDownCounter
	queueWaitDuration      metric.Float64Histogram
	queueOldestItemAge     metric.Float64Histogram
	queueWorkDuration      metric.Float64Histogram
	reconcileInFlight      metric.Int64ObservableGauge
	reconcileInFlightCount atomic.Int64

	reconcileSuccess          metric.Int64Counter
	reconcileFailure          metric.Int64Counter
	reconcileRequeueAfter     metric.Int64Counter
	reconcileRequeueImmediate metric.Int64Counter
	reconcileErrorRequeue     metric.Int64Counter
	queueRequeueTotal         metric.Int64Counter

	observeDuration metric.Float64Histogram
	observeFailure  metric.Int64Counter

	createDuration metric.Float64Histogram
	createFailure  metric.Int64Counter

	updateDuration metric.Float64Histogram
	updateFailure  metric.Int64Counter

	deleteDuration metric.Float64Histogram
	deleteFailure  metric.Int64Counter

	finalizerAddDuration    metric.Float64Histogram
	finalizerAddFailure     metric.Int64Counter
	finalizerRemoveDuration metric.Float64Histogram
	finalizerRemoveFailure  metric.Int64Counter

	statusUpdateDuration metric.Float64Histogram
	statusUpdateFailure  metric.Int64Counter
}

// Setup creates and configures an OTLP metrics pipeline.
//
// When metrics are disabled, Setup returns a nil Metrics handle and a no-op
// shutdown function.
func Setup(ctx context.Context, log logging.Logger, cfg Config) (*Metrics, func(context.Context) error, error) {
	if log == nil {
		log = logging.NewNopLogger()
	}

	if !cfg.Enabled {
		return nil, func(context.Context) error { return nil }, nil
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	exportInterval := cfg.ExportInterval
	if exportInterval <= 0 {
		exportInterval = defaultExportInterval
	}

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Build resource attributes including deployment name for stable multi-instance identification
	attrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
	}
	if cfg.DeploymentName != "" {
		attrs = append(attrs,
			attribute.String("k8s.deployment.name", cfg.DeploymentName),
			attribute.String("service.instance.id", cfg.DeploymentName),
		)
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewSchemaless(attrs...))
	if err != nil {
		return nil, nil, err
	}

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(exportInterval))
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	meter := provider.Meter("github.com/krateoplatformops/unstructured-runtime")
	metrics, err := newMetrics(meter, log, cfg.DeploymentName)
	if err != nil {
		_ = provider.Shutdown(ctx)
		return nil, nil, err
	}

	otel.SetMeterProvider(provider)
	log.Info("OpenTelemetry metrics initialized", "deploymentName", cfg.DeploymentName, "serviceName", serviceName, "exportInterval", exportInterval)

	return metrics, provider.Shutdown, nil
}

func newMetrics(meter metric.Meter, log logging.Logger, deploymentName string) (*Metrics, error) {
	var err error
	m := &Metrics{
		log:            log,
		deploymentName: deploymentName,
	}

	if m.startupSuccess, err = meter.Int64Counter("unstructured_runtime.startup.success"); err != nil {
		return nil, err
	}
	if m.startupFailure, err = meter.Int64Counter("unstructured_runtime.startup.failure"); err != nil {
		return nil, err
	}
	if m.getDuration, err = meter.Float64Histogram("unstructured_runtime.reconcile.get.duration_seconds"); err != nil {
		return nil, err
	}
	if m.getFailure, err = meter.Int64Counter("unstructured_runtime.reconcile.get.failure"); err != nil {
		return nil, err
	}
	if m.reconcileDuration, err = meter.Float64Histogram("unstructured_runtime.reconcile.duration_seconds"); err != nil {
		return nil, err
	}
	if m.queueDepth, err = meter.Int64UpDownCounter("unstructured_runtime.reconcile.queue.depth"); err != nil {
		return nil, err
	}
	if m.queueWaitDuration, err = meter.Float64Histogram("unstructured_runtime.reconcile.queue.wait.duration_seconds"); err != nil {
		return nil, err
	}
	if m.queueOldestItemAge, err = meter.Float64Histogram("unstructured_runtime.reconcile.queue.oldest_item_age_seconds"); err != nil {
		return nil, err
	}
	if m.queueWorkDuration, err = meter.Float64Histogram("unstructured_runtime.reconcile.queue.work.duration_seconds"); err != nil {
		return nil, err
	}
	if m.reconcileInFlight, err = meter.Int64ObservableGauge("unstructured_runtime.reconcile.in_flight"); err != nil {
		return nil, err
	}
	if m.reconcileSuccess, err = meter.Int64Counter("unstructured_runtime.reconcile.success"); err != nil {
		return nil, err
	}
	if m.reconcileFailure, err = meter.Int64Counter("unstructured_runtime.reconcile.failure"); err != nil {
		return nil, err
	}
	if m.reconcileRequeueAfter, err = meter.Int64Counter("unstructured_runtime.reconcile.requeue.after"); err != nil {
		return nil, err
	}
	if m.reconcileRequeueImmediate, err = meter.Int64Counter("unstructured_runtime.reconcile.requeue.immediate"); err != nil {
		return nil, err
	}
	if m.reconcileErrorRequeue, err = meter.Int64Counter("unstructured_runtime.reconcile.requeue.error"); err != nil {
		return nil, err
	}
	if m.queueRequeueTotal, err = meter.Int64Counter("unstructured_runtime.reconcile.queue.requeues"); err != nil {
		return nil, err
	}
	if m.connectDuration, err = meter.Float64Histogram("unstructured_runtime.external.connect.duration_seconds"); err != nil {
		return nil, err
	}
	if m.connectFailure, err = meter.Int64Counter("unstructured_runtime.external.connect.failure"); err != nil {
		return nil, err
	}
	if m.observeDuration, err = meter.Float64Histogram("unstructured_runtime.external.observe.duration_seconds"); err != nil {
		return nil, err
	}
	if m.observeFailure, err = meter.Int64Counter("unstructured_runtime.external.observe.failure"); err != nil {
		return nil, err
	}
	if m.createDuration, err = meter.Float64Histogram("unstructured_runtime.external.create.duration_seconds"); err != nil {
		return nil, err
	}
	if m.createFailure, err = meter.Int64Counter("unstructured_runtime.external.create.failure"); err != nil {
		return nil, err
	}
	if m.updateDuration, err = meter.Float64Histogram("unstructured_runtime.external.update.duration_seconds"); err != nil {
		return nil, err
	}
	if m.updateFailure, err = meter.Int64Counter("unstructured_runtime.external.update.failure"); err != nil {
		return nil, err
	}
	if m.deleteDuration, err = meter.Float64Histogram("unstructured_runtime.external.delete.duration_seconds"); err != nil {
		return nil, err
	}
	if m.deleteFailure, err = meter.Int64Counter("unstructured_runtime.external.delete.failure"); err != nil {
		return nil, err
	}
	if m.finalizerAddDuration, err = meter.Float64Histogram("unstructured_runtime.finalizer.add.duration_seconds"); err != nil {
		return nil, err
	}
	if m.finalizerAddFailure, err = meter.Int64Counter("unstructured_runtime.finalizer.add.failure"); err != nil {
		return nil, err
	}
	if m.finalizerRemoveDuration, err = meter.Float64Histogram("unstructured_runtime.finalizer.remove.duration_seconds"); err != nil {
		return nil, err
	}
	if m.finalizerRemoveFailure, err = meter.Int64Counter("unstructured_runtime.finalizer.remove.failure"); err != nil {
		return nil, err
	}
	if m.statusUpdateDuration, err = meter.Float64Histogram("unstructured_runtime.status.update.duration_seconds"); err != nil {
		return nil, err
	}
	if m.statusUpdateFailure, err = meter.Int64Counter("unstructured_runtime.status.update.failure"); err != nil {
		return nil, err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(m.reconcileInFlight, m.reconcileInFlightCount.Load())
		return nil
	}, m.reconcileInFlight)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// instanceAttrs returns metric attributes for instance identification.
// This enables metrics exported to Prometheus to include service.instance.id as exported_instance.
func (m *Metrics) instanceAttrs() []attribute.KeyValue {
	if m == nil || m.deploymentName == "" {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String("service.instance.id", m.deploymentName),
	}
}

func (m *Metrics) IncStartupSuccess(ctx context.Context) {
	if m == nil {
		return
	}
	m.startupSuccess.Add(ctx, 1)
}

func (m *Metrics) IncStartupFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.startupFailure.Add(ctx, 1)
}

func (m *Metrics) RecordGetDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.getDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncGetFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.getFailure.Add(ctx, 1)
}

func (m *Metrics) RecordReconcileDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.reconcileDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) RecordQueueWaitDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.queueWaitDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) RecordQueueWorkDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.queueWorkDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) recordRequeue(ctx context.Context, reason string, counter metric.Int64Counter) {
	if m == nil {
		return
	}
	counter.Add(ctx, 1)
	m.queueRequeueTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

func (m *Metrics) IncReconcileSuccess(ctx context.Context) {
	if m == nil {
		return
	}
	m.reconcileSuccess.Add(ctx, 1)
}

func (m *Metrics) IncReconcileFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.reconcileFailure.Add(ctx, 1)
}

func (m *Metrics) IncReconcileRequeueAfter(ctx context.Context) {
	m.recordRequeue(ctx, "after", m.reconcileRequeueAfter)
}

func (m *Metrics) IncReconcileRequeueImmediate(ctx context.Context) {
	m.recordRequeue(ctx, "immediate", m.reconcileRequeueImmediate)
}

func (m *Metrics) IncReconcileErrorRequeue(ctx context.Context) {
	m.recordRequeue(ctx, "error", m.reconcileErrorRequeue)
}

func (m *Metrics) AddReconcileInFlight(delta int64) {
	if m == nil || delta == 0 {
		return
	}
	m.reconcileInFlightCount.Add(delta)
}

func (m *Metrics) RecordObserveDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.observeDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncObserveFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.observeFailure.Add(ctx, 1)
}

func (m *Metrics) RecordCreateDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.createDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncCreateFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.createFailure.Add(ctx, 1)
}

func (m *Metrics) RecordUpdateDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.updateDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncUpdateFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.updateFailure.Add(ctx, 1)
}

func (m *Metrics) RecordDeleteDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.deleteDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncDeleteFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.deleteFailure.Add(ctx, 1)
}

func (m *Metrics) RecordFinalizerAddDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.finalizerAddDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncFinalizerAddFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.finalizerAddFailure.Add(ctx, 1)
}

func (m *Metrics) RecordFinalizerRemoveDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.finalizerRemoveDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncFinalizerRemoveFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.finalizerRemoveFailure.Add(ctx, 1)
}

func (m *Metrics) RecordStatusUpdateDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.statusUpdateDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncStatusUpdateFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.statusUpdateFailure.Add(ctx, 1)
}

func (m *Metrics) AddQueueDepth(ctx context.Context, delta int64) {
	if m == nil {
		return
	}
	m.queueDepth.Add(ctx, delta)
}

func (m *Metrics) RecordQueueOldestItemAge(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.queueOldestItemAge.Record(ctx, d.Seconds())
}

func (m *Metrics) RecordConnectDuration(ctx context.Context, d time.Duration) {
	if m == nil {
		return
	}
	m.connectDuration.Record(ctx, d.Seconds())
}

func (m *Metrics) IncConnectFailure(ctx context.Context) {
	if m == nil {
		return
	}
	m.connectFailure.Add(ctx, 1)
}
