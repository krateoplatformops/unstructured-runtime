package condition

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types.
const (
	// TypeReady resources are believed to be ready to handle work.
	TypeReady string = "Ready"

	// TypeSynced resources are believed to be in sync with the
	// Kubernetes resources that manage their lifecycle.
	TypeSynced string = "Synced"
)

// Reasons a resource is or is not ready.
const (
	ReasonAvailable   string = "Available"
	ReasonUnavailable string = "Unavailable"
	ReasonCreating    string = "Creating"
	ReasonDeleting    string = "Deleting"
)

// Reasons a resource is or is not synced.
const (
	ReasonReconcileSuccess string = "ReconcileSuccess"
	ReasonReconcileError   string = "ReconcileError"
	ReasonReconcilePaused  string = "ReconcilePaused"
)

func Unavailable() metav1.Condition {
	return metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonUnavailable,
	}
}

// Creating returns a condition that indicates the resource is currently
// being created.
func Creating() metav1.Condition {
	return metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonCreating,
	}
}

func FailWithReason(reason string) metav1.Condition {
	return metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
	}
}

// ReconcilePaused returns a condition that indicates reconciliation on
// the managed resource is paused via the pause annotation.
func ReconcilePaused() metav1.Condition {
	return metav1.Condition{
		Type:               TypeSynced,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonReconcilePaused,
	}
}

// Deleting returns a condition that indicates the resource is currently
// being deleted.
func Deleting() metav1.Condition {
	return metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonDeleting,
	}
}

// Available returns a condition that indicates the resource is
// currently observed to be available for use.
func Available() metav1.Condition {
	return metav1.Condition{
		Type:               TypeReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonAvailable,
	}
}

func Upsert(conds *[]metav1.Condition, co metav1.Condition) {
	for idx, el := range *conds {
		if el.Type == co.Type {
			(*conds)[idx] = co
			return
		}
	}
	*conds = append(*conds, co)
}

func Join(conds *[]metav1.Condition, all []metav1.Condition) {
	for _, el := range *conds {
		Upsert(conds, el)
	}
}

func Remove(conds *[]metav1.Condition, typ string) {
	for idx, el := range *conds {
		if el.Type == typ {
			*conds = append((*conds)[:idx], (*conds)[idx+1:]...)
			return
		}
	}
}

/*
type Status struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
*/
