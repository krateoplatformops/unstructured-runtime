package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

type mockPluralizer struct {
	gvr schema.GroupVersionResource
	err error
}

func (m *mockPluralizer) GVKtoGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	return m.gvr, m.err
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		pluralizer    pluralizer.PluralizerInterface
		client        dynamic.Interface
		expectedError bool
	}{
		{
			name: "successful update",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
				},
			},
			pluralizer: &mockPluralizer{
				gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
			},
			client:        createClientWithExistingConfigMap(),
			expectedError: false,
		},
		{
			name: "pluralizer error",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
			},
			pluralizer: &mockPluralizer{
				err: errors.New("pluralizer error"),
			},
			client:        fake.NewSimpleDynamicClient(scheme.Scheme),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := UpdateOptions{
				Pluralizer:    tt.pluralizer,
				DynamicClient: tt.client,
			}

			_, err := Update(context.Background(), tt.obj, opts)

			if tt.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateStatus(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		pluralizer    pluralizer.PluralizerInterface
		client        dynamic.Interface
		expectedError bool
	}{
		{
			name: "successful status update",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
				},
			},
			pluralizer: &mockPluralizer{
				gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
			},
			client:        createClientWithExistingConfigMap(),
			expectedError: false,
		},
		{
			name: "pluralizer error",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
			},
			pluralizer: &mockPluralizer{
				err: errors.New("pluralizer error"),
			},
			client:        fake.NewSimpleDynamicClient(scheme.Scheme),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := UpdateOptions{
				Pluralizer:    tt.pluralizer,
				DynamicClient: tt.client,
			}

			_, err := UpdateStatus(context.Background(), tt.obj, opts)

			if tt.expectedError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Helper function to create a fake client with an existing ConfigMap
func createClientWithExistingConfigMap() dynamic.Interface {
	existingConfigMap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
	}

	return fake.NewSimpleDynamicClient(scheme.Scheme, existingConfigMap)
}
