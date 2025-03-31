package controller

import (
	"context"
	"fmt"
	"time"

	ctrlevent "github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	"github.com/krateoplatformops/unstructured-runtime/pkg/errors"
	"github.com/krateoplatformops/unstructured-runtime/pkg/event"
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

	reasonDeleted event.Reason = "DeletedExternalResource"
	reasonCreated event.Reason = "CreatedExternalResource"
	reasonUpdated event.Reason = "UpdatedExternalResource"
	reasonPending event.Reason = "PendingExternalResource"
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

func (c *Controller) runWorker(ctx context.Context) {
	for {
		obj, shutdown := c.queue.Get()
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

		err := c.processItem(ctx, ev)
		c.items.Delete(dig)
		c.queue.Forget(ev)
		c.queue.Done(ev)
		c.handleErr(err, ev)
	}
}

func (c *Controller) handleErr(err error, obj ctrlevent.Event) {
	if err == nil {
		return
	}

	c.logger.WithValues("retries", c.queue.NumRequeues(obj)).
		WithValues("obj", fmt.Sprintf("%v", obj.EventType)).
		Debug("processing event, retrying", "error", err)

	c.queue.AddRateLimited(obj)
}

func (c *Controller) processItem(ctx context.Context, obj interface{}) error {
	evt, ok := obj.(ctrlevent.Event)
	if !ok {
		c.logger.Debug("unexpected event", "object", obj)
		return nil
	}

	el, err := c.fetch(ctx, evt.ObjectRef, false)
	if err != nil {
		// if the object is not found, we will not retry to process it and we not throw an error
		return nil
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	// If managed resource has a deletion timestamp and and a deletion policy of
	// Orphan, we do not need to observe the external resource before attempting
	// to remove finalizer.
	if meta.WasDeleted(el) && !meta.ShouldDelete(el) {
		log := c.logger.WithValues("deletion-timestamp", el.GetDeletionTimestamp())

		if slices.Contains(el.GetFinalizers(), finalizerName) {
			meta.RemoveFinalizer(el, finalizerName)
			_, err = resourceCli.Update(context.Background(), el, metav1.UpdateOptions{})
			if err != nil {
				log.Debug("Removing finalizer", "error", err)
				return err
			}
		}
	}

	if !meta.WasDeleted(el) {
		finalizers := el.GetFinalizers()
		exist := slices.Contains(finalizers, finalizerName)

		if !exist {
			el.SetFinalizers(append(finalizers, finalizerName))
			_, err = resourceCli.Update(context.Background(), el, metav1.UpdateOptions{})
			if err != nil {
				c.logger.Debug("Adding finalizer", "error", err)
				return err
			}
		}
	}
	// else if meta.ShouldDelete(el) {
	// 	c.logger.Debug("processing deletion", "objectRef", evt.ObjectRef.String())
	// 	err = c.handleDelete(ctx, evt.ObjectRef)
	// 	if err != nil {
	// 		c.logger.Debug("deleting", "error", err)
	// 		return err
	// 	}
	// 	return nil
	// }

	c.logger.Debug("processing", "objectRef", evt.ObjectRef.String())
	switch evt.EventType {
	case ctrlevent.Create:
		return c.handleCreate(ctx, evt.ObjectRef)
	case ctrlevent.Update:
		return c.handleUpdate(ctx, evt.ObjectRef)
	case ctrlevent.Delete:
		return c.handleDelete(ctx, evt.ObjectRef)
	default:
		return c.handleObserve(ctx, evt.ObjectRef)
	}
}

func (c *Controller) handleObserve(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(ctrlevent.Observe)).Debug("No event handler registered.")
		return nil
	}

	log := c.logger.WithValues("objectRef", ref.String())

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Resolving unstructured object")
		return err
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	if meta.IsPaused(el) {
		log.Debug("Reconciliation is paused via the pause annotation.", "annotation", meta.AnnotationKeyReconciliationPaused, "value", "true")
		c.recorder.Event(el, event.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetConditions(el, condition.ReconcilePaused())
		if err != nil {
			log.Debug("Cannot set cond", "error", err)
			return err
		}

		_, err := resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Updating status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	observation, actionErr := c.externalClient.Observe(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotObserve, actionErr))
		if apierrors.IsNotFound(actionErr) {
			return c.externalClient.Update(ctx, el)
		}

		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetConditions(e, condition.ReconcileError(errors.Wrap(actionErr, errReconcileObserve)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			c.logger.Debug("Observe: updating status", "error", err)
			return err
		}
		return actionErr
	}
	if !observation.ResourceExists && meta.ShouldCreate(el) {
		return c.handleCreate(ctx, ref)
	} else if observation.ResourceExists && !observation.ResourceUpToDate && meta.ShouldUpdate(el) {
		return c.handleUpdate(ctx, ref)
	}

	return nil
}

func (c *Controller) handleCreate(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(ctrlevent.Create)).Debug("No event handler registered.")
		return nil
	}

	log := c.logger.WithValues("objectRef", ref.String())
	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Resolving unstructured object")
		return err
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	if !meta.ShouldCreate(el) {
		log.Debug("Managed resource is not marked for creation")
		return nil
	}

	// We write this annotation for two reasons. Firstly, it helps
	// us to detect the case in which we fail to persist critical
	// information (like the external name) that may be set by the
	// subsequent external.Create call. Secondly, it guarantees that
	// we're operating on the latest version of our resource. We
	// don't use the CriticalAnnotationUpdater because we _want_ the
	// update to fail if we get a 409 due to a stale version.
	meta.SetExternalCreatePending(el, time.Now())
	if el, uerr := resourceCli.Update(ctx, el, metav1.UpdateOptions{}); uerr != nil {
		log.Debug(errUpdateManaged, "error", err)
		c.recorder.Event(el, event.Warning(reasonCannotUpdateManaged, errors.Wrap(err, errUpdateManaged)))
		unstructuredtools.SetConditions(el, condition.Creating(), condition.ReconcileError(errors.Wrap(err, errUpdateManaged)))

		_, err := resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}
		return errors.Wrap(uerr, errUpdateManaged)
	}

	if meta.IsPaused(el) {
		log.Debug("Reconciliation is paused via the pause annotation")
		c.recorder.Event(el, event.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetConditions(el, condition.ReconcilePaused())
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			return err
		}

		_, err := resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	el, err = c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Resolving unstructured object")
		return err
	}

	actionErr := c.externalClient.Create(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotCreate, actionErr))
		log.Debug("Cannot create external resource", "error", actionErr)
		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetConditions(e, condition.Creating(), condition.ReconcileError(errors.Wrap(actionErr, errReconcileCreate)))
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}

		return actionErr
	}

	log.Debug("Successfully requested creation of external resource")
	c.recorder.Event(el, event.Normal(reasonCreated, "Successfully requested creation of external resource"))
	return nil
}

