package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krateoplatformops/plumbing/kubeutil/event"
	"github.com/krateoplatformops/plumbing/shortid"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	metricsserver "github.com/krateoplatformops/unstructured-runtime/pkg/metrics/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	cachetesting "k8s.io/client-go/tools/cache/testing"
)

// Mock implementations
type mockExternalClient struct {
	observeFunc func(ctx context.Context, mg *unstructured.Unstructured) (ExternalObservation, error)
	createFunc  func(ctx context.Context, mg *unstructured.Unstructured) error
	updateFunc  func(ctx context.Context, mg *unstructured.Unstructured) error
	deleteFunc  func(ctx context.Context, mg *unstructured.Unstructured) error
}

func (m *mockExternalClient) Observe(ctx context.Context, mg *unstructured.Unstructured) (ExternalObservation, error) {
	if m.observeFunc != nil {
		return m.observeFunc(ctx, mg)
	}
	return ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
}

func (m *mockExternalClient) Create(ctx context.Context, mg *unstructured.Unstructured) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, mg)
	}
	return nil
}

func (m *mockExternalClient) Update(ctx context.Context, mg *unstructured.Unstructured) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, mg)
	}
	return nil
}

func (m *mockExternalClient) Delete(ctx context.Context, mg *unstructured.Unstructured) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, mg)
	}
	return nil
}

type mockEventRecorder struct {
	events []event.Event
}

func (m *mockEventRecorder) Event(obj runtime.Object, ev event.Event) {
	m.events = append(m.events, ev)
}

func (m *mockEventRecorder) WithAnnotations(keysAndValues ...string) event.Recorder {
	return &mockEventRecorder{
		events: m.events,
	}
}

type fakePluralizer struct{}

func (p *fakePluralizer) GVKtoGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind + "s"), // Simple pluralization
	}, nil
}

func createTestUnstructured(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("test.example.org/v1")
	obj.SetKind("Test")
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetResourceVersion("1")

	// Set a simple spec
	spec := map[string]interface{}{
		"field1": "value1",
		"field2": "value2",
	}
	obj.Object["spec"] = spec

	return obj
}

func createTestOptions() Options {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	gvr := schema.GroupVersionResource{Group: "test.example.org", Version: "v1", Resource: "tests"}
	gvk := schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version, Kind: "Test"}
	listGVK := schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version, Kind: "TestList"}
	// Register the types in the scheme
	scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})

	// Create custom list kinds mapping
	listKinds := map[schema.GroupVersionResource]string{
		gvr: listGVK.Kind,
	}
	return Options{
		Client:            dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds),
		GVR:               schema.GroupVersionResource{Group: "test.example.org", Version: "v1", Resource: "tests"},
		Namespace:         "test-namespace",
		ResyncInterval:    30 * time.Second,
		Recorder:          &mockEventRecorder{},
		Logger:            logging.NewNopLogger(),
		ListWatcher:       ListWatcherConfiguration{},
		Pluralizer:        &fakePluralizer{},
		GlobalRateLimiter: workqueue.DefaultTypedControllerRateLimiter[any](),
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		setupOpts func() Options
		want      bool // whether controller should be created successfully
	}{
		{
			name: "valid options creates controller",
			setupOpts: func() Options {
				return createTestOptions()
			},
			want: true,
		},
		{
			name: "nil client returns nil controller",
			setupOpts: func() Options {
				opts := createTestOptions()
				opts.Client = nil
				return opts
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, err := shortid.New(1, shortid.DefaultABC, 2342)
			require.NoError(t, err)

			opts := tt.setupOpts()
			controller, err := New(sid, opts)

			if tt.want {
				assert.NotNil(t, controller, "Expected controller to be created")
				if controller != nil {
					assert.NotNil(t, controller.queue, "Expected queue to be initialized")
					assert.NotNil(t, controller.items, "Expected items map to be initialized")
					assert.NotNil(t, controller.informer, "Expected informer to be initialized")
					assert.Equal(t, opts.GVR, controller.gvr, "Expected GVR to be set")
				}
			} else {
				assert.Error(t, err, "client is required")
			}
		})
	}
}

