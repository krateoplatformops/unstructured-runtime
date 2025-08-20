package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/krateoplatformops/plumbing/kubeutil/event"
	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/priorityqueue"
	"github.com/krateoplatformops/unstructured-runtime/pkg/errors"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools"
	unstructuredtools "github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"slices"

	"k8s.io/apimachinery/pkg/util/runtime"
)

const (
	maxRetries    = 5
	finalizerName = "composition.krateo.io/finalizer"
)

// Event reasons.
const (
	reasonCannotInitialize    event.Reason = "CannotInitializeManagedResource"
	reasonCannotResolveRefs   event.Reason = "CannotResolveResourceReferences"
	reasonCannotObserve       event.Reason = "CannotObserveExternalResource"
	reasonCannotCreate        event.Reason = "CannotCreateExternalResource"
	reasonCannotDelete        event.Reason = "CannotDeleteExternalResource"
	reasonCannotPublish       event.Reason = "CannotPublishConnectionDetails"
	reasonCannotUnpublish     event.Reason = "CannotUnpublishConnectionDetails"
	reasonCannotUpdate        event.Reason = "CannotUpdateExternalResource"
	reasonCannotUpdateManaged event.Reason = "CannotUpdateManagedResource"
	reasonCannotSetConditions event.Reason = "CannotSetConditions"
	reasonCannotFetchManaged  event.Reason = "CannotFetchManagedResource"

	reasonDeleted event.Reason = "DeletedExternalResource"
	reasonCreated event.Reason = "CreatedExternalResource"
	reasonUpdated event.Reason = "UpdatedExternalResource"
	reasonPending event.Reason = "PendingExternalResource"
)

// Event actions.
const (
	actionProcessEvent            event.Action = "ProcessEvent"
	actionCreateEvent             event.Action = "CreateEvent"
	actionUpdateEvent             event.Action = "UpdateEvent"
	actionDeleteEvent             event.Action = "DeleteEvent"
	actionObserveEvent            event.Action = "ObserveEvent"
	actionCreateExternalResource  event.Action = "CreateExternalResource"
	actionUpdateExternalResource  event.Action = "UpdateExternalResource"
	actionDeleteExternalResource  event.Action = "DeleteExternalResource"
	actionObserveExternalResource event.Action = "ObserveExternalResource"

	actionFetchManagedResource  event.Action = "FetchManagedResource"
	actionUpdateManagedResource event.Action = "UpdateManagedResource"
)

// Error strings.
const (
	errGetManaged                = "cannot get managed resource"
	errUpdateManagedAnnotations  = "cannot update managed resource annotations"
	errCreateIncomplete          = "cannot determine creation result - remove the " + meta.AnnotationKeyExternalCreatePending + " annotation if it is safe to proceed"
	errReconcileConnect          = "connect failed"
	errReconcileObserve          = "observe failed"
	errReconcileCreate           = "create failed"
	errReconcileUpdate           = "update failed"
	errReconcileDelete           = "delete failed"
	errExternalResourceNotExist  = "external resource does not exist"
	errCreateOrUpdateSecret      = "cannot create or update connection secret"
	errUpdateManaged             = "cannot update managed resource"
	errUpdateManagedStatus       = "cannot update managed resource status"
	errResolveReferences         = "cannot resolve references"
	errUpdateCriticalAnnotations = "cannot update critical annotations"
)

func (c *Controller) recordMetric(evt ctrlevent.Event, operation string, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}

	reconcileTotal.WithLabelValues(
		evt.ObjectRef.Kind,
		evt.ObjectRef.Namespace,
		result,
	).Inc()
}

func (c *Controller) runWorker(ctx context.Context) {
	// Generate unique worker ID for tracking
	workerID := fmt.Sprintf("worker-%d", time.Now().UnixNano()%10000)

	for {
		obj, priority, shutdown := c.queue.GetWithPriority()
		if shutdown {
			break
		}

		ev, ok := obj.(ctrlevent.Event)
		if !ok {
			c.queue.Forget(obj)
			c.queue.Done(obj)
			runtime.HandleError(fmt.Errorf("unexpected object in queue: %v", obj))
			continue
		}

		dig := ctrlevent.DigestForEvent(ev)

		// Start timing for processing
		processingStartTime := time.Now()
		queueLength := c.queue.Len()

		// Calculate queue wait time
		var queueWaitDuration time.Duration
		if !ev.QueuedAt.IsZero() {
			queueWaitDuration = processingStartTime.Sub(ev.QueuedAt)
		}

		err := c.processItem(ctx, ev)

		// End timing and calculate durations
		processingEndTime := time.Now()
		processingDuration := processingEndTime.Sub(processingStartTime)

		var totalDuration time.Duration
		if !ev.QueuedAt.IsZero() {
			totalDuration = processingEndTime.Sub(ev.QueuedAt)
		} else {
			totalDuration = processingDuration
		}

		// Enhanced debug logging
		c.logger.WithValues(
			"kind", ev.ObjectRef.Kind,
			"apiVersion", ev.ObjectRef.APIVersion,
			"name", ev.ObjectRef.Name,
			"namespace", ev.ObjectRef.Namespace,
			"queuedAt", ev.QueuedAt,
		).Debug("Event processed",
			"priority", priority,
			"type", ev.EventType,
			"queueWaitTime", queueWaitDuration.String(),
			"processingTime", processingDuration.String(),
			"totalTime", totalDuration.String(),
			"success", err == nil,
			"workerID", workerID,
			"queueLength", queueLength,
		)

		c.items.Delete(dig)
		c.queue.Forget(ev)
		c.queue.Done(ev)
		c.handleErr(err, ev, priority)
	}
}

