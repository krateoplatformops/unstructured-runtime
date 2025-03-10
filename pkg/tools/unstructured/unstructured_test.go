package unstructured

import (
	"testing"
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetConditions(t *testing.T) {
	un := createFakeObject()

	all := GetConditions(un)
	assert.Equal(t, 0, len(all))

	err := SetConditions(un, condition.Unavailable())
	assert.Nil(t, err)

	all = GetConditions(un)
	assert.Equal(t, 1, len(all))
}

func TestIsAvailable(t *testing.T) {
	un := createFakeObject()

	ok, err := IsAvailable(un)
	assert.Nil(t, err)
	assert.True(t, ok)

	err = SetConditions(un, condition.Unavailable())
	assert.Nil(t, err)

	ok, err = IsAvailable(un)
	assert.NotNil(t, err)
	assert.False(t, ok)
	if assert.IsType(t, &NotAvailableError{}, err) {
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

func TestGVR(t *testing.T) {
	un := createFakeObject()
	gvr, err := GVR(un)
	assert.Nil(t, err)
	assert.Equal(t, "tests.example.org", gvr.Group)
	assert.Equal(t, "v1", gvr.Version)
	assert.Equal(t, "tests", gvr.Resource)
}

func TestSetConditions(t *testing.T) {
	un := createFakeObject()
	cond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Tested",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}
	err := SetConditions(un, cond)
	assert.Nil(t, err)

	conds := GetConditions(un)
	assert.Equal(t, 1, len(conds))
	assert.Equal(t, cond.Type, conds[0].Type)
	assert.Equal(t, cond.Status, conds[0].Status)
	assert.Equal(t, cond.Reason, conds[0].Reason)
}

func TestGetCondition(t *testing.T) {
	un := createFakeObject()
	cond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Tested",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}
	err := SetConditions(un, cond)
	assert.Nil(t, err)

	retrievedCond := GetCondition(un, "Ready", "Tested")
	assert.NotNil(t, retrievedCond)
	assert.Equal(t, cond.Type, retrievedCond.Type)
	assert.Equal(t, cond.Status, retrievedCond.Status)
	assert.Equal(t, cond.Reason, retrievedCond.Reason)
}

func TestIsConditionSet(t *testing.T) {
	un := createFakeObject()
	cond := metav1.Condition{
		Type:   "Ready",
		Status: metav1.ConditionTrue,
		Reason: "Tested",
		LastTransitionTime: metav1.Time{
			Time: time.Now(),
		},
	}
	err := SetConditions(un, cond)
	assert.Nil(t, err)

	isSet := IsConditionSet(un, cond)
	assert.True(t, isSet)
}

func TestGetFieldsFromUnstructured(t *testing.T) {
	un := createFakeObject()
	fields, err := GetFieldsFromUnstructured(un, "metadata")
	assert.Nil(t, err)
	assert.NotNil(t, fields)
	assert.Equal(t, "test", fields["name"])
	assert.Equal(t, "test-system", fields["namespace"])
}

func createFakeObject() *unstructured.Unstructured {
	un := &unstructured.Unstructured{}
	un.SetGroupVersionKind(schema.FromAPIVersionAndKind("tests.example.org/v1", "Test"))
	un.SetName("test")
	un.SetNamespace("test-system")
	return un
}
