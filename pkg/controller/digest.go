package controller

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

func digestForEvent(ev Event) string {
	hasher := murmur3.New64()

	cleanEvent := cleanEvent{
		eventType: string(ev.EventType),
		objectRef: ev.ObjectRef,
	}

	bin_buf := []byte(fmt.Sprintf("%v", cleanEvent))

	hasher.Write(bin_buf)

	return strconv.FormatUint(hasher.Sum64(), 16)
}
