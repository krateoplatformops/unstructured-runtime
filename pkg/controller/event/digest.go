package event

import (
	"fmt"
	"strconv"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/twmb/murmur3"
)

type cleanEvent struct {
	eventType string
	objectRef objectref.ObjectRef
}

// digestForEvent generates a hash digest for a given event.
// It uses the Murmur3 hashing algorithm to create a 64-bit hash value.
// The event is first cleaned and then converted to a byte slice before hashing.
// The resulting hash is returned as a hexadecimal string.
//
// Parameters:
// - ev: The event for which the digest is to be generated.
//
// Returns:
// - A hexadecimal string representing the hash digest of the event.
func DigestForEvent(ev Event) string {
	hasher := murmur3.New64()

	cleanEvent := cleanEvent{
		eventType: string(ev.EventType),
		objectRef: ev.ObjectRef,
	}

	bin_buf := []byte(fmt.Sprintf("%v", cleanEvent))

	hasher.Write(bin_buf)

	return strconv.FormatUint(hasher.Sum64(), 16)
}
