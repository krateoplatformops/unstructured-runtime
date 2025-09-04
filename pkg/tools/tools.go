package tools

import (
	"context"
	"fmt"

	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type UpdateOptions struct {
	Pluralizer    pluralizer.PluralizerInterface
	DynamicClient dynamic.Interface
}

const (
	ErrNilPluralizer = "pluralizer cannot be nil"
	ErrNilDynamic    = "dynamic client cannot be nil"
)

// Update updates the given unstructured object using the provided dynamic client and pluralizer. It returns the updated object or an error if the update fails.
// In case of error the original object is returned along with the error.
func Update(ctx context.Context, el *unstructured.Unstructured, opts UpdateOptions) (*unstructured.Unstructured, error) {
	if opts.Pluralizer == nil {
		return el, fmt.Errorf(ErrNilPluralizer)
	}
	if opts.DynamicClient == nil {
		return el, fmt.Errorf(ErrNilDynamic)
	}
	gvr, err := opts.Pluralizer.GVKtoGVR(el.GroupVersionKind())
	if err != nil {
		return el, err
	}

	res, err := opts.DynamicClient.Resource(gvr).
		Namespace(el.GetNamespace()).
		Update(ctx, el, metav1.UpdateOptions{
			FieldValidation: "Ignore",
		})
	if err != nil {
		return el, err
	}

	return res, nil
}

// UpdateStatus updates the status subresource of the given unstructured object using the provided dynamic client and pluralizer. It returns the updated object or an error if the update fails.
// In case of error the original object is returned along with the error.
func UpdateStatus(ctx context.Context, el *unstructured.Unstructured, opts UpdateOptions) (*unstructured.Unstructured, error) {
	if opts.Pluralizer == nil {
		return el, fmt.Errorf(ErrNilPluralizer)
	}
	if opts.DynamicClient == nil {
		return el, fmt.Errorf(ErrNilDynamic)
	}
	gvr, err := opts.Pluralizer.GVKtoGVR(el.GroupVersionKind())
	if err != nil {
		return el, err
	}

	res, err := opts.DynamicClient.Resource(gvr).
		Namespace(el.GetNamespace()).
		UpdateStatus(ctx, el, metav1.UpdateOptions{})

	if err != nil {
		return el, err
	}

	return res, nil
}
