package pluralizer

import (
	"github.com/krateoplatformops/plumbing/cache"
	"github.com/krateoplatformops/plumbing/kubeutil/plurals"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type names struct {
	Plural   string   `json:"plural"`
	Singular string   `json:"singular"`
	Shorts   []string `json:"shorts"`
}

type PluralizerInterface interface {
	GVKtoGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error)
}

type Pluralizer struct {
	cache *cache.TTLCache[string, plurals.Info]
}

func New() *Pluralizer {
	return &Pluralizer{
		cache: cache.NewTTL[string, plurals.Info](),
	}
}

func (p Pluralizer) GVKtoGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	info, err := plurals.Get(gvk, plurals.GetOptions{
		Cache: p.cache,
	})
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: info.Plural,
	}, nil
}
