package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	eventrec "github.com/krateoplatformops/unstructured-runtime/pkg/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/listwatcher"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	unstructuredtools "github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
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
	reasonReconciliationPaused eventrec.Reason = "ReconciliationPaused"
	reasonReconciliationFailed eventrec.Reason = "ReconciliationFailed"
)

type Options struct {
	Client         dynamic.Interface
	Discovery      discovery.DiscoveryInterface
	GVR            schema.GroupVersionResource
	Namespace      string
	ResyncInterval time.Duration
	Recorder       eventrec.Recorder
	Logger         logging.Logger
	ExternalClient ExternalClient
}

type Controller struct {
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	gvr             schema.GroupVersionResource
	queue           workqueue.TypedRateLimitingInterface[Event]
	items           *sync.Map
	informer        cache.Controller
	recorder        eventrec.Recorder
	logger          logging.Logger
	externalClient  ExternalClient
}

func New(sid *shortid.Shortid, opts Options) *Controller {
	rateLimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[Event](3*time.Second, 180*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.TypedBucketRateLimiter[Event]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)

	queue := workqueue.NewTypedRateLimitingQueue(rateLimiter)
	workqueue.NewTypedRateLimitingQueueWithConfig(rateLimiter, workqueue.TypedRateLimitingQueueConfig[Event]{})
	items := sync.Map{}

	lw, err := listwatcher.Create(listwatcher.CreateOptions{
		Discovery: opts.Discovery,
		Client:    opts.Client,
		GVR:       opts.GVR,
		Namespace: opts.Namespace,
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
				el, ok := obj.(*unstructured.Unstructured)
				if !ok {
					opts.Logger.Debug("AddFunc: object is not an unstructured.")
					return
				}

				id, err := sid.Generate()
				if err != nil {
					opts.Logger.Debug(fmt.Errorf("AddFunc: generating short id: %w", err).Error())
					return
				}

				item := Event{
					Id:        id,
					EventType: Observe,
					ObjectRef: objectref.ObjectRef{
						APIVersion: el.GetAPIVersion(),
						Kind:       el.GetKind(),
						Name:       el.GetName(),
						Namespace:  el.GetNamespace(),
					},
				}
				dig := digestForEvent(item)

				if _, exists := items.Load(dig); exists {
					opts.Logger.Debug("AddFunc: item already exists in the queue.")
					return
				}

				if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
					queue.AddRateLimited(item)
					// fmt.Println("Queueing Add event")
					// fmt.Println("Queue length: ", queue.Len())
					// fmt.Println("Queue elements: ", queue)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				oldUns, ok := old.(*unstructured.Unstructured)
				if !ok {
					opts.Logger.Debug("UpdateFunc: object is not an unstructured.")
					return
				}

				newUns, ok := new.(*unstructured.Unstructured)
				if !ok {
					opts.Logger.Debug("UpdateFunc: object is not an unstructured.")
					return
				}

				id, err := sid.Generate()
				if err != nil {
					opts.Logger.Debug(fmt.Errorf("UpdateFunc: generating short id: %w", err).Error())
					return
				}

				newSpec, _, err := unstructured.NestedMap(newUns.Object, "spec")
				if err != nil {
					opts.Logger.Debug(fmt.Errorf("UpdateFunc: getting new object spec: %w", err).Error())
					return
				}

				oldSpec, _, err := unstructured.NestedMap(oldUns.Object, "spec")
				if err != nil {
					opts.Logger.Debug(fmt.Errorf("UpdateFunc: getting old object spec: %w", err).Error())
				}

				diff := cmp.Diff(newSpec, oldSpec)
				opts.Logger.Debug(fmt.Sprintf("UpdateFunc: comparing current spec with desired spec: %s", diff))
				if len(diff) > 0 {
					item := Event{
						Id:        id,
						EventType: Update,
						ObjectRef: objectref.ObjectRef{
							APIVersion: newUns.GetAPIVersion(),
							Kind:       newUns.GetKind(),
							Name:       newUns.GetName(),
							Namespace:  newUns.GetNamespace(),
						}}
					dig := digestForEvent(item)

					if _, exists := items.Load(dig); exists {
						opts.Logger.Debug("UpdaeteFunc: item already exists in the queue.")
						return
					}
					if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
						queue.AddRateLimited(item)
						// fmt.Println("Queueing update event")
						// fmt.Println("Queue length: ", queue.Len())
						// fmt.Println("Queue elements: ", queue)
					}
				} else {
					item := Event{
						Id:        id,
						EventType: Observe,
						ObjectRef: objectref.ObjectRef{
							APIVersion: newUns.GetAPIVersion(),
							Kind:       newUns.GetKind(),
							Name:       newUns.GetName(),
							Namespace:  newUns.GetNamespace(),
						},
					}
					dig := digestForEvent(item)

					// fmt.Println("Dig: ", dig)

					if _, exists := items.Load(dig); exists {
						opts.Logger.Debug("UpdateFunc: item already exists in the queue - (observe)")
						return
					}

					if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
						queue.AddAfter(item, opts.ResyncInterval)
						// fmt.Println("Queueing after", opts.ResyncInterval, "observe event")
						// fmt.Println("Queueing observe event - (observe)")
						// fmt.Println("Queue length: ", queue.Len())
						// fmt.Println("Queue elements: ", queue)
					}
				}

			},
			DeleteFunc: func(obj interface{}) {
				el, ok := obj.(*unstructured.Unstructured)
				if !ok {
					opts.Logger.Debug("DeleteFunc: object is not an unstructured.")
					return
				}

				if meta.IsPaused(el) {
					opts.Logger.Debug(fmt.Sprintf("Reconciliation is paused via the pause annotation %s: %s; %s: %s", "annotation", meta.AnnotationKeyReconciliationPaused, "value", "true"))
					opts.Recorder.Event(el, eventrec.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
					unstructuredtools.SetCondition(el, condition.ReconcilePaused())
					// if the pause annotation is removed, we will have a chance to reconcile again and resume
					// and if status update fails, we will reconcile again to retry to update the status
					return
				}

				id, err := sid.Generate()
				if err != nil {
					opts.Logger.Debug(fmt.Errorf("DeleteFunc: generating short id: %w", err).Error())
					return
				}

				item := Event{
					Id:        id,
					EventType: Delete,
					ObjectRef: objectref.ObjectRef{
						APIVersion: el.GetAPIVersion(),
						Kind:       el.GetKind(),
						Name:       el.GetName(),
						Namespace:  el.GetNamespace(),
					},
				}
				dig := digestForEvent(item)

				if _, exists := items.Load(dig); exists {
					opts.Logger.Debug("DeleteFunc: item already exists in the queue.")
					return
				}
				if _, loaded := items.LoadOrStore(dig, struct{}{}); !loaded {
					queue.AddRateLimited(item)
					// fmt.Println("Queueing delete event")
					// fmt.Println("Queue length: ", queue.Len())
					// fmt.Println("Queue elements: ", queue)
				}
			},
		},
	})
	return &Controller{
		items:           &items,
		dynamicClient:   opts.Client,
		discoveryClient: opts.Discovery,
		gvr:             opts.GVR,
		recorder:        opts.Recorder,
		logger:          opts.Logger,
		informer:        informer,
		queue:           queue,
		externalClient:  opts.ExternalClient,
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
