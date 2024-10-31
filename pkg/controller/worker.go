package controller

import (
	"context"
	"fmt"

	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/objectref"
	eventrec "github.com/krateoplatformops/unstructured-runtime/pkg/event"
	"github.com/krateoplatformops/unstructured-runtime/pkg/meta"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools"
	unstructuredtools "github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured"
	"github.com/krateoplatformops/unstructured-runtime/pkg/tools/unstructured/condition"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/util/runtime"
)

const (
	maxRetries    = 5
	finalizerName = "composition.krateo.io/finalizer"
)

func (c *Controller) runWorker(ctx context.Context) {
	for {
		obj, shutdown := c.queue.Get()
		if shutdown {
			break
		}
		dig := event.DigestForEvent(obj)

		c.items.Delete(dig)
		c.queue.Forget(obj)
		c.queue.Done(obj)

		err := c.processItem(ctx, obj)
		c.handleErr(err, obj)
	}
}

func (c *Controller) handleErr(err error, obj event.Event) {
	if err == nil {
		return
	}

	if retries := c.queue.NumRequeues(obj); retries < maxRetries {
		c.logger.WithValues("retries", retries).
			WithValues("obj", fmt.Sprintf("%v", obj)).
			Debug("processing event, retrying", "error", err)

		dig := event.DigestForEvent(obj)

		if _, loaded := c.items.LoadOrStore(dig, struct{}{}); !loaded {
			c.queue.Add(obj)
		}
		return
	}

	c.logger.Debug("error processing event (max retries reached)", "error", err)
	runtime.HandleError(err)
}

func (c *Controller) processItem(ctx context.Context, obj interface{}) error {
	evt, ok := obj.(event.Event)
	if !ok {
		c.logger.Debug("unexpected event", "object", obj)
		return nil
	}

	el, err := c.fetch(ctx, evt.ObjectRef, false)
	if err != nil {
		c.logger.Debug("Resolving unstructured object", "error", evt.ObjectRef.String())
		return nil
	}

	if el.GetDeletionTimestamp().IsZero() {
		finalizers := el.GetFinalizers()
		exist := false
		for _, finalizer := range finalizers {
			if finalizer == finalizerName {
				exist = true
				break
			}
		}

		if !exist {
			el.SetFinalizers(append(finalizers, finalizerName))
			_, err = c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace()).Update(context.Background(), el, metav1.UpdateOptions{})
			if err != nil {
				c.logger.Debug("UpdateFunc: adding finalizer", "error", err)
				return err
			}
		}
	} else {
		c.handleDeleteEvent(ctx, evt.ObjectRef)
	}

	c.logger.Debug("processing", "objectRef", evt.ObjectRef.String())
	switch evt.EventType {
	case event.Create:
		return c.handleCreate(ctx, evt.ObjectRef)
	case event.Update:
		return c.handleUpdateEvent(ctx, evt.ObjectRef)
	case event.Delete:
		return c.handleDeleteEvent(ctx, evt.ObjectRef)
	default:
		return c.handleObserve(ctx, evt.ObjectRef)
	}
}

