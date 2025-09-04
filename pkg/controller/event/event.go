package event

import (
	"time"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
)

type EventType string

const (
	Observe EventType = "Observe"
	Create  EventType = "Create"
	Update  EventType = "Update"
	Delete  EventType = "Delete"
)

type Event struct {
	EventType EventType
	ObjectRef objectref.ObjectRef
	QueuedAt  time.Time
}

func (e Event) CompareTo(other Event) int {
	if e.QueuedAt.Before(other.QueuedAt) {
		return -1
	}
	if e.QueuedAt.After(other.QueuedAt) {
		return 1
	}
	return 0
}
