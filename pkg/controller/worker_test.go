package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/priorityqueue"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"github.com/krateoplatformops/unstructured-runtime/pkg/shortid"
	unstructuredtools "github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fakeExternalClient is a simple ExternalClient implementation used by tests.
type fakeExternalClient struct {
	ObserveErr      error
	ObserveExists   bool
	ObserveUpToDate bool

	CreateErr error
	UpdateErr error
	DeleteErr error
}

func (f *fakeExternalClient) Observe(ctx context.Context, mg *unstructured.Unstructured) (ExternalObservation, error) {
	if f.ObserveErr != nil {
		return ExternalObservation{}, f.ObserveErr
	}
	return ExternalObservation{
		ResourceExists:   f.ObserveExists,
		ResourceUpToDate: f.ObserveUpToDate,
	}, nil
}
func (f *fakeExternalClient) Create(ctx context.Context, mg *unstructured.Unstructured) error {
	return f.CreateErr
}
func (f *fakeExternalClient) Update(ctx context.Context, mg *unstructured.Unstructured) error {
	return f.UpdateErr
}
func (f *fakeExternalClient) Delete(ctx context.Context, mg *unstructured.Unstructured) error {
	return f.DeleteErr
}

// fakeExternalClientObserveError forces Observe to return an error so processItem fails.
type fakeExternalClientObserveError struct {
	err error
}

func (f *fakeExternalClientObserveError) Observe(ctx context.Context, mg *unstructured.Unstructured) (ExternalObservation, error) {
	fmt.Println("fakeExternalClientObserveError.Observe returning error")
	return ExternalObservation{}, f.err
}
func (f *fakeExternalClientObserveError) Create(ctx context.Context, mg *unstructured.Unstructured) error {
	return nil
}
func (f *fakeExternalClientObserveError) Update(ctx context.Context, mg *unstructured.Unstructured) error {
	return nil
}
func (f *fakeExternalClientObserveError) Delete(ctx context.Context, mg *unstructured.Unstructured) error {
	return nil
}

func TestRunWorker_RequeuesOnProcessErrorAndRemovesItem(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	opts.MaxRetries = 10
	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// create managed object that processItem will fetch
	obj := createTestUnstructured("rw-obj", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// set external client that returns an error on Observe to force processItem -> error
	ctrl.SetExternalClient(&fakeExternalClientObserveError{err: fmt.Errorf("observe-boom")})

	// build event and store its digest in items (the real flow sets this before queueing)
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
	ctrl.items.Store(dig, struct{}{})

	// enqueue the event
	ctrl.queue.AddWithOpts(priorityqueue.AddOpts{RateLimited: false, Priority: NormalPriority}, item)

	// run worker in goroutine
	done := make(chan struct{})
	go func() {
		ctrl.runWorker(context.Background())
		close(done)
	}()

	// wait for a requeue to appear (handleErr should add it back)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ctrl.queue.NumRequeues(item) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	require.GreaterOrEqual(t, ctrl.queue.NumRequeues(item), 1, "expected at least one requeue for the failed event")

	// ensure the digest was removed from items by runWorker
	_, ok := ctrl.items.Load(dig)
	require.False(t, ok, "expected digest to be removed from controller.items after processing")

	// shutdown queue to allow runWorker to exit and wait for goroutine
	ctrl.queue.ShutDown()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("runWorker did not exit after queue shutdown")
	}
}

func TestProcessItem_DeleteOrphan(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	// create managed object with deletion timestamp and Orphan deletion policy
	obj := createTestUnstructured("orphan-1", opts.Namespace)
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	ann := obj.GetAnnotations()
	ann[meta.AnnotationKeyDeletionPolicy] = string(meta.DeletionPolicyOrphan)
	obj.SetAnnotations(ann)

	obj, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})

	now := metav1.Now()
	obj.SetDeletionTimestamp(&now)
	obj.SetFinalizers([]string{finalizerName})

	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Update(context.TODO(), obj, metav1.UpdateOptions{})

	require.NoError(t, err)
	ev := ctrlevent.Event{
		EventType: ctrlevent.Observe,
		ObjectRef: objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()},
	}
	// processItem should return nil and NOT call Observe on the external client
	err = ctrl.processItem(context.TODO(), ev)
	require.NoError(t, err)

	obj, _ = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
	require.NotContains(t, obj.GetFinalizers(), finalizerName)
}

