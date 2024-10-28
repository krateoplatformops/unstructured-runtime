package condition

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUnavailable(t *testing.T) {
	expected := metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonUnavailable,
	}

	result := Unavailable()

	if result.Type != expected.Type {
		t.Errorf("Expected Type to be %s, but got %s", expected.Type, result.Type)
	}

	if result.Status != expected.Status {
		t.Errorf("Expected Status to be %s, but got %s", expected.Status, result.Status)
	}

	if result.Reason != expected.Reason {
		t.Errorf("Expected Reason to be %s, but got %s", expected.Reason, result.Reason)
	}
}
func TestCreating(t *testing.T) {
	expected := metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonCreating,
	}

	result := Creating()

	if result.Type != expected.Type {
		t.Errorf("Expected Type to be %s, but got %s", expected.Type, result.Type)
	}

	if result.Status != expected.Status {
		t.Errorf("Expected Status to be %s, but got %s", expected.Status, result.Status)
	}

	if result.Reason != expected.Reason {
		t.Errorf("Expected Reason to be %s, but got %s", expected.Reason, result.Reason)
	}
}
func TestFailWithReason(t *testing.T) {
	reason := "SomeReason"
	expected := metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
	}

	result := FailWithReason(reason)

	if result.Type != expected.Type {
		t.Errorf("Expected Type to be %s, but got %s", expected.Type, result.Type)
	}

	if result.Status != expected.Status {
		t.Errorf("Expected Status to be %s, but got %s", expected.Status, result.Status)
	}

	if result.Reason != expected.Reason {
		t.Errorf("Expected Reason to be %s, but got %s", expected.Reason, result.Reason)
	}
}
func TestDeleting(t *testing.T) {
	expected := metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonDeleting,
	}

	result := Deleting()

	if result.Type != expected.Type {
		t.Errorf("Expected Type to be %s, but got %s", expected.Type, result.Type)
	}

	if result.Status != expected.Status {
		t.Errorf("Expected Status to be %s, but got %s", expected.Status, result.Status)
	}

	if result.Reason != expected.Reason {
		t.Errorf("Expected Reason to be %s, but got %s", expected.Reason, result.Reason)
	}
}
func TestAvailable(t *testing.T) {
	expected := metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonAvailable,
	}

	result := Available()

	if result.Type != expected.Type {
		t.Errorf("Expected Type to be %s, but got %s", expected.Type, result.Type)
	}

	if result.Status != expected.Status {
		t.Errorf("Expected Status to be %s, but got %s", expected.Status, result.Status)
	}

	if result.Reason != expected.Reason {
		t.Errorf("Expected Reason to be %s, but got %s", expected.Reason, result.Reason)
	}
}
func TestUpsert(t *testing.T) {
	conds := []metav1.Condition{
		{
			Type:   "Condition1",
			Status: metav1.ConditionTrue,
		},
		{
			Type:   "Condition2",
			Status: metav1.ConditionFalse,
		},
	}

	co := metav1.Condition{
		Type:   "Condition1",
		Status: metav1.ConditionFalse,
	}

	Upsert(&conds, co)

	found := false
	for _, c := range conds {
		if c.Type == co.Type {
			if c.Status != co.Status {
				t.Errorf("Expected Status to be %s, but got %s", co.Status, c.Status)
			}
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Condition %s not found", co.Type)
	}
}
func TestJoin(t *testing.T) {
	conds := []metav1.Condition{
		{
			Type:   "Condition1",
			Status: metav1.ConditionTrue,
		},
		{
			Type:   "Condition2",
			Status: metav1.ConditionFalse,
		},
	}

	all := []metav1.Condition{
		{
			Type:   "Condition1",
			Status: metav1.ConditionTrue,
		},
		{
			Type:   "Condition3",
			Status: metav1.ConditionUnknown,
		},
	}

	expected := []metav1.Condition{
		{
			Type:   "Condition1",
			Status: metav1.ConditionTrue,
		},
		{
			Type:   "Condition2",
			Status: metav1.ConditionFalse,
		},
		// {
		// 	Type:   "Condition3",
		// 	Status: metav1.ConditionTrue,
		// },
	}

	Join(&conds, all)

	if len(conds) != len(expected) {
		t.Errorf("Expected %d conditions, but got %d", len(expected), len(conds))
	}

	for i := range conds {
		if conds[i].Type != expected[i].Type {
			t.Errorf("Expected Type to be %s, but got %s", expected[i].Type, conds[i].Type)
		}

		if conds[i].Status != expected[i].Status {
			t.Errorf("Expected Status to be %s, but got %s", expected[i].Status, conds[i].Status)
		}
	}
}
func TestRemove(t *testing.T) {
	conds := []metav1.Condition{
		{
			Type:   "Condition1",
			Status: metav1.ConditionTrue,
		},
		{
			Type:   "Condition2",
			Status: metav1.ConditionFalse,
		},
		{
			Type:   "Condition3",
			Status: metav1.ConditionTrue,
		},
	}

	typ := "Condition2"

	Remove(&conds, typ)

	found := false
	for _, c := range conds {
		if c.Type == typ {
			found = true
			break
		}
	}

	if found {
		t.Errorf("Condition %s should have been removed", typ)
	}

	if len(conds) != 2 {
		t.Errorf("Expected 2 conditions, but got %d", len(conds))
	}
}