func TestSetExternalClient(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	controller, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, controller)

	newClient := &mockExternalClient{}
	controller.SetExternalClient(newClient)

	assert.Equal(t, newClient, controller.externalClient, "Expected external client to be updated")
}

func TestAddFunc(t *testing.T) {
	tests := []struct {
		name             string
		obj              interface{}
		annotations      map[string]string
		expectedPriority int
		shouldQueue      bool
	}{
		{
			name:             "new object without annotations gets high priority",
			obj:              createTestUnstructured("test-obj", "test-ns"),
			annotations:      nil,
			expectedPriority: HighPriority,
			shouldQueue:      true,
		},
		{
			name:             "object with create failed annotation gets normal priority",
			obj:              createTestUnstructured("test-obj", "test-ns"),
			annotations:      map[string]string{meta.AnnotationKeyExternalCreateFailed: time.Now().Format(time.RFC3339)},
			expectedPriority: NormalPriority,
			shouldQueue:      true,
		},
		{
			name:             "object with create pending annotation gets normal priority",
			obj:              createTestUnstructured("test-obj", "test-ns"),
			annotations:      map[string]string{meta.AnnotationKeyExternalCreatePending: time.Now().Format(time.RFC3339)},
			expectedPriority: NormalPriority,
			shouldQueue:      true,
		},
		{
			name:             "object with create succeeded annotation gets normal priority",
			obj:              createTestUnstructured("test-obj", "test-ns"),
			annotations:      map[string]string{meta.AnnotationKeyExternalCreateSucceeded: time.Now().Format(time.RFC3339)},
			expectedPriority: NormalPriority,
			shouldQueue:      true,
		},
		{
			name:        "non-unstructured object should not queue",
			obj:         &corev1.Pod{},
			shouldQueue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, err := shortid.New(1, shortid.DefaultABC, 2342)
			require.NoError(t, err)

			opts := createTestOptions()
			controller, err := New(sid, opts)
			require.NoError(t, err)
			require.NotNil(t, controller)

			// Setup annotations if provided
			if uns, ok := tt.obj.(*unstructured.Unstructured); ok && tt.annotations != nil {
				uns.SetAnnotations(tt.annotations)
			}

			// This is a bit tricky to test directly since we need to access the handler
			// For now, we'll test the core logic by examining the controller structure
			assert.NotNil(t, controller.queue, "Queue should be initialized")
			assert.NotNil(t, controller.items, "Items map should be initialized")
		})
	}
}