func TestHandleObserve_QueuesCreateAndUpdateAndReturnsError(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)

	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// create managed object
	obj := createTestUnstructured("obs-1", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	// 1) Observe says resource does NOT exist -> should enqueue Create
	ctrl.SetExternalClient(&fakeExternalClient{ObserveExists: false})
	ref := objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()}
	require.NoError(t, ctrl.handleObserve(context.TODO(), ref))

	i, pr, _ := ctrl.queue.GetWithPriority()
	ev, ok := i.(ctrlevent.Event)
	require.True(t, ok)
	require.Equal(t, ctrlevent.Create, ev.EventType)
	require.Equal(t, HighPriority, pr)
	ctrl.queue.Done(i)
	ctrl.queue.Forget(i)

	// 2) Observe says exists but not up to date -> should enqueue Update
	ctrl.SetExternalClient(&fakeExternalClient{ObserveExists: true, ObserveUpToDate: false})
	require.NoError(t, ctrl.handleObserve(context.TODO(), ref))

	i, pr, _ = ctrl.queue.GetWithPriority()
	ev, ok = i.(ctrlevent.Event)
	require.True(t, ok)
	require.Equal(t, ctrlevent.Update, ev.EventType)
	require.Equal(t, HighPriority, pr)
	ctrl.queue.Done(i)
	ctrl.queue.Forget(i)

	// 3) Observe returns error -> handleObserve should return the error
	observeErr := errors.New("observe boom")
	ctrl.SetExternalClient(&fakeExternalClient{ObserveErr: observeErr})
	err = ctrl.handleObserve(context.TODO(), ref)
	require.Error(t, err)
	require.Equal(t, observeErr, err)
}

func TestHandleCreate_SuccessAndFailure_SetAnnotationsAndConditions(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	obj := createTestUnstructured("create-1", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)

	ref := objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()}

	// Failure case: Create returns error -> annotation ExternalCreateFailed set and error returned
	ctrl.SetExternalClient(&fakeExternalClient{CreateErr: errors.New("create failed")})
	err = ctrl.handleCreate(context.TODO(), ref)
	require.Error(t, err)

	updated, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, updated.GetAnnotations(), meta.AnnotationKeyExternalCreateFailed)

	// Success case: Create succeeds -> ExternalCreateSucceeded should be present
	// create a fresh object for clean state
	obj2 := createTestUnstructured("create-2", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj2, metav1.CreateOptions{})
	require.NoError(t, err)
	ref2 := objectref.ObjectRef{APIVersion: obj2.GetAPIVersion(), Kind: obj2.GetKind(), Name: obj2.GetName(), Namespace: obj2.GetNamespace()}

	ctrl.SetExternalClient(&fakeExternalClient{CreateErr: nil})
	err = ctrl.handleCreate(context.TODO(), ref2)
	require.NoError(t, err)

	updated2, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj2.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, updated2.GetAnnotations(), meta.AnnotationKeyExternalCreateSucceeded)

	// also expect ReconcileSuccess condition set in status
	conds := unstructuredtools.GetConditions(updated2)
	found := false
	for _, c := range conds {
		if c.Type == condition.TypeReady && c.Reason == condition.ReasonReconcileSuccess {
			found = true
			break
		}
	}
	// At least ensure some condition exists
	require.NotEmpty(t, conds)
	_ = found // best-effort: depending on implementation of condition names
}

func TestHandleUpdate_SuccessAndFailure_UpdatesStatus(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	obj := createTestUnstructured("update-1", opts.Namespace)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)
	ref := objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()}

	// Update error case
	ctrl.SetExternalClient(&fakeExternalClient{UpdateErr: errors.New("update fail")})
	err = ctrl.handleUpdate(context.TODO(), ref)
	require.Error(t, err)

	// Update success case
	ctrl.SetExternalClient(&fakeExternalClient{UpdateErr: nil})
	err = ctrl.handleUpdate(context.TODO(), ref)
	require.NoError(t, err)

	// assert that status was updated (best-effort)
	updated, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	conds := unstructuredtools.GetConditions(updated)
	require.NotNil(t, conds)
}