func (c *Controller) handleErr(err error, obj ctrlevent.Event, priority int) {
	if err == nil {
		return
	}

	c.logger.WithValues("retries", c.queue.NumRequeues(obj)).
		Debug("processing event, retrying", "error", err)

	// Preserve original queue timestamp for retry
	retryEvent := obj
	if retryEvent.QueuedAt.IsZero() {
		retryEvent.QueuedAt = time.Now() // Set timestamp if missing
	}

	c.queue.AddWithOpts(priorityqueue.AddOpts{
		RateLimited: true, // Always rate limit retries to avoid overwhelming the controller
		Priority:    priority,
	}, obj)
}

func (c *Controller) processItem(ctx context.Context, obj interface{}) error {
	evt, ok := obj.(ctrlevent.Event)
	if !ok {
		c.logger.Debug("unexpected event", "object", obj)
		return nil
	}

	c.logger = c.logger.WithValues(
		"kind", evt.ObjectRef.Kind,
		"apiVersion", evt.ObjectRef.APIVersion,
		"name", evt.ObjectRef.Name,
		"namespace", evt.ObjectRef.Namespace,
		"queuedAt", evt.QueuedAt,
	)

	lg := c.logger

	el, err := c.fetch(ctx, evt.ObjectRef, false)
	if err != nil {
		lg.Debug("Object not found")
		// if the object is not found, we will not retry to process it and we not throw an error
		return nil
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	if meta.IsPaused(el) {
		log := lg.WithValues("annotation", meta.AnnotationKeyReconciliationPaused)

		log.Debug("Reconciliation is paused via the pause annotation")
		c.recorder.Event(el, event.Normal(reasonReconciliationPaused, actionProcessEvent, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetConditions(el, condition.ReconcilePaused())
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}

		_, err := resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Updating status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateExternalResource, err))
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	// If managed resource has a deletion timestamp and and a deletion policy of
	// Orphan, we do not need to observe the external resource before attempting
	// to remove finalizer.
	if meta.WasDeleted(el) && !meta.ShouldDelete(el) {
		log := lg.WithValues("deletion-timestamp", el.GetDeletionTimestamp())

		if slices.Contains(el.GetFinalizers(), finalizerName) {
			meta.RemoveFinalizer(el, finalizerName)
			_, err = resourceCli.Update(context.Background(), el, metav1.UpdateOptions{})
			if err != nil {
				log.Debug("Removing finalizer", "error", err)
				c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
				return err
			}
		}
	}

	// If we started but never completed creation of an external resource we
	// may have lost critical information. For example if we didn't persist
	// an updated external name we've leaked a resource. The safest thing to
	// do is to refuse to proceed.
	if meta.ExternalCreateIncomplete(el) {
		lg.Debug(errCreateIncomplete)
		c.recorder.Event(el, event.Warning(reasonCannotInitialize, actionProcessEvent, errors.New(errCreateIncomplete)))

		err = unstructuredtools.SetConditions(el,
			condition.Creating(),
			condition.ReconcileError(errors.New(errCreateIncomplete)),
		)
		if err != nil {
			lg.Debug("Cannot set condition", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}

		_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			lg.Debug("Updating status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}

		return nil
	}

	if !meta.WasDeleted(el) {
		finalizers := el.GetFinalizers()
		exist := slices.Contains(finalizers, finalizerName)

		if !exist {
			el.SetFinalizers(append(finalizers, finalizerName))
			_, err = resourceCli.Update(context.Background(), el, metav1.UpdateOptions{})
			if err != nil {
				lg.Debug("Adding finalizer", "error", err)
				c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
				return err
			}
		}
	}

	lg.Debug("Start processing")
	switch evt.EventType {
	case ctrlevent.Create:
		err = c.handleCreate(ctx, evt.ObjectRef)
		c.recordMetric(evt, "create", err)
		return err
	case ctrlevent.Update:
		err = c.handleUpdate(ctx, evt.ObjectRef)
		c.recordMetric(evt, "update", err)
		return err
	case ctrlevent.Delete:
		err = c.handleDelete(ctx, evt.ObjectRef)
		c.recordMetric(evt, "delete", err)
		return err
	default:
		err = c.handleObserve(ctx, evt.ObjectRef)
		c.recordMetric(evt, "observe", err)
		return err
	}
}

func (c *Controller) handleObserve(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.Debug("No event handler registered.")
		return nil
	}

	log := c.logger

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}
	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	observation, actionErr := c.externalClient.Observe(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotObserve, actionObserveExternalResource, actionErr))
		if apierrors.IsNotFound(actionErr) {
			item := ctrlevent.Event{
				EventType: ctrlevent.Update,
				ObjectRef: objectref.ObjectRef{
					APIVersion: el.GetAPIVersion(),
					Kind:       el.GetKind(),
					Name:       el.GetName(),
					Namespace:  el.GetNamespace(),
				},
				QueuedAt: time.Now(),
			}
			c.queue.AddWithOpts(priorityqueue.AddOpts{
				RateLimited: false,
				Priority:    NormalPriority,
			}, item)
			return nil
		}

		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Cannot fetch managed resource")
			c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
			return err
		}

		err = unstructuredtools.SetConditions(e, condition.ReconcileError(errors.Wrap(actionErr, errReconcileObserve)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			c.logger.Debug("Observe: updating status", "error", err)
			c.recorder.Event(e, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}
		return actionErr
	}

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	if !observation.ResourceExists {
		item := ctrlevent.Event{
			EventType: ctrlevent.Create,
			ObjectRef: objectref.ObjectRef{
				APIVersion: el.GetAPIVersion(),
				Kind:       el.GetKind(),
				Name:       el.GetName(),
				Namespace:  el.GetNamespace(),
			},
			QueuedAt: time.Now(),
		}
		c.queue.AddWithOpts(priorityqueue.AddOpts{
			RateLimited: false,
			Priority:    NormalPriority,
		}, item)
		return nil
	} else if !observation.ResourceUpToDate {
		item := ctrlevent.Event{
			EventType: ctrlevent.Update,
			ObjectRef: objectref.ObjectRef{
				APIVersion: el.GetAPIVersion(),
				Kind:       el.GetKind(),
				Name:       el.GetName(),
				Namespace:  el.GetNamespace(),
			},
			QueuedAt: time.Now(),
		}
		c.queue.AddWithOpts(priorityqueue.AddOpts{
			RateLimited: false,
			Priority:    NormalPriority,
		}, item)
		return nil
	} else {
		err = unstructuredtools.SetConditions(el, condition.ReconcileSuccess())
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}
		_, err = resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}
		log.Info("External resource is up to date")
	}

	return nil
}