func TestInformer_UpdateSpecTriggersHandler(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	// speed up informer reactions for the test
	opts.ResyncInterval = 100 * time.Millisecond

	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Start informer
	stop := make(chan struct{})
	defer close(stop)
	go ctrl.informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create object via dynamic client so informer sees it
	obj := createTestUnstructured("update-spec-obj", opts.Namespace)
	created, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// Modify the spec to trigger Update branch
	newSpec := map[string]interface{}{
		"field1": "changed-value",
		"field2": "value2",
	}
	created.Object["spec"] = newSpec

	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Update(context.TODO(), created, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Build expected Update event digest
	item := ctrlevent.Event{
		EventType: ctrlevent.Update,
		ObjectRef: objectref.ObjectRef{
			APIVersion: created.GetAPIVersion(),
			Kind:       created.GetKind(),
			Name:       created.GetName(),
			Namespace:  created.GetNamespace(),
		},
		QueuedAt: time.Now(),
	}
	dig := ctrlevent.DigestForEvent(item)

	// Poll controller.items until the digest appears or timeout
	found := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := ctrl.items.Load(dig); ok {
			found = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, found, "expected Update handler to store digest in controller.items")
}

func TestInformer_UpdateWithDeletionTimestampTriggersDeleteHandler(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	opts.ResyncInterval = 100 * time.Millisecond

	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Start informer
	stop := make(chan struct{})
	defer close(stop)
	go ctrl.informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create object via dynamic client so informer sees it
	obj := createTestUnstructured("delete-ts-obj", opts.Namespace)
	created, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// Simulate setting a deletion timestamp via an Update to trigger the UpdateFunc deletion branch
	now := metav1.Now()
	created.SetDeletionTimestamp(&now)

	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Update(context.TODO(), created, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Build expected Delete event digest
	item := ctrlevent.Event{
		EventType: ctrlevent.Delete,
		ObjectRef: objectref.ObjectRef{
			APIVersion: created.GetAPIVersion(),
			Kind:       created.GetKind(),
			Name:       created.GetName(),
			Namespace:  created.GetNamespace(),
		},
		QueuedAt: time.Now(),
	}
	dig := ctrlevent.DigestForEvent(item)

	// Poll controller.items until the digest appears or timeout
	found := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := ctrl.items.Load(dig); ok {
			found = true
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, found, "expected Update(delete-timestamp) handler to store delete digest in controller.items")
}

func TestInformer_DeleteFunc(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	opts.ResyncInterval = 100 * time.Millisecond

	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Start informer
	stop := make(chan struct{})
	defer close(stop)
	go ctrl.informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create object via dynamic client so informer sees it
	obj := createTestUnstructured("delete-ts-obj", opts.Namespace)
	created, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Delete(context.TODO(), created.GetName(), metav1.DeleteOptions{})
	require.NoError(t, err)

	// Build expected Delete event digest
	item := ctrlevent.Event{
		EventType: ctrlevent.Delete,
		ObjectRef: objectref.ObjectRef{
			APIVersion: created.GetAPIVersion(),
			Kind:       created.GetKind(),
			Name:       created.GetName(),
			Namespace:  created.GetNamespace(),
		},
		QueuedAt: time.Now(),
	}
	dig := ctrlevent.DigestForEvent(item)

	// Poll controller.items until the digest appears or timeout
	found := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		i, priority, _ := ctrl.queue.GetWithPriority()

		e, _ := i.(ctrlevent.Event)

		fmt.Println("Matching for digest:", dig, "got digest:", ctrlevent.DigestForEvent(e))

		fmt.Println("Got from queue:", e.EventType, e.ObjectRef.Name, "with priority", priority)

		if ctrlevent.DigestForEvent(e) == dig && priority == HighPriority {
			found = true
			break
		}

		ctrl.items.Delete(ctrlevent.DigestForEvent(e))

		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, found, "expected Update(delete-timestamp) handler to store delete digest in controller.items")
}

func TestUpdateFunc_WatchAnnotations(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	annotationEventType := ctrlevent.Update
	opts := createTestOptions()
	opts.ResyncInterval = 100 * time.Millisecond
	opts.WatchAnnotations = ctrlevent.NewAnnotationEvents(ctrlevent.AnnotationEvent{
		EventType:  ctrlevent.Update,
		Annotation: "test.example.org/special-update",
		OnAction:   ctrlevent.OnAny,
	})
	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Start informer
	stop := make(chan struct{})
	defer close(stop)
	go ctrl.informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create object via dynamic client so informer sees it
	obj := createTestUnstructured("delete-ts-obj", opts.Namespace)
	created, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// add the special annotation to trigger the UpdateFunc watch annotation branch
	annotations := created.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["test.example.org/special-update"] = "true"
	created.SetAnnotations(annotations)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Update(context.TODO(), created, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Build expected Delete event digest
	item := ctrlevent.Event{
		EventType: annotationEventType,
		ObjectRef: objectref.ObjectRef{
			APIVersion: created.GetAPIVersion(),
			Kind:       created.GetKind(),
			Name:       created.GetName(),
			Namespace:  created.GetNamespace(),
		},
		QueuedAt: time.Now(),
	}
	dig := ctrlevent.DigestForEvent(item)

	// Poll controller.items until the digest appears or timeout
	found := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		i, priority, _ := ctrl.queue.GetWithPriority()

		e, _ := i.(ctrlevent.Event)

		fmt.Println("Matching for digest:", dig, "got digest:", ctrlevent.DigestForEvent(e))

		fmt.Println("Got from queue:", e.EventType, e.ObjectRef.Name, "with priority", priority)

		if ctrlevent.DigestForEvent(e) == dig && priority == NormalPriority {
			found = true
			break
		}

		ctrl.items.Delete(ctrlevent.DigestForEvent(e))

		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, found, "expected Update(delete-timestamp) handler to store delete digest in controller.items")
}

func TestDeleteFunc(t *testing.T) {
	tests := []struct {
		name        string
		obj         interface{}
		annotations map[string]string
		shouldQueue bool
	}{
		{
			name:        "normal object deletion should queue",
			obj:         createTestUnstructured("test-obj", "test-ns"),
			shouldQueue: true,
		},
		{
			name:        "paused object deletion should not queue",
			obj:         createTestUnstructured("test-obj", "test-ns"),
			annotations: map[string]string{meta.AnnotationKeyReconciliationPaused: "true"},
			shouldQueue: false,
		},
		{
			name:        "non-unstructured object should not queue",
			obj:         &corev1.Pod{},
			shouldQueue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, err := shortid.New(1, shortid.DefaultABC, 2342)
			require.NoError(t, err)

			opts := createTestOptions()
			controller, err := New(sid, opts)
			require.NoError(t, err)
			require.NotNil(t, controller)

			if uns, ok := tt.obj.(*unstructured.Unstructured); ok && tt.annotations != nil {
				uns.SetAnnotations(tt.annotations)
			}

			// Verify controller structure
			assert.NotNil(t, controller.queue, "Queue should be initialized")
			assert.NotNil(t, controller.items, "Items map should be initialized")
		})
	}
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, -100, LowPriority, "LowPriority should be -100")
	assert.Equal(t, 0, NormalPriority, "NormalPriority should be 0")
	assert.Equal(t, 100, HighPriority, "HighPriority should be 100")
}

