package event

type ActionCustomResource string

const (
	CRCreated  ActionCustomResource = "Create"
	CRUpdated  ActionCustomResource = "Update"
	CRDeleted  ActionCustomResource = "Delete"
	CRObserved ActionCustomResource = "Observe"
)

type ActionsEvent map[ActionCustomResource]EventType

var defaultActions = ActionsEvent{
	CRCreated:  Observe, // default to Observe for Create
	CRUpdated:  Update,
	CRDeleted:  Delete,
	CRObserved: Observe,
}

func NewDefaultActionsEvent() ActionsEvent {
	// return a copy to avoid accidental mutation of package-level default
	c := make(ActionsEvent, len(defaultActions))
	for k, v := range defaultActions {
		c[k] = v
	}
	return c
}

// Use pointer receiver so we can allocate and modify the caller's map
func (ae *ActionsEvent) MutateEvent(action ActionCustomResource, eventType EventType) {
	if ae == nil {
		return // defensive: nil pointer receiver shouldn't happen, but avoid panic
	}
	if *ae == nil {
		*ae = make(ActionsEvent)
	}
	(*ae)[action] = eventType
}

// GetEventType returns the stored value when present, otherwise returns the package default.
// It never mutates the caller.
func (ae ActionsEvent) GetEventType(action ActionCustomResource) EventType {
	if ae == nil {
		return defaultActions[action]
	}
	if v, ok := ae[action]; ok {
		return v
	}
	return defaultActions[action]
}
