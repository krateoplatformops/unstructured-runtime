package event

import "testing"

func TestAnnotationEvents_Add_Fixed(t *testing.T) {
	tests := []struct {
		name          string
		initialEvents AnnotationEvents
		eventType     EventType
		annotation    string
		action        Action
		overwrite     bool
		expectedLen   int
		shouldContain string
	}{
		{
			name:          "add to empty list",
			initialEvents: NewAnnotationEvents(),
			eventType:     Observe,
			annotation:    "annotation1",
			action:        OnCreate,
			overwrite:     false,
			expectedLen:   1,
			shouldContain: "annotation1",
		},
		{
			name: "add new annotation without overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1", OnAction: OnCreate},
			),
			eventType:     Create,
			annotation:    "annotation2",
			action:        OnChange,
			overwrite:     false,
			expectedLen:   2,
			shouldContain: "annotation2",
		},
		{
			name: "add existing annotation without overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1", OnAction: OnCreate},
			),
			eventType:   Observe,
			annotation:  "annotation1",
			action:      OnCreate,
			overwrite:   false,
			expectedLen: 1,
		},
		{
			name: "add existing annotation with overwrite",
			initialEvents: NewAnnotationEvents(
				AnnotationEvent{EventType: Observe, Annotation: "annotation1", OnAction: OnCreate},
			),
			eventType:     Create,
			annotation:    "annotation1",
			action:        OnDelete,
			overwrite:     true,
			expectedLen:   1,
			shouldContain: "annotation1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := tt.initialEvents
			events.Add(tt.eventType, tt.annotation, tt.action, tt.overwrite)

			if len(events) != tt.expectedLen {
				t.Errorf("expected length %d, got %d", tt.expectedLen, len(events))
			}

			if tt.shouldContain != "" && !events.HasAnnotation(tt.shouldContain) {
				t.Errorf("expected annotation %s to exist", tt.shouldContain)
			}

			// Check that the event type and action are correct for the added annotation
			for _, event := range events {
				if event.Annotation == tt.annotation {
					if event.EventType != tt.eventType {
						t.Errorf("expected event type %v for annotation %s, got %v", tt.eventType, tt.annotation, event.EventType)
					}
					if event.OnAction != tt.action {
						t.Errorf("expected action %v for annotation %s, got %v", tt.action, tt.annotation, event.OnAction)
					}
				}
			}
		})
	}
}

func TestAction_Constants(t *testing.T) {
	tests := []struct {
		action   Action
		expected string
	}{
		{OnChange, "OnChange"},
		{OnDelete, "OnDelete"},
		{OnCreate, "OnCreate"},
		{OnAny, "OnAny"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.action))
			}
		})
	}
}

func TestAnnotationEvent_Fields(t *testing.T) {
	event := AnnotationEvent{
		EventType:  Observe,
		Annotation: "test-annotation",
		OnAction:   OnCreate,
	}

	if event.EventType != Observe {
		t.Errorf("expected EventType %v, got %v", Observe, event.EventType)
	}
	if event.Annotation != "test-annotation" {
		t.Errorf("expected Annotation 'test-annotation', got '%s'", event.Annotation)
	}
	if event.OnAction != OnCreate {
		t.Errorf("expected OnAction %v, got %v", OnCreate, event.OnAction)
	}
}

func TestAnnotationEvents_EdgeCases(t *testing.T) {
	t.Run("has annotation on empty slice", func(t *testing.T) {
		events := NewAnnotationEvents()
		if events.HasAnnotation("any") {
			t.Error("expected false for empty slice")
		}
	})

	t.Run("remove from single item slice", func(t *testing.T) {
		events := NewAnnotationEvents(
			AnnotationEvent{EventType: Observe, Annotation: "only", OnAction: OnCreate},
		)
		events.Remove("only")
		if len(events) != 0 {
			t.Errorf("expected empty slice, got length %d", len(events))
		}
	})

	t.Run("add with overwrite removes all matching annotations", func(t *testing.T) {
		events := NewAnnotationEvents(
			AnnotationEvent{EventType: Observe, Annotation: "duplicate", OnAction: OnCreate},
			AnnotationEvent{EventType: Create, Annotation: "duplicate", OnAction: OnDelete},
			AnnotationEvent{EventType: Observe, Annotation: "other", OnAction: OnAny},
		)

		events.Add(Update, "duplicate", OnChange, true)

		if len(events) != 2 {
			t.Errorf("expected 2 events after overwrite, got %d", len(events))
		}

		duplicateCount := 0
		for _, event := range events {
			if event.Annotation == "duplicate" {
				duplicateCount++
				if event.EventType != Update || event.OnAction != OnChange {
					t.Error("overwritten event has incorrect values")
				}
			}
		}

		if duplicateCount != 1 {
			t.Errorf("expected 1 duplicate annotation after overwrite, got %d", duplicateCount)
		}
	})
}