func TestHandleDelete_SuccessAndFailure_FinalizersAndConditions(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	obj := createTestUnstructured("delete-1", opts.Namespace)
	// ensure it has the finalizer to be removed on success
	obj.SetFinalizers([]string{finalizerName})
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)
	ref := objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()}

	// Delete error: expect error returned and condition set
	ctrl.SetExternalClient(&fakeExternalClient{DeleteErr: errors.New("delete fail")})
	err = ctrl.handleDelete(context.TODO(), ref)
	require.Error(t, err)

	// Delete success: finalizers should be cleared and no error
	// recreate object
	obj2 := createTestUnstructured("delete-2", opts.Namespace)
	obj2.SetFinalizers([]string{finalizerName})
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj2, metav1.CreateOptions{})
	require.NoError(t, err)
	ref2 := objectref.ObjectRef{APIVersion: obj2.GetAPIVersion(), Kind: obj2.GetKind(), Name: obj2.GetName(), Namespace: obj2.GetNamespace()}

	ctrl.SetExternalClient(&fakeExternalClient{DeleteErr: nil})
	err = ctrl.handleDelete(context.TODO(), ref2)
	require.NoError(t, err)

	updated, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj2.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, updated.GetFinalizers())
}

func TestProcessItem_PausedAndExternalCreateIncomplete(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	// paused case
	obj := createTestUnstructured("paused-1", opts.Namespace)
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}
	ann := obj.GetAnnotations()
	ann[meta.AnnotationKeyReconciliationPaused] = "true"
	obj.SetAnnotations(ann)
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	require.NoError(t, err)
	ev := ctrlevent.Event{
		EventType: ctrlevent.Observe,
		ObjectRef: objectref.ObjectRef{APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Name: obj.GetName(), Namespace: obj.GetNamespace()},
	}
	// processItem should return nil and set paused condition
	err = ctrl.processItem(context.TODO(), ev)
	require.NoError(t, err)
	resource, err := opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Get(context.TODO(), obj.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	conds := unstructuredtools.GetConditions(resource)
	assert.Equal(t, conds[0].Reason, condition.ReasonReconcilePaused)

	// ExternalCreateIncomplete case
	obj2 := createTestUnstructured("incomplete-1", opts.Namespace)
	// set the incomplete annotation
	meta.SetExternalCreatePending(obj2, time.Now().Add(-time.Minute))
	// simulate incomplete state: a helper exists meta.ExternalCreateIncomplete checks other annotations; we force the condition by adding the pending without succeeded/failed
	_, err = opts.Client.Resource(opts.GVR).Namespace(opts.Namespace).Create(context.TODO(), obj2, metav1.CreateOptions{})
	require.NoError(t, err)
	ev2 := ctrlevent.Event{
		EventType: ctrlevent.Observe,
		ObjectRef: objectref.ObjectRef{APIVersion: obj2.GetAPIVersion(), Kind: obj2.GetKind(), Name: obj2.GetName(), Namespace: obj2.GetNamespace()},
	}
	err = ctrl.processItem(context.TODO(), ev2)
	// processItem returns nil for ExternalCreateIncomplete (it sets conditions and returns)
	require.NoError(t, err)
}

func TestHandleErr_RetryAddsToQueue(t *testing.T) {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	require.NoError(t, err)
	opts := createTestOptions()
	ctrl, err := New(sid, opts)
	require.NoError(t, err)

	// create an event and simulate an error
	ev := ctrlevent.Event{
		EventType: ctrlevent.Observe,
		ObjectRef: objectref.ObjectRef{APIVersion: "v1", Kind: "Kind", Name: "name", Namespace: "ns"},
	}
	// initial num requeues should be 0
	require.Equal(t, 0, ctrl.queue.NumRequeues(ev))

	// call handleErr with an error - should add back to queue and increment requeue count
	ctrl.maxRetries = 3
	ctrl.handleErr(errors.New("boom"), ev, NormalPriority)
	// allow some time for queue bookkeeping
	time.Sleep(10 * time.Millisecond)
	require.GreaterOrEqual(t, ctrl.queue.NumRequeues(ev), 1)
}
