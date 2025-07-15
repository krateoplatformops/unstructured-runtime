// Package event records Kubernetes events.
package event

import (
	"k8s.io/apimachinery/pkg/runtime"
	record "k8s.io/client-go/tools/events"
)

// A Type of event.
type Type string

// Types of events.
var (
	TypeNormal  Type = "Normal"
	TypeWarning Type = "Warning"
)

// Reason an event occurred.
type Reason string

// An Event relating to a custom resource.
type Event struct {
	Type    Type           // Type of this event (Normal, Warning), new types could be added in the future.
	Related runtime.Object // 'related' is the secondary object for more complex actions. E.g. when regarding object triggers a creation or deletion of related object.
	Reason  Reason         // Why the action was taken.
	Message string         // A human-readable description of the status of this operation. Maximal length of the note is 1kB, but libraries should be prepared to handle values up to 64kB.
	Action  string         // What action was taken/failed regarding to the regarding object.
}

type EventOption func(*Event)

func WithAction(action string) EventOption {
	return func(e *Event) {
		e.Action = action
	}
}

func WithRelated(related runtime.Object) EventOption {
	return func(e *Event) {
		e.Related = related
	}
}

// Normal returns a normal, informational event.
func Normal(r Reason, message string, opts ...EventOption) Event {
	e := Event{
		Type:    TypeNormal,
		Reason:  r,
		Message: message,
	}

	for _, opt := range opts {
		opt(&e)
	}

	return e
}

// Warning returns a warning event, typically due to an error.
func Warning(r Reason, err error, opts ...EventOption) Event {
	e := Event{
		Type:    TypeWarning,
		Reason:  r,
		Message: err.Error(),
	}

	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// A Recorder records Kubernetes events.
type Recorder interface {
	Event(obj runtime.Object, e Event)
}

// An APIRecorder records Kubernetes events to an API server.
type APIRecorder struct {
	kube record.EventRecorder
}

// NewAPIRecorder returns an APIRecorder that records Kubernetes events to an
// APIServer using the supplied EventRecorder.
func NewAPIRecorder(r record.EventRecorder) *APIRecorder {
	if r == nil {
		return nil
	}

	return &APIRecorder{kube: r}
}

// Event records the supplied event.
func (r *APIRecorder) Event(obj runtime.Object, e Event) {
	r.kube.Eventf(obj, e.Related, string(e.Type), string(e.Reason), e.Action, e.Message)
}

// A NopRecorder does nothing.
type NopRecorder struct{}

// NewNopRecorder returns a Recorder that does nothing.
func NewNopRecorder() *NopRecorder {
	return &NopRecorder{}
}

// Event does nothing.
func (r *NopRecorder) Event(_ runtime.Object, _ Event) {}
