package event

import (
	"testing"
)

func TestNewDefaultActionsEvent(t *testing.T) {
	actionsEvent := NewDefaultActionsEvent()

	// Test that all default actions are present
	if actionsEvent[CRCreated] != Observe {
		t.Errorf("Expected CRCreated to have EventType Observe, got %v", actionsEvent[CRCreated])
	}

	if actionsEvent[CRUpdated] != Update {
		t.Errorf("Expected CRUpdated to have EventType Update, got %v", actionsEvent[CRUpdated])
	}

	if actionsEvent[CRDeleted] != Delete {
		t.Errorf("Expected CRDeleted to have EventType Delete, got %v", actionsEvent[CRDeleted])
	}

	if actionsEvent[CRObserved] != Observe {
		t.Errorf("Expected CRObserved to have EventType Observe, got %v", actionsEvent[CRObserved])
	}

	// Test that the map has exactly 3 entries
	if len(actionsEvent) != 4 {
		t.Errorf("Expected ActionsEvent to have 3 entries, got %d", len(actionsEvent))
	}
}

func TestActionsEvent_MutateEvent(t *testing.T) {
	actionsEvent := NewDefaultActionsEvent()

	// Test mutating an existing action
	actionsEvent.MutateEvent(CRCreated, Update)
	if actionsEvent[CRCreated] != Update {
		t.Errorf("Expected CRCreated to be mutated to Update, got %v", actionsEvent[CRCreated])
	}

	// Test mutating another action
	actionsEvent.MutateEvent(CRDeleted, Observe)
	if actionsEvent[CRDeleted] != Observe {
		t.Errorf("Expected CRDeleted to be mutated to Observe, got %v", actionsEvent[CRDeleted])
	}

	// Test adding a new action (if ActionCustomResource allows custom values)
	customAction := ActionCustomResource("CustomAction")
	actionsEvent.MutateEvent(customAction, Delete)
	if actionsEvent[customAction] != Delete {
		t.Errorf("Expected custom action to have EventType Delete, got %v", actionsEvent[customAction])
	}
}

func TestActionCustomResourceConstants(t *testing.T) {
	// Test that constants have expected values
	if CRCreated != "Create" {
		t.Errorf("Expected CRCreated to be 'Create', got %s", CRCreated)
	}

	if CRUpdated != "Update" {
		t.Errorf("Expected CRUpdated to be 'Update', got %s", CRUpdated)
	}

	if CRDeleted != "Delete" {
		t.Errorf("Expected CRDeleted to be 'Delete', got %s", CRDeleted)
	}
}

func TestNil(t *testing.T) {
	var actionsEvent ActionsEvent = ActionsEvent{}

	// Test mutating an action on a nil map
	actionsEvent.MutateEvent(CRCreated, Update)

	if actionsEvent[CRCreated] != Update {
		t.Errorf("Expected CRCreated to be mutated to Update, got %v", actionsEvent[CRCreated])
	}
}

func TestActionsEvent_GetEventType(t *testing.T) {
	defaults := NewDefaultActionsEvent()

	t.Run("nil receiver returns defaults and does not mutate caller", func(t *testing.T) {
		var ae ActionsEvent // nil
		for _, action := range []ActionCustomResource{CRCreated, CRUpdated, CRDeleted} {
			got := ae.GetEventType(action)
			want := defaults[action]
			if got != want {
				t.Fatalf("nil receiver: GetEventType(%s) = %v, want %v", action, got, want)
			}
		}
	})

	t.Run("empty map returns defaults and does not mutate caller", func(t *testing.T) {
		ae := ActionsEvent{} // non-nil, empty
		for _, action := range []ActionCustomResource{CRCreated, CRUpdated, CRDeleted} {
			got := ae.GetEventType(action)
			want := defaults[action]
			if got != want {
				t.Fatalf("empty map: GetEventType(%s) = %v, want %v", action, got, want)
			}
		}
		if len(ae) != 0 {
			t.Fatalf("expected caller map to remain empty, got len=%d", len(ae))
		}
	})

	t.Run("partial map triggers fallback to defaults (current behaviour)", func(t *testing.T) {
		ae := ActionsEvent{
			CRCreated: Update, // partial: only one key
		}
		got := ae.GetEventType(CRCreated)
		want := ae[CRCreated]
		if got != want {
			t.Fatalf("partial map: GetEventType(CRCreated) = %v, want %v (defaults)", got, want)
		}
		// ensure original map wasn't modified
		if v, ok := ae[CRCreated]; !ok || v != Update {
			t.Fatalf("expected original caller map to keep its entry CRCreated=Update, got %v (ok=%v)", v, ok)
		}
		// other keys should fall back to defaults
		for _, action := range []ActionCustomResource{CRUpdated, CRDeleted} {
			got := ae.GetEventType(action)
			want := defaults[action]
			if got != want {
				t.Fatalf("partial map: GetEventType(%s) = %v, want %v (defaults)", action, got, want)
			}
		}
	})

	t.Run("complete map uses stored values", func(t *testing.T) {
		ae := NewDefaultActionsEvent()
		// mutate one entry
		ae.MutateEvent(CRCreated, Update)
		got := ae.GetEventType(CRCreated)
		if got != Update {
			t.Fatalf("complete map: GetEventType(CRCreated) = %v, want %v", got, Update)
		}
	})
}
