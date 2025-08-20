package event

type AnnotationEvent struct {
	EventType  EventType `json:"eventType"`
	Annotation string    `json:"annotation"`
}

// This is a list of annotation events.
// It is used to store events that are triggered by annotations in the resource.
// The list is used to determine which events to trigger based on the annotations present in the resource.
type AnnotationEvents []AnnotationEvent

func NewAnnotationEvents(evts ...AnnotationEvent) AnnotationEvents {
	return AnnotationEvents(evts)
}

func (ae AnnotationEvents) HasAnnotation(annotation string) bool {
	for _, event := range ae {
		if event.Annotation == annotation {
			return true
		}
	}
	return false
}

// Remove removes all events with the specified annotation
func (ae *AnnotationEvents) Remove(annotation string) {
	filtered := make(AnnotationEvents, 0, len(*ae))
	for _, event := range *ae {
		if event.Annotation != annotation {
			filtered = append(filtered, event)
		}
	}
	*ae = filtered
}

// Add adds an event with optional overwrite capability
func (ae *AnnotationEvents) Add(eventType EventType, annotation string, overwrite bool) {
	if overwrite {
		// Remove existing event with same annotation
		ae.Remove(annotation)
	} else if ae.HasAnnotation(annotation) {
		// Don't add if already exists and overwrite is false
		return
	}

	*ae = append(*ae, AnnotationEvent{
		EventType:  eventType,
		Annotation: annotation,
	})
}
