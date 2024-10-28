package listwatcher

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

const (
	CompositionVersionLabel = "krateo.io/composition-version"
)

type Selectors struct {
	LabelSelector *string
	FieldSelector *string
}

type CreateOptions struct {
	Client        dynamic.Interface
	Discovery     discovery.DiscoveryInterface
	GVR           schema.GroupVersionResource
	LabelSelector *string
	FieldSelector *string
	Namespace     string
}

func Create(opts CreateOptions) (*cache.ListWatch, error) {
	return &cache.ListWatch{
		ListFunc: func(lo metav1.ListOptions) (runtime.Object, error) {
			if opts.LabelSelector != nil {
				lo.LabelSelector = *opts.LabelSelector
			}
			if opts.FieldSelector != nil {
				lo.FieldSelector = *opts.FieldSelector
			}
			return opts.Client.Resource(opts.GVR).
				Namespace(opts.Namespace).
				List(context.Background(), lo)
		},
		WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
			if opts.LabelSelector != nil {
				lo.LabelSelector = *opts.LabelSelector
			}
			if opts.FieldSelector != nil {
				lo.FieldSelector = *opts.FieldSelector
			}
			return opts.Client.Resource(opts.GVR).
				Namespace(opts.Namespace).
				Watch(context.Background(), lo)
		},
	}, nil
}
