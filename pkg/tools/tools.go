package tools

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"
)

type UpdateOptions struct {
	DiscoveryClient discovery.DiscoveryInterface
	DynamicClient   dynamic.Interface
}

func Update(ctx context.Context, el *unstructured.Unstructured, opts UpdateOptions) (*unstructured.Unstructured, error) {
	gvr, err := GVKtoGVR(opts.DiscoveryClient, el.GroupVersionKind())
	if err != nil {
		return nil, err
	}

	res, err := opts.DynamicClient.Resource(gvr).
		Namespace(el.GetNamespace()).
		Update(ctx, el, metav1.UpdateOptions{
			FieldValidation: "Ignore",
		})

	return res, err
}

func UpdateStatus(ctx context.Context, el *unstructured.Unstructured, opts UpdateOptions) (*unstructured.Unstructured, error) {
	gvr, err := GVKtoGVR(opts.DiscoveryClient, el.GroupVersionKind())
	if err != nil {
		return nil, err
	}

	res, err := opts.DynamicClient.Resource(gvr).
		Namespace(el.GetNamespace()).
		UpdateStatus(ctx, el, metav1.UpdateOptions{})

	return res, err
}

func GVKtoGVR(dc discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	groupResources, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	return mapping.Resource, nil
}

func InferGVKtoGVR(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	kind := types.Type{Name: types.Name{Name: gvk.Kind}}
	namer := namer.NewPrivatePluralNamer(nil)
	resource := strings.ToLower(namer.Name(&kind))

	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: resource,
	}
}
