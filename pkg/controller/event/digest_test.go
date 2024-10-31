package event

import (
	"testing"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/stretchr/testify/assert"
)

func TestDigestForEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		expected string
	}{
		{
			name: "Test event 1",
			event: Event{
				EventType: "create",
				ObjectRef: objectref.ObjectRef{
					Kind:      "Pod",
					Namespace: "default",
					Name:      "mypod",
				},
			},
			expected: "2555f9c98d0663c1", // Replace with the actual expected hash
		},
		{
			name: "Test event 2",
			event: Event{
				EventType: "delete",
				ObjectRef: objectref.ObjectRef{
					Kind:      "Service",
					Namespace: "default",
					Name:      "myservice",
				},
			},
			expected: "c515572a42a933fd", // Replace with the actual expected hash
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := DigestForEvent(tt.event)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