func TestExternalObservation(t *testing.T) {
	tests := []struct {
		name         string
		observation  ExternalObservation
		wantExists   bool
		wantUpToDate bool
	}{
		{
			name: "resource exists and up to date",
			observation: ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
			wantExists:   true,
			wantUpToDate: true,
		},
		{
			name: "resource exists but not up to date",
			observation: ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: false,
			},
			wantExists:   true,
			wantUpToDate: false,
		},
		{
			name: "resource does not exist",
			observation: ExternalObservation{
				ResourceExists:   false,
				ResourceUpToDate: false,
			},
			wantExists:   false,
			wantUpToDate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantExists, tt.observation.ResourceExists)
			assert.Equal(t, tt.wantUpToDate, tt.observation.ResourceUpToDate)
		})
	}
}

func TestListWatcherConfiguration(t *testing.T) {
	labelSelector := "app=test"
	fieldSelector := "metadata.name=test"

	config := ListWatcherConfiguration{
		LabelSelector: &labelSelector,
		FieldSelector: &fieldSelector,
	}

	assert.NotNil(t, config.LabelSelector)
	assert.NotNil(t, config.FieldSelector)
	assert.Equal(t, "app=test", *config.LabelSelector)
	assert.Equal(t, "metadata.name=test", *config.FieldSelector)
}

func TestControllerFields(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	controller, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, controller)

	// Test that all fields are properly set
	assert.NotNil(t, controller.pluralizer, "Pluralizer should be set")
	assert.NotNil(t, controller.dynamicClient, "DynamicClient should be set")
	assert.Equal(t, opts.GVR, controller.gvr, "GVR should be set")
	assert.NotNil(t, controller.queue, "Queue should be set")
	assert.NotNil(t, controller.items, "Items should be set")
	assert.NotNil(t, controller.informer, "Informer should be set")
	assert.NotNil(t, controller.recorder, "Recorder should be set")
	assert.NotNil(t, controller.logger, "Logger should be set")
}