func (c *Controller) handleUpdate(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(ctrlevent.Update)).Debug("No event handler registered.")
		return nil
	}

	log := c.logger.WithValues("objectRef", ref.String())

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Resolving unstructured object")
		return err
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	if !meta.ShouldUpdate(el) {
		log.Debug("Managed resource is not marked for update")
		return nil
	}

	if meta.IsPaused(el) {
		log.WithValues("annotation", meta.AnnotationKeyReconciliationPaused).Debug("Reconciliation is paused via the pause annotation")
		c.recorder.Event(el, event.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetConditions(el, condition.ReconcilePaused())
		if err != nil {
			log.Debug("Cannot set conditions", "error", err)
			return err
		}

		_, err := resourceCli.UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	actionErr := c.externalClient.Update(ctx, el)
	if actionErr != nil {
		c.recorder.Event(el, event.Warning(reasonCannotUpdate, actionErr))
		log.Debug("Cannot update external resource", "error", actionErr)
		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			log.Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetConditions(e, condition.ReconcileError(errors.Wrap(err, errReconcileUpdate)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}

		return actionErr
	}

	log.Debug("Successfully requested update of external resource")
	c.recorder.Event(el, event.Normal(reasonUpdated, "Successfully requested update of external resource"))
	return nil
}

func (c *Controller) handleDelete(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(ctrlevent.Delete)).Debug("No event handler registered.")
		return nil
	}

	log := c.logger.WithValues("objectRef", ref.String())

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		log.Debug("Cannot resolve unstructured object", "error", err)
		return err
	}

	resourceCli := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace())

	if !meta.ShouldDelete(el) {
		log.Debug("Managed resource is not marked for deletion")
		return nil
	}

	actionErr := c.externalClient.Delete(ctx, el)
	if actionErr != nil {
		log.Debug("Cannot delete external resource", "error", actionErr)
		c.recorder.Event(el, event.Warning(reasonCannotDelete, actionErr))

		err := unstructuredtools.SetConditions(el, condition.Deleting(), condition.ReconcileError(errors.Wrap(actionErr, errReconcileDelete)))
		if err != nil {
			log.Debug("Cannot set condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, el, tools.UpdateOptions{
			Pluralizer:    c.pluralizer,
			DynamicClient: c.dynamicClient,
		})
		if err != nil {
			log.Debug("Cannot update status", "error", err)
			return err
		}
		return actionErr
	}

	el.SetFinalizers([]string{})
	_, err = resourceCli.Update(context.Background(), el, metav1.UpdateOptions{})
	if err != nil {
		log.Debug("Cannot remove finalizers", "error", err)
		return err
	}

	log.Debug("Successfully requested deletion of external resource")
	c.recorder.Event(el, event.Normal(reasonDeleted, "Successfully requested deletion of external resource"))
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
