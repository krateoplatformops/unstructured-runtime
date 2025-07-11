package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller"
	"github.com/krateoplatformops/unstructured-runtime/pkg/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/eventrecorder"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	MetricsAddr       string                              `json:"metricsAddr"`
	MetricsPort       int                                 `json:"metricsPort"`
}

func New(opts Options) *controller.Controller {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	if err != nil {
		logging.NewNopLogger().Info("failed to create shortid", "err", err)
	}

	rec, err := eventrecorder.Create(opts.Config)
	if err != nil {
		return nil
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
	})

	// Start metrics server if configured
	if opts.MetricsAddr != "" && opts.MetricsPort > 0 {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())

			addr := opts.MetricsAddr + ":" + strconv.Itoa(opts.MetricsPort)
			opts.Logger.Info("Starting metrics server", "addr", addr)

			if err := http.ListenAndServe(addr, mux); err != nil {
				opts.Logger.Error(err, "Metrics server failed")
			}
		}()
	}

	return ctrl
}