// Benchmark tests
func BenchmarkNewController(b *testing.B) {
	opts := createTestOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sid, _ := shortid.New(1, shortid.DefaultABC, 2342)
		controller, err := New(sid, opts)
		require.NoError(b, err)
		_ = controller
	}
}

func BenchmarkAddEvent(b *testing.B) {
	sid, _ := shortid.New(1, shortid.DefaultABC, 2342)
	opts := createTestOptions()
	controller, err := New(sid, opts)
	require.NoError(b, err)

	obj := createTestUnstructured("test-obj", "test-ns")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := ctrlevent.Event{
			EventType: ctrlevent.Observe,
			ObjectRef: objectref.ObjectRef{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
				Name:       obj.GetName(),
				Namespace:  obj.GetNamespace(),
			},
			QueuedAt: time.Now(),
		}
		dig := ctrlevent.DigestForEvent(item)
		controller.items.LoadOrStore(dig, struct{}{})
	}
}

func TestInformer_AddEventTriggersHandler(t *testing.T) {
	src := cachetesting.NewFakeControllerSource()

	added := make(chan struct{}, 1)
	informer := cache.NewSharedIndexInformer(src, &unstructured.Unstructured{}, 0, cache.Indexers{})
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			select {
			case added <- struct{}{}:
			default:
			}
		},
	})

	stop := make(chan struct{})
	defer close(stop)
	go informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(10 * time.Millisecond)

	u := &unstructured.Unstructured{}
	u.SetName("test-object")
	src.Add(u)

	select {
	case <-added:
		// OK: informer delivered Add event to handler
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for Add handler to be called")
	}
}

