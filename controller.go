package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/krateoplatformops/plumbing/kubeutil/event"
	"github.com/krateoplatformops/plumbing/kubeutil/eventrecorder"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	metricsserver "github.com/krateoplatformops/unstructured-runtime/pkg/metrics/server"
	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
)

type Options struct {
	Debug             bool                                `json:"debug"`
	ProviderName      string                              `json:"providerName"`
	Pluralizer        pluralizer.PluralizerInterface      `json:"pluralizer"`
	GVR               schema.GroupVersionResource         `json:"gvr"`
	Client            dynamic.Interface                   `json:"client"`
	Discovery         discovery.CachedDiscoveryInterface  `json:"discovery"`
	Namespace         string                              `json:"namespace"`
	Config            *rest.Config                        `json:"config"`
	ResyncInterval    time.Duration                       `json:"resyncInterval"`
	Logger            logging.Logger                      `json:"logger"`
	ListWatcher       controller.ListWatcherConfiguration `json:"listWatcher"`
	GlobalRateLimiter workqueue.TypedRateLimiter[any]     `json:"globalRateLimiter"`
	Metrics           metricsserver.Options               `json:"metrics"`

	// WachAnnotations is a map of annotations to watch for changes.
	// The key is the annotation name, and the value is the event to trigger.
	// If an annotation is not present, no event will be triggered.
	// If an annotation is present, the corresponding event will be triggered.
	// This is useful for triggering events based on annotations in the resource.
	WatchAnnotations ctrlevent.AnnotationEvents `json:"watchAnnotations"`
}

func GetConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	if cfg.QPS == 0.0 {
		// Disable client-side ratelimer by default, we can rely on
		// API priority and fairness
		cfg.QPS = -1
	}
	return cfg, nil
}

func New(ctx context.Context, opts Options) *controller.Controller {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	if err != nil {
		logging.NewNopLogger().Info("failed to create shortid", "err", err)
	}

	rec, err := eventrecorder.Create(ctx, opts.Config, "unstructured-runtime", nil)
	if err != nil {
		return nil
	}

	metricsServer, err := metricsserver.NewServer(opts.Metrics, opts.Config, http.DefaultClient)
	if err != nil {
		opts.Logger.Error(err, "failed to create metrics server")
		return nil
	}

	watchAnnotations := ctrlevent.NewAnnotationEvents()
	watchAnnotations.Add(ctrlevent.Observe, meta.AnnotationKeyReconciliationPaused, ctrlevent.OnAny, false)
	watchAnnotations.Add(ctrlevent.Create, meta.AnnotationKeyExternalCreatePending, ctrlevent.OnDelete, false)
	if len(opts.WatchAnnotations) > 0 {
		for _, event := range opts.WatchAnnotations {
			watchAnnotations.Add(event.EventType, event.Annotation, ctrlevent.OnAny, true)
		}
	}

	ctrl := controller.New(sid, controller.Options{
		Pluralizer:        opts.Pluralizer,
		Client:            opts.Client,
		Discovery:         opts.Discovery,
		GVR:               opts.GVR,
		Namespace:         opts.Namespace,
		ResyncInterval:    opts.ResyncInterval,
		Recorder:          event.NewAPIRecorder(rec),
		Logger:            opts.Logger,
		ListWatcher:       opts.ListWatcher,
		GlobalRateLimiter: opts.GlobalRateLimiter,
		MetricsServer:     metricsServer,
		WatchAnnotations:  watchAnnotations,
	})

	return ctrl
}
