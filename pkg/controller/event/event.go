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
