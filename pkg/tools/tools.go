package tools

import (
	"context"

	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type UpdateOptions struct {
	Pluralizer    pluralizer.PluralizerInterface
	DynamicClient dynamic.Interface
}

func Update(ctx context.Context, el *unstructured.Unstructured, opts UpdateOptions) (*unstructured.Unstructured, error) {
	gvr, err := opts.Pluralizer.GVKtoGVR(el.GroupVersionKind())
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
	gvr, err := opts.Pluralizer.GVKtoGVR(el.GroupVersionKind())
	if err != nil {
		return nil, err
	}

	res, err := opts.DynamicClient.Resource(gvr).
		Namespace(el.GetNamespace()).
		UpdateStatus(ctx, el, metav1.UpdateOptions{})

	return res, err
}
