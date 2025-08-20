package event

import (
	"testing"
)

func TestNewAnnotationEvents(t *testing.T) {
	tests := []struct {
		name     string
		events   []AnnotationEvent
		expected int
	}{
		{
			name:     "empty events",
			events:   []AnnotationEvent{},
			expected: 0,
		},
		{
			name: "single event",
			events: []AnnotationEvent{
				{EventType: Observe, Annotation: "annotation1"},
			},
			expected: 1,
		},
		{
			name: "multiple events",
			events: []AnnotationEvent{
				{EventType: Observe, Annotation: "annotation1"},
				{EventType: Create, Annotation: "annotation2"},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAnnotationEvents(tt.events...)
			if len(result) != tt.expected {
				t.Errorf("expected %d events, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestAnnotationEvents_HasAnnotation(t *testing.T) {
	events := NewAnnotationEvents(
		AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
		AnnotationEvent{EventType: Create, Annotation: "annotation2"},
	)

	tests := []struct {
		name       string
		annotation string
		expected   bool
	}{
		{
			name:       "existing annotation",
			annotation: "annotation1",
			expected:   true,
		},
		{
			name:       "non-existing annotation",
			annotation: "annotation3",
			expected:   false,
		},
		{
			name:       "empty annotation",
			annotation: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := events.HasAnnotation(tt.annotation)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAnnotationEvents_Remove(t *testing.T) {
	tests := []struct {
		name                string
		initialEvents       AnnotationEvents
		removeAnnotation    string
		expectedLen         int
		expectedAnnotations []string
	}{
		{
			name: "remove existing annotation",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
				AnnotationEvent{EventType: Create, Annotation: "annotation2"},
			),
			removeAnnotation:    "annotation1",
			expectedLen:         1,
			expectedAnnotations: []string{"annotation2"},
		},
		{
			name: "remove non-existing annotation",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
			),
			removeAnnotation:    "annotation3",
			expectedLen:         1,
			expectedAnnotations: []string{"annotation1"},
		},
		{
			name:                "remove from empty list",
			initialEvents:       NewAnnotationEvents(),
			removeAnnotation:    "annotation1",
			expectedLen:         0,
			expectedAnnotations: []string{},
		},
		{
			name: "remove multiple occurrences",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
				AnnotationEvent{EventType: Create, Annotation: "annotation1"},
				AnnotationEvent{EventType: Observe, Annotation: "annotation2"},
			),
			removeAnnotation:    "annotation1",
			expectedLen:         1,
			expectedAnnotations: []string{"annotation2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := tt.initialEvents
			events.Remove(tt.removeAnnotation)

			if len(events) != tt.expectedLen {
				t.Errorf("expected length %d, got %d", tt.expectedLen, len(events))
			}

			for _, expectedAnnotation := range tt.expectedAnnotations {
				if !events.HasAnnotation(expectedAnnotation) {
					t.Errorf("expected annotation %s to exist", expectedAnnotation)
				}
			}
		})
	}
}

func TestAnnotationEvents_Add(t *testing.T) {
	tests := []struct {
		name          string
		initialEvents AnnotationEvents
		eventType     EventType
		annotation    string
		overwrite     bool
		expectedLen   int
		shouldContain string
	}{
		{
			name:          "add to empty list",
			initialEvents: NewAnnotationEvents(),
			eventType:     Observe,
			annotation:    "annotation1",
			overwrite:     false,
			expectedLen:   1,
			shouldContain: "annotation1",
		},
		{
			name: "add new annotation without overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
			),
			eventType:     Create,
			annotation:    "annotation2",
			overwrite:     false,
			expectedLen:   2,
			shouldContain: "annotation2",
		},
		{
			name: "add existing annotation without overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
			),
			eventType:     Create,
			annotation:    "annotation1",
			overwrite:     false,
			expectedLen:   1,
			shouldContain: "annotation1",
		},
		{
			name: "add existing annotation with overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
			),
			eventType:     Create,
			annotation:    "annotation1",
			overwrite:     true,
			expectedLen:   1,
			shouldContain: "annotation1",
		},
		{
			name: "add new annotation with overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1"},
			),
			eventType:     Create,
			annotation:    "annotation2",
			overwrite:     true,
			expectedLen:   2,
			shouldContain: "annotation2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := tt.initialEvents
			events.Add(tt.eventType, tt.annotation, tt.overwrite)

			if len(events) != tt.expectedLen {
				t.Errorf("expected length %d, got %d", tt.expectedLen, len(events))
			}

			if !events.HasAnnotation(tt.shouldContain) {
				t.Errorf("expected annotation %s to exist", tt.shouldContain)
			}

			// Check that the event type is correct for the added annotation
			for _, event := range events {
				if event.Annotation == tt.annotation && event.EventType != tt.eventType {
					t.Errorf("expected event type %v for annotation %s, got %v", tt.eventType, tt.annotation, event.EventType)
				}
			}
		})
	}
}