func (c *Controller) handleCreate(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.Debug("No event handler registered.")
		return nil
	}
	log := c.logger
	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	// We write this annotation for two reasons. Firstly, it helps
	// us to detect the case in which we fail to persist critical
	// information (like the external name) that may be set by the
	// subsequent external.Create call. Secondly, it guarantees that
	// we're operating on the latest version of our resource. We
	// don't use the CriticalAnnotationUpdater because we _want_ the
	// update to fail if we get a 409 due to a stale version.
	meta.SetExternalCreatePending(el, time.Now())
	if _, uerr := resourceCli.Update(ctx, el, metav1.UpdateOptions{}); uerr != nil {
		log.Debug(errUpdateManaged, "error", uerr)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionCreateEvent, errors.Wrap(uerr, errUpdateManaged)))
		// If we cannot update the managed resource, we set the conditions
		el, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Cannot fetch managed resource")
			c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
			return err
		}
		err = unstructuredtools.SetConditions(el, condition.Creating(), condition.ReconcileError(errors.Wrap(uerr, errUpdateManaged)))
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}
		_, err = resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}
		return errors.Wrap(uerr, errUpdateManaged)
	}

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	actionErr := c.externalClient.Create(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotCreate, actionCreateExternalResource, actionErr))
		log.Debug("Cannot create external resource", "error", actionErr)
		el, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Cannot fetch managed resource")
			c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
			return err
		}

		meta.SetExternalCreateFailed(el, time.Now())
		err = unstructuredtools.SetConditions(el, condition.Creating(), condition.ReconcileError(errors.Wrap(actionErr, errReconcileCreate)))
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}
		_, err = tools.Update(ctx, el, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}

		return actionErr
	}

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	meta.SetExternalCreateSucceeded(el, time.Now())
	if _, uerr := resourceCli.Update(ctx, el, metav1.UpdateOptions{}); uerr != nil {
		log.Debug(errUpdateManagedAnnotations, "error", uerr)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, errors.Wrap(uerr, errUpdateManagedAnnotations)))
		// If we cannot update the managed resource, we set the conditions
		el, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Cannot fetch managed resource")
			c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
			return err
		}
		err = unstructuredtools.SetConditions(el, condition.Creating(), condition.ReconcileError(errors.Wrap(uerr, errUpdateManagedAnnotations)))
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}
		_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}
		return errors.Wrap(uerr, errUpdateManagedAnnotations)
	}

	log.Debug("Successfully requested creation of external resource")

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}
	err = unstructuredtools.SetConditions(el, condition.ReconcileSuccess())
	if err != nil {
		log.Debug("Cannot set conditions", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
		return err
	}
	_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
		Pluralizer:    c.pluralizer,
		DynamicClient: c.dynamicClient,
	})
	if err != nil {
		log.Debug("Cannot update status", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
		return err
	}
	c.recorder.Event(el, event.Normal(reasonCreated, actionCreateEvent, "Successfully requested creation of external resource"))
	return nil
}

