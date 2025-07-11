package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	metricsserver "github.com/krateoplatformops/unstructured-runtime/pkg/metrics/server"

	"github.com/google/go-cmp/cmp"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/priorityqueue"
	"github.com/krateoplatformops/unstructured-runtime/pkg/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/listwatcher"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	unstructuredtools "github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	reasonReconciliationPaused event.Reason = "ReconciliationPaused"
	reasonReconciliationFailed event.Reason = "ReconciliationFailed"
)

const LowPriority = -100 //Lowest priority for the priorityqueue
const NormalPriority = 0 //Lowest priority for the priorityqueue
const HighPriority = 10  //Lowest priority for the priorityqueue

// An ExternalClient manages the lifecycle of an external resource.
// None of the calls here should be blocking. All of the calls should be
// idempotent. For example, Create call should not return AlreadyExists error
// if it's called again with the same parameters or Delete call should not
// return error if there is an ongoing deletion or resource does not exist.
type ExternalClient interface {
	Observe(ctx context.Context, mg *unstructured.Unstructured) (ExternalObservation, error)
	Create(ctx context.Context, mg *unstructured.Unstructured) error
	Update(ctx context.Context, mg *unstructured.Unstructured) error
	Delete(ctx context.Context, mg *unstructured.Unstructured) error
}

// An ExternalObservation is the result of an observation of an external resource.
type ExternalObservation struct {
	// ResourceExists must be true if a corresponding external resource exists
	// for the managed resource.
	ResourceExists bool

	// ResourceUpToDate should be true if the corresponding external resource
	// appears to be up-to-date - i.e. updating the external resource to match
	// the desired state of the managed resource would be a no-op.
	ResourceUpToDate bool
}

type ListWatcherConfiguration struct {
	LabelSelector *string
	FieldSelector *string
}

type Options struct {
	Client            dynamic.Interface
	Discovery         discovery.DiscoveryInterface
	GVR               schema.GroupVersionResource
	Namespace         string
	ResyncInterval    time.Duration
	Recorder          event.Recorder
	Logger            logging.Logger
	ExternalClient    ExternalClient
	ListWatcher       ListWatcherConfiguration
	Pluralizer        pluralizer.PluralizerInterface
	GlobalRateLimiter workqueue.TypedRateLimiter[any]
	MetricsServer     metricsserver.Server
}

type Controller struct {
	metricsServer   metricsserver.Server
	pluralizer      pluralizer.PluralizerInterface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	gvr             schema.GroupVersionResource
	queue           priorityqueue.PriorityQueue[any]
	items           *sync.Map
	informer        cache.Controller
	recorder        event.Recorder
	logger          logging.Logger
	externalClient  ExternalClient
}

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "controller_reconcile_total",
			Help: "Total number of reconciliations",
		},
		[]string{"kind", "namespace", "result"},
	)

	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "controller_reconcile_duration_seconds",
			Help: "Time spent reconciling",
		},
		[]string{"kind", "namespace"},
	)
)

func init() {
	prometheus.MustRegister(reconcileTotal)
	prometheus.MustRegister(reconcileDuration)
}