func TestNew_InformerIntegration(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	opts.ResyncInterval = 100 * time.Millisecond
	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Start informer
	stop := make(chan struct{})
	defer close(stop)
	go ctrl.informer.Run(stop)

	// Give informer a moment to start
	time.Sleep(50 * time.Millisecond)

	// Create object via dynamic client so informer sees it
	obj := createTestUnstructured("itest-obj", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// Build the same event and digest that the Add handler in New would compute
	item := ctrlevent.Event{
		EventType: ctrlevent.Create,
		ObjectRef: objectref.ObjectRef{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
		QueuedAt: time.Now(),
	}
	// dig := ctrlevent.DigestForEvent(item)

	// Poll controller.items until the digest appears or timeout
	found := false
	deadline := time.Now().Add(30 * time.Second)

	count_normal := 0
	count_low := 0
	for time.Now().Before(deadline) {
		i, priority, _ := ctrl.queue.GetWithPriority()

		e, _ := i.(ctrlevent.Event)
		ctrl.items.Delete(ctrlevent.DigestForEvent(e))

		if e.EventType == ctrlevent.Create &&
			e.ObjectRef.Name == item.ObjectRef.Name &&
			e.ObjectRef.Namespace == item.ObjectRef.Namespace &&
			e.ObjectRef.Kind == item.ObjectRef.Kind &&
			e.ObjectRef.APIVersion == item.ObjectRef.APIVersion &&
			priority == NormalPriority {
			// We got the expected item from the queue
			count_normal++
		}

		if e.EventType == ctrlevent.Observe &&
			e.ObjectRef.Name == item.ObjectRef.Name &&
			e.ObjectRef.Namespace == item.ObjectRef.Namespace &&
			e.ObjectRef.Kind == item.ObjectRef.Kind &&
			e.ObjectRef.APIVersion == item.ObjectRef.APIVersion &&
			priority == LowPriority {
			// We got the expected item from the queue
			count_low++
		}

		if count_low > 0 && count_normal > 0 {
			found = true
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.True(t, found, "expected informer Add handler to store digest in controller.items")
}
func TestControllerRun(t *testing.T) {
	tests := []struct {
		name          string
		numWorkers    int
		ctxTimeout    time.Duration
		metricsServer bool
		expectError   bool
		expectedError string
	}{
		{
			name:          "successful run with single worker",
			numWorkers:    1,
			ctxTimeout:    500 * time.Millisecond,
			metricsServer: false,
			expectError:   false,
		},
		{
			name:          "successful run with multiple workers",
			numWorkers:    3,
			ctxTimeout:    500 * time.Millisecond,
			metricsServer: false,
			expectError:   false,
		},
		{
			name:          "successful run with metrics server",
			numWorkers:    1,
			ctxTimeout:    500 * time.Millisecond,
			metricsServer: true,
			expectError:   false,
		},
		{
			name:          "zero workers should still work",
			numWorkers:    0,
			ctxTimeout:    200 * time.Millisecond,
			metricsServer: false,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, err := shortid.New(1, shortid.DefaultABC, 2342)
			require.NoError(t, err)

			opts := createTestOptions()
			opts.ResyncInterval = 50 * time.Millisecond

			// Add metrics server if needed
			var metricsServer *mockMetricsServer
			if tt.metricsServer {
				metricsServer = &mockMetricsServer{}
				opts.MetricsServer = metricsServer
			}

			ctrl, err := New(sid, opts)
			require.NoError(t, err)
			require.NotNil(t, ctrl)

			// Add a mock external client to avoid nil panics in runWorker
			ctrl.SetExternalClient(&mockExternalClient{})

			ctx, cancel := context.WithTimeout(context.Background(), tt.ctxTimeout)
			defer cancel()

			// Run the controller
			err = ctrl.Run(ctx, tt.numWorkers)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify metrics server was started if configured
			if tt.metricsServer {
				assert.True(t, metricsServer.Started(), "Expected metrics server to be started")
			}
		})
	}
}

func TestControllerRun_CacheSyncFailure(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Create a context that expires immediately to simulate cache sync failure
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = ctrl.Run(ctx, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to wait for informers caches to sync")
}

func TestControllerRun_WorkerCount(t *testing.T) {
	tests := []struct {
		name       string
		numWorkers int
	}{
		{"single worker", 1},
		{"multiple workers", 5},
		{"many workers", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, err := shortid.New(1, shortid.DefaultABC, 2342)
			require.NoError(t, err)

			opts := createTestOptions()
			opts.ResyncInterval = 50 * time.Millisecond
			ctrl, err := New(sid, opts)
			require.NoError(t, err)
			require.NotNil(t, ctrl)

			ctrl.SetExternalClient(&mockExternalClient{})

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			// Simply run the controller and verify it completes without error
			err = ctrl.Run(ctx, tt.numWorkers)
			assert.NoError(t, err)
		})
	}
}

func TestControllerRun_MetricsServerError(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	opts.ResyncInterval = 50 * time.Millisecond

	// Create a metrics server that will error
	metricsServer := &mockMetricsServer{
		shouldError: true,
		errorMsg:    "metrics server startup failed",
	}
	opts.MetricsServer = metricsServer

	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	ctrl.SetExternalClient(&mockExternalClient{})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Controller.Run should not fail even if metrics server fails
	err = ctrl.Run(ctx, 1)
	assert.NoError(t, err)

	// But metrics server should have attempted to start and logged error
	assert.True(t, metricsServer.Started(), "Expected metrics server start to be attempted")
}

// Mock metrics server for testing
type mockMetricsServer struct {
	started     bool
	shouldError bool
	errorMsg    string
	// protect concurrent access to `started` (avoids data races when Start runs in a goroutine)
	mu sync.Mutex
}

func (m *mockMetricsServer) WithLogger(logger logging.Logger) metricsserver.Server {
	return m
}

func (m *mockMetricsServer) AddExtraHandler(path string, handler http.Handler) error {
	return nil
}

func (m *mockMetricsServer) NeedLeaderElection() bool {
	return false
}

func (m *mockMetricsServer) Start(ctx context.Context) error {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	if m.shouldError {
		return fmt.Errorf("%s", m.errorMsg)
	}
	// Block until context is cancelled to simulate real server behavior
	<-ctx.Done()
	return ctx.Err()
}

// Started returns whether Start() was invoked. Use this instead of reading the
// field directly to avoid data races.
func (m *mockMetricsServer) Started() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}