func (c *Controller) handleUpdate(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.Debug("No event handler registered.")
		return nil
	}

	log := c.logger

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	actionErr := c.externalClient.Update(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotUpdate, actionUpdateEvent, actionErr))
		log.Debug("Cannot update external resource", "error", actionErr)
		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Cannot fetch managed resource")
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}

		err = unstructuredtools.SetConditions(e, condition.ReconcileError(errors.Wrap(actionErr, errReconcileUpdate)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}

		return actionErr
	}

	log.Debug("Successfully requested update of external resource")

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot fetch managed resource")
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	err = unstructuredtools.SetConditions(el, condition.ReconcileSuccess())
	if err != nil {
		log.Debug("Cannot set conditions", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
		return err
	}
	_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
		Pluralizer:    c.pluralizer,
		DynamicClient: c.dynamicClient,
	})
	if err != nil {
		log.Debug("Cannot update status", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
		return err
	}
	c.recorder.Event(el, event.Normal(reasonUpdated, actionUpdateEvent, "Successfully requested update of external resource"))
	return nil
}

func (c *Controller) handleDelete(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.Debug("No event handler registered.")
		return nil
	}

	log := c.logger

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot resolve unstructured object", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}
	actionErr := c.externalClient.Delete(ctx, el)
	if actionErr != nil {
		log.Debug("Cannot delete external resource", "error", actionErr)
		c.recorder.Event(el, event.Warning(reasonCannotDelete, actionDeleteExternalResource, actionErr))

		err := unstructuredtools.SetConditions(el, condition.Deleting(), condition.ReconcileError(errors.Wrap(actionErr, errReconcileDelete)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
			return err
		}

		_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
			return err
		}
		return actionErr
	}

	log.Debug("Successfully requested deletion of external resource")

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot resolve unstructured object", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotFetchManaged, actionFetchManagedResource, err))
		return err
	}

	el.SetFinalizers([]string{})
	err = unstructuredtools.SetConditions(el, condition.Deleting(), condition.ReconcileSuccess())
	if err != nil {
		log.Debug("Cannot set conditions", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotSetConditions, actionUpdateManagedResource, err))
		return err
	}
	_, err = tools.Update(ctx, el, tools.UpdateOptions{
		Pluralizer:    c.pluralizer,
		DynamicClient: c.dynamicClient,
	})
	if err != nil {
		log.Debug("Cannot update resource", "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, actionUpdateManagedResource, err))
		return err
	}
	c.recorder.Event(el, event.Normal(reasonDeleted, actionDeleteEvent, "Successfully requested deletion of external resource"))
	return nil
}

func (c *Controller) fetch(ctx context.Context, ref objectref.ObjectRef, clean bool) (*unstructured.Unstructured, error) {
	res, err := c.dynamicClient.Resource(c.gvr).
		Namespace(ref.Namespace).
		Get(ctx, ref.Name, metav1.GetOptions{})
	if err == nil {
		if clean && res != nil {
			unstructured.RemoveNestedField(res.Object,
				"metadata", "annotations", "kubectl.kubernetes.io/last-applied-configuration")
			unstructured.RemoveNestedField(res.Object, "metadata", "creationTimestamp")
			unstructured.RemoveNestedField(res.Object, "metadata", "generation")
			unstructured.RemoveNestedField(res.Object, "metadata", "uid")
		}
	}
	return res, err
}