func New(sid *shortid.Shortid, opts Options) *Controller {
	if opts.Client == nil {
		return nil
	}
	if opts.Discovery == nil {
		return nil
	}

	if opts.GlobalRateLimiter == nil {
		opts.GlobalRateLimiter = workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[any](3*time.Second, 180*time.Second),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.TypedBucketRateLimiter[any]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		)
	}

	// queue := workqueue.NewTypedRateLimitingQueue(opts.GlobalRateLimiter)
	queue := priorityqueue.New("controller", func(o *priorityqueue.Opts[any]) {
		o.RateLimiter = opts.GlobalRateLimiter
	})
	items := &sync.Map{}

	lw, err := listwatcher.Create(listwatcher.CreateOption{
		Client:        opts.Client,
		Discovery:     opts.Discovery,
		GVR:           opts.GVR,
		LabelSelector: opts.ListWatcher.LabelSelector,
		FieldSelector: opts.ListWatcher.FieldSelector,
		Namespace:     opts.Namespace,
	})
	if err != nil {
		opts.Logger.Debug("Failed to create listwatcher.")
		return nil
	}

	_, informer := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: lw,
		ObjectType:    &unstructured.Unstructured{},
		ResyncPeriod:  opts.ResyncInterval,
		Indexers:      cache.Indexers{},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				log := opts.Logger
				el, ok := obj.(*unstructured.Unstructured)
				if !ok {
					log.Debug("Object is not an unstructured.")
					return
				}

				item := ctrlevent.Event{
					EventType: ctrlevent.Observe,
					ObjectRef: objectref.ObjectRef{
						APIVersion: el.GetAPIVersion(),
						Kind:       el.GetKind(),
						Name:       el.GetName(),
						Namespace:  el.GetNamespace(),
					},
					QueuedAt: time.Now(),
				}
				dig := ctrlevent.DigestForEvent(item)

				// Checking if the object is already being processed
				priority := HighPriority
				annotations := el.GetAnnotations()
				if annotations == nil {
					priority = HighPriority
				}
				_, ok = annotations[meta.AnnotationKeyExternalCreateFailed]
				_, ok_pending := annotations[meta.AnnotationKeyExternalCreatePending]
				_, ok_succeded := annotations[meta.AnnotationKeyExternalCreateSucceeded]
				if ok || ok_pending || ok_succeded {
					priority = NormalPriority //These are events that are already being processed, so we can lower the priority
				}

				if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
					log.WithValues(
						"kind", item.ObjectRef.Kind,
						"apiVersion", item.ObjectRef.APIVersion,
						"name", item.ObjectRef.Name,
						"namespace", item.ObjectRef.Namespace,
						"queuedAt", item.QueuedAt,
					).Debug("Adding Observe event to queue", "priority", priority)
					queue.AddWithOpts(priorityqueue.AddOpts{
						RateLimited: false,
						Priority:    priority,
					}, item)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				log := opts.Logger
				oldUns, ok := old.(*unstructured.Unstructured)
				if !ok {
					log.Debug("Object is not an unstructured.")
					return
				}

				newUns, ok := new.(*unstructured.Unstructured)
				if !ok {
					log.Debug("Object is not an unstructured.")
					return
				}

				if meta.WasDeleted(newUns) {
					log.Debug(fmt.Sprintf("Object %s/%s is being deleted", newUns.GetNamespace(), newUns.GetName()))

					item := ctrlevent.Event{
						EventType: ctrlevent.Delete,
						ObjectRef: objectref.ObjectRef{
							APIVersion: newUns.GetAPIVersion(),
							Kind:       newUns.GetKind(),
							Name:       newUns.GetName(),
							Namespace:  newUns.GetNamespace(),
						},
						QueuedAt: time.Now(),
					}

					dig := ctrlevent.DigestForEvent(item)

					if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
						log.WithValues(
							"kind", item.ObjectRef.Kind,
							"apiVersion", item.ObjectRef.APIVersion,
							"name", item.ObjectRef.Name,
							"namespace", item.ObjectRef.Namespace,
							"queuedAt", item.QueuedAt,
						).Debug("Adding Delete event to queue", "priority", HighPriority)
						queue.AddWithOpts(priorityqueue.AddOpts{
							RateLimited: false,
							Priority:    HighPriority,
						}, item)
					}
					return
				}

				//Checking Krateo Annotations - "krateo.io/paused"
				if meta.IsPaused(newUns) && !meta.IsPaused(oldUns) {
					log.Debug(fmt.Sprintf("Reconciliation is paused via the pause annotation %s: %s; %s: %s", "annotation", meta.AnnotationKeyReconciliationPaused, "value", "true"))
					opts.Recorder.Event(newUns, event.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
					unstructuredtools.SetConditions(newUns, condition.ReconcilePaused())
					// if the pause annotation is removed, we will have a chance to reconcile again and resume
					// and if status update fails, we will reconcile again to retry to update the status
					return
				}

				newSpec, _, err := unstructured.NestedMap(newUns.Object, "spec")
				if err != nil {
					log.Debug(fmt.Errorf("getting new object spec: %w", err).Error())
					return
				}

				oldSpec, _, err := unstructured.NestedMap(oldUns.Object, "spec")
				if err != nil {
					log.Debug(fmt.Errorf("getting old object spec: %w", err).Error())
				}

				diff := cmp.Diff(newSpec, oldSpec)
				if len(diff) > 0 {
					item := ctrlevent.Event{
						EventType: ctrlevent.Update,
						ObjectRef: objectref.ObjectRef{
							APIVersion: newUns.GetAPIVersion(),
							Kind:       newUns.GetKind(),
							Name:       newUns.GetName(),
							Namespace:  newUns.GetNamespace(),
						},
						QueuedAt: time.Now(),
					}

					dig := ctrlevent.DigestForEvent(item)

					if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
						log.WithValues(
							"kind", item.ObjectRef.Kind,
							"apiVersion", item.ObjectRef.APIVersion,
							"name", item.ObjectRef.Name,
							"namespace", item.ObjectRef.Namespace,
							"queuedAt", item.QueuedAt,
						).Debug("Adding Update event to queue", "priority", HighPriority)
						queue.AddWithOpts(priorityqueue.AddOpts{
							RateLimited: false,
							Priority:    HighPriority,
						}, item)
					}
				} else {
					item := ctrlevent.Event{
						EventType: ctrlevent.Observe,
						ObjectRef: objectref.ObjectRef{
							APIVersion: newUns.GetAPIVersion(),
							Kind:       newUns.GetKind(),
							Name:       newUns.GetName(),
							Namespace:  newUns.GetNamespace(),
						},
						QueuedAt: time.Now(),
					}

					dig := ctrlevent.DigestForEvent(item)

					if _, loaded := items.Load(dig); !loaded {
						items.Store(dig, struct{}{})
						time.AfterFunc(opts.ResyncInterval, func() {
							log.WithValues(
								"kind", item.ObjectRef.Kind,
								"apiVersion", item.ObjectRef.APIVersion,
								"name", item.ObjectRef.Name,
								"namespace", item.ObjectRef.Namespace,
								"queuedAt", item.QueuedAt,
							).Debug("Adding Observe event to queue")
							queue.AddWithOpts(priorityqueue.AddOpts{
								RateLimited: true,
								Priority:    LowPriority,
							}, item)
						})
					}
				}

			},
			DeleteFunc: func(obj interface{}) {
				log := opts.Logger
				el, ok := obj.(*unstructured.Unstructured)
				if !ok {
					log.Debug("Object is not an unstructured")
					return
				}

				log.Debug(fmt.Sprintf("Deleting object %s/%s", el.GetNamespace(), el.GetName()))

				if meta.IsPaused(el) {
					log.Debug(fmt.Sprintf("Reconciliation is paused via the pause annotation %s: %s; %s: %s", "annotation", meta.AnnotationKeyReconciliationPaused, "value", "true"))
					opts.Recorder.Event(el, event.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
					unstructuredtools.SetConditions(el, condition.ReconcilePaused())
					// if the pause annotation is removed, we will have a chance to reconcile again and resume
					// and if status update fails, we will reconcile again to retry to update the status
					return
				}

				item := ctrlevent.Event{
					EventType: ctrlevent.Delete,
					ObjectRef: objectref.ObjectRef{
						APIVersion: el.GetAPIVersion(),
						Kind:       el.GetKind(),
						Name:       el.GetName(),
						Namespace:  el.GetNamespace(),
					},
					QueuedAt: time.Now(),
				}
				dig := ctrlevent.DigestForEvent(item)

				if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
					log.WithValues(
						"kind", item.ObjectRef.Kind,
						"apiVersion", item.ObjectRef.APIVersion,
						"name", item.ObjectRef.Name,
						"namespace", item.ObjectRef.Namespace,
						"queuedAt", item.QueuedAt,
					).Debug("Adding Delete event to queue")
					queue.AddWithOpts(priorityqueue.AddOpts{
						RateLimited: false,
						Priority:    HighPriority,
					}, item)
				}
			},
		},
	})
	return &Controller{
		dynamicClient:   opts.Client,
		discoveryClient: opts.Discovery,
		gvr:             opts.GVR,
		items:           items,
		recorder:        opts.Recorder,
		logger:          opts.Logger,
		informer:        informer,
		queue:           queue,
		externalClient:  opts.ExternalClient,
		pluralizer:      opts.Pluralizer,
		metricsServer:   opts.MetricsServer,
	}
}

func (c *Controller) SetExternalClient(ec ExternalClient) {
	c.externalClient = ec
}

// Run begins watching and syncing.
func (c *Controller) Run(ctx context.Context, numWorkers int) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting controller")
	go c.informer.Run(ctx.Done())

	// Start metrics server in goroutine so it doesn't block
	go func() {
		if err := c.metricsServer.WithLogger(c.logger).Start(ctx); err != nil {
			c.logger.Error(err, "metrics server failed")
		}
	}()

	// Wait for all involved caches to be synced, before
	// processing items from the queue is started
	c.logger.Info("waiting for informer caches to sync")
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		err := fmt.Errorf("failed to wait for informers caches to sync")
		utilruntime.HandleError(err)
		return err
	}

	c.logger.Info(fmt.Sprintf("Starting workers: %d", numWorkers))
	for i := 0; i < numWorkers; i++ {
		go wait.Until(func() {
			c.runWorker(ctx)
		}, 2*time.Second, ctx.Done())
	}
	c.logger.Info("Controller ready.")

	<-ctx.Done()
	c.logger.Info("Stopping controller.")

	return nil
}
