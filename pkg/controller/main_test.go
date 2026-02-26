package controller

import (
	"os"
	"testing"

	clientfeatures "k8s.io/client-go/features"
)

func TestMain(m *testing.M) {
	// The WatchListClient feature gate is enabled by default in client-go v0.35+.
	// The fake dynamic client used in these tests cannot handle the streaming
	// WatchList protocol (it never emits the required BOOKMARK events), so the
	// reflector would wait indefinitely for a bookmark and never deliver events
	// to the informer handlers. Disabling the gate here switches the reflector
	// back to the classic List-then-Watch path for the entire test binary.
	type featureSetter interface {
		Set(clientfeatures.Feature, bool) error
	}
	if fg, ok := clientfeatures.FeatureGates().(featureSetter); ok {
		if err := fg.Set(clientfeatures.WatchListClient, false); err != nil {
			panic("failed to disable WatchListClient feature gate: " + err.Error())
		}
	}
	os.Exit(m.Run())
}