func (c *Controller) handleObserve(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(event.Observe)).Debug("No event handler registered.")
		return nil
	}

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
		return err
	}

	if meta.IsPaused(el) {
		c.logger.Debug("Reconciliation is paused via the pause annotation.", "annotation", meta.AnnotationKeyReconciliationPaused, "value", "true")
		c.recorder.Event(el, eventrec.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetCondition(el, condition.ReconcilePaused())
		if err != nil {
			c.logger.Debug("UpdateFunc: setting condition", "error", err)
			return err
		}

		_, err := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace()).UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			c.logger.Debug("UpdateFunc: updating status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	exists, actionErr := c.externalClient.Observe(ctx, el)
	if actionErr != nil {
		if apierrors.IsNotFound(actionErr) {
			return c.externalClient.Update(ctx, el)
		}

		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetCondition(e, condition.FailWithReason(fmt.Sprintf("failed to observe object: %s", actionErr)))
		if err != nil {
			c.logger.Debug("Observe: setting condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			DiscoveryClient: c.discoveryClient,
			DynamicClient:   c.dynamicClient,
		})
		if err != nil {
			c.logger.Debug("Observe: updating status", "error", err)
			return err
		}
	}
	if !exists {
		return c.externalClient.Create(ctx, el)
	}

	return nil
}

func (c *Controller) handleCreate(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(event.Create)).Debug("No event handler registered.")
		return nil
	}

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
		return err
	}

	if meta.IsPaused(el) {
		c.logger.WithValues("objectRef", ref.String()).Debug("Reconciliation is paused via the pause annotation")
		c.recorder.Event(el, eventrec.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetCondition(el, condition.ReconcilePaused())
		if err != nil {
			c.logger.Debug("CreateFunc: setting condition", "error", err)
			return err
		}

		_, err := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace()).UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			c.logger.Debug("CreateFunc: updating status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	actionErr := c.externalClient.Create(ctx, el)
	if actionErr != nil {
		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetCondition(e, condition.FailWithReason(fmt.Sprintf("failed to create object: %s", actionErr)))
		if err != nil {
			c.logger.Debug("UpdateFunc: setting condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			DiscoveryClient: c.discoveryClient,
			DynamicClient:   c.dynamicClient,
		})
		if err != nil {
			// c.logger.Error().Err(err).Msg("UpdateFunc: updating status.")
			c.logger.Debug("UpdateFunc: updating status", "error", err)
			return err
		}

	}

	return actionErr
}

func (c *Controller) handleUpdateEvent(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(event.Update)).Debug("No event handler registered.")
		return nil
	}

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
		return err
	}

	if meta.IsPaused(el) {
		c.logger.WithValues("annotation", meta.AnnotationKeyReconciliationPaused).Debug("Reconciliation is paused via the pause annotation")
		c.recorder.Event(el, eventrec.Normal(reasonReconciliationPaused, "Reconciliation is paused via the pause annotation"))
		err = unstructuredtools.SetCondition(el, condition.ReconcilePaused())
		if err != nil {
			c.logger.Debug("UpdateFunc: setting condition", "error", err)
			return err
		}

		_, err := c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace()).UpdateStatus(context.Background(), el, metav1.UpdateOptions{})
		if err != nil {
			c.logger.Debug("UpdateFunc: updating status", "error", err)
			return err
		}
		// if the pause annotation is removed, we will have a chance to reconcile again and resume
		// and if status update fails, we will reconcile again to retry to update the status
		return nil
	}

	actionErr := c.externalClient.Update(ctx, el)

	if actionErr != nil {
		e, err := c.fetch(ctx, ref, false)
		if err != nil {
			c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
			return err
		}

		err = unstructuredtools.SetCondition(e, condition.FailWithReason(fmt.Sprintf("failed to update object: %s", actionErr)))
		if err != nil {
			c.logger.Debug("UpdateFunc: setting condition", "error", err)
			return err
		}

		_, err = tools.UpdateStatus(ctx, e, tools.UpdateOptions{
			DiscoveryClient: c.discoveryClient,
			DynamicClient:   c.dynamicClient,
		})
		if err != nil {
			c.logger.Debug("UpdateFunc: updating status", "error", err)
			return err
		}

	}

	return actionErr
}

func (c *Controller) handleDeleteEvent(ctx context.Context, ref objectref.ObjectRef) error {
	if c.externalClient == nil {
		c.logger.WithValues("eventType", string(event.Delete)).Debug("No event handler registered.")
		return nil
	}

	el, err := c.fetch(ctx, ref, false)
	if err != nil {
		c.logger.WithValues("objectRef", ref.String()).Debug("Resolving unstructured object")
		return err
	}

	err = c.externalClient.Delete(ctx, el)
	if err != nil {
		c.logger.Debug("DeleteFunc: deleting object", "error", err)
		return err
	}

	el.SetFinalizers([]string{})
	_, err = c.dynamicClient.Resource(c.gvr).Namespace(el.GetNamespace()).Update(context.Background(), el, metav1.UpdateOptions{})
	if err != nil {
		c.logger.Debug("DeleteFunc: removing finalizer", "error", err)
		return err
	}
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
