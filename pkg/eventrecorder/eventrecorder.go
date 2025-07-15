package eventrecorder

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	record "k8s.io/client-go/tools/events"
)

func Create(ctx context.Context, rc *rest.Config) (record.EventRecorder, error) {
	clientset, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = eventsv1.AddToScheme(scheme)

	eventBroadcaster := record.NewBroadcaster(&record.EventSinkImpl{
		Interface: clientset.EventsV1(),
	})
	eventBroadcaster.StartStructuredLogging(4)
	eventBroadcaster.StartRecordingToSinkWithContext(ctx)
	return eventBroadcaster.NewRecorder(scheme, "unstructured-runtime"), nil
}
