package unstructured

import (
	"testing"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetConditions(t *testing.T) {
	un := createFakeObject()

	all := GetConditions(un)
	assert.Equal(t, 0, len(all))

	err := SetCondition(un, condition.Unavailable())
	assert.Nil(t, err)

	all = GetConditions(un)
	assert.Equal(t, 1, len(all))
}

func TestIsAvailable(t *testing.T) {
	un := createFakeObject()

	ok, err := IsAvailable(un)
	assert.Nil(t, err)
	assert.True(t, ok)

	err = SetCondition(un, condition.Unavailable())
	assert.Nil(t, err)

	ok, err = IsAvailable(un)
	assert.NotNil(t, err)
	assert.False(t, ok)
	if assert.IsType(t, &NotAvailableError{}, err) {
		//if ex, ok := err.(*NotAvailableError); ok {
		ex, _ := err.(*NotAvailableError)
		assert.NotNil(t, ex.FailedObjectRef)
	}
}

func TestFailedObjectRef(t *testing.T) {
	un := createFakeObject()

	ref, err := ExtractFailedObjectRef(un)
	assert.Nil(t, err)
	assert.Nil(t, ref)

	want := &objectref.ObjectRef{
		APIVersion: "test.example.org",
		Kind:       "Test",
		Name:       "test",
		Namespace:  "test-system",
	}

	err = SetFailedObjectRef(un, want)
	assert.Nil(t, err)

	ref, err = ExtractFailedObjectRef(un)
	assert.Nil(t, err)
	assert.NotNil(t, ref)
	assert.Equal(t, want, ref)

	UnsetFailedObjectRef(un)
	ref, err = ExtractFailedObjectRef(un)
	assert.Nil(t, err)
	assert.Nil(t, ref)
}

func createFakeObject() *unstructured.Unstructured {
	un := &unstructured.Unstructured{}
	un.SetGroupVersionKind(schema.FromAPIVersionAndKind("tests.example.org", "Test"))
	un.SetName("test")
	un.SetNamespace("test-system")
	return un
}
