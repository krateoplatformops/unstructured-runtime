package builder

import (
	"context"
	"fmt"
	"net/http"

	"github.com/krateoplatformops/plumbing/kubeutil/event"
	"github.com/krateoplatformops/plumbing/kubeutil/eventrecorder"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	metricsserver "github.com/krateoplatformops/unstructured-runtime/pkg/metrics/server"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

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

type Configuration struct {
	Config       *rest.Config
	GVR          schema.GroupVersionResource
	ProviderName string
}

func (c Configuration) validate() error {
	if c.Config == nil {
		return fmt.Errorf("kube config is required")
	}
	if c.GVR.Group == "" || c.GVR.Version == "" || c.GVR.Resource == "" {
		return fmt.Errorf("gvr is required")
	}
	if c.ProviderName == "" {
		return fmt.Errorf("provider name is required")
	}
	return nil
}

type FuncOption func(o *options)

func Build(ctx context.Context, conf Configuration, optsFuncs ...FuncOption) (*controller.Controller, error) {
	if err := conf.validate(); err != nil {
		return nil, err
	}
	opts := defaultOptions()
	for _, o := range optsFuncs {
		o(&opts)
	}

	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	if err != nil {
		opts.logger.Info("failed to create shortid", "err", err)
		return nil, fmt.Errorf("failed to create shortid: %w", err)
	}

	rec, err := eventrecorder.Create(ctx, conf.Config, "unstructured-runtime", nil)
	if err != nil {
		opts.logger.Error(err, "failed to create event recorder")
		return nil, fmt.Errorf("failed to create event recorder: %w", err)
	}

	metricsServer, err := metricsserver.NewServer(opts.metrics, conf.Config, http.DefaultClient)
	if err != nil {
		opts.logger.Error(err, "failed to create metrics server")
		return nil, fmt.Errorf("failed to create metrics server: %w", err)
	}

	watchAnnotations := ctrlevent.NewAnnotationEvents()
	watchAnnotations.Add(ctrlevent.Observe, meta.AnnotationKeyReconciliationPaused, ctrlevent.OnAny, false)
	watchAnnotations.Add(ctrlevent.Create, meta.AnnotationKeyExternalCreatePending, ctrlevent.OnDelete, false)
	if len(opts.watchAnnotations) > 0 {
		for _, event := range opts.watchAnnotations {
			watchAnnotations.Add(event.EventType, event.Annotation, ctrlevent.OnAny, true)
		}
	}

	client, err := dynamic.NewForConfig(conf.Config)
	if err != nil {
		opts.logger.Error(err, "failed to create dynamic client")
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	ctrl, err := controller.New(sid, controller.Options{
		Pluralizer:        opts.pluralizer,
		Client:            client,
		GVR:               conf.GVR,
		Namespace:         opts.namespace,
		ResyncInterval:    opts.resyncInterval,
		Recorder:          event.NewAPIRecorder(rec),
		Logger:            opts.logger,
		ListWatcher:       opts.listWatcher,
		GlobalRateLimiter: opts.globalRateLimiter,
		MetricsServer:     metricsServer,
		WatchAnnotations:  watchAnnotations,
		MaxRetries:        opts.maxRetries,
	})
	if err != nil {
		opts.logger.Error(err, "failed to create controller")
		return nil, fmt.Errorf("failed to create controller: %w", err)
	}

	return ctrl, nil
}
