# Architecture

What the library provides, how a reconcile flows, and the truth about worker scaling.

## What's in the box

- **The controller** — the watch over a resource type, a priority work queue, a pool of workers, and the dispatch that routes each event to one of your operations.
- **Event handling** — the mapping of "what changed" to "which operation runs" is configurable (the CDC, for example, treats an update as an observe).
- **The priority queue** — de-duplicating, priority-aware, and rate-limited, so bursts are fair and retries back off.
- **Pluggable-logic helpers** — for reading desired state out of an untyped object and for writing conditions and status back.
- **Runtime type resolution** — turning a kind into the resource type to act on, since nothing is known at compile time.
- **Cross-cutting** — logging, optional telemetry, and an optional metrics server.

## How a reconcile flows

```mermaid
flowchart TB
    W[watch over the resource type] -->|enqueue| Q[priority queue]
    Q --> WK[worker]
    WK -->|fetch the live object| K8s[(Kubernetes API)]
    WK --> CH{framework checks: pause, finalizer, create-safety}
    CH --> D{dispatch}
    D -->|observe| OBS[Observe]
    D -->|create / update / delete| CUD[Create / Update / Delete]
    OBS -.->|self-enqueue create or update if needed| Q
```

A worker pulls an event, fetches the live object, runs the framework checks (skip if paused; drop the finalizer if it's an orphan being deleted; refuse if a create is unconfirmed; add the finalizer for live objects), then dispatches to one of your operations. `Observe` doesn't create or update directly — it reports its findings, and the framework enqueues the appropriate follow-up.

## Worker scaling: the reality

The number of workers is a **fixed value passed at startup — there is no autoscaling.** The repository includes a design note on the subject, but it is **advisory**, not an implemented feature: its conclusion is that adding workers in a single instance mostly causes contention, and that the right way to scale is to **shard** — run several controller instances, each handling a slice of the resources via a label selector — and to **rate-limit** against slow external APIs. The priority queue is the fairness mechanism within an instance. So the levers are: the worker count, the priority queue, the rate limiter, and operationally, sharding. Don't go looking for an autoscaler — there isn't one.

## Two things to keep in mind

- **Status goes to the status subresource.** Conditions and status are written there, so the watched resource must define a status subresource.
- **Keep "up to date" stable.** The controller writes status, finalizers, and annotations as part of normal operation; if `Observe` reports "out of date" on inputs that didn't really change, you'll get needless churn. (Updates that only change the object's version, not its spec, are already ignored to avoid self-trigger loops.)

## Management and deletion policies

Two annotations on the watched object let an operator narrow what the controller is allowed to do. They're set by whoever applies the object, but they directly decide **which of your operations the controller actually calls** — so it matters when you're building a controller (don't assume your `Create` runs just because the object is missing).

**Management policy** — the annotation `krateo.io/management-policy` — gates the allowed actions:

| Value | Observe | Create | Update | Delete |
| --- | :---: | :---: | :---: | :---: |
| `default` (when the annotation is absent) | ✓ | ✓ | ✓ | ✓ |
| `observe-create-update` | ✓ | ✓ | ✓ | — |
| `observe-delete` | ✓ | — | — | ✓ |
| `observe` | ✓ | — | — | — |

`default` is full management. `observe` is the read-only case — the object is owned by something else and the controller only observes it. Under any non-`default` value, the disallowed operations are simply never invoked, even if `Observe` reports the object as missing or drifted.

**Deletion policy** — the annotation `krateo.io/deletion-policy` — decides what happens to the *external* resource when the watched object is deleted:

- `delete` (the default when absent) — the external resource is deleted too.
- `orphan` — the external resource is left in place.

**How they combine on delete.** When the object is being deleted, the controller deletes the external resource only when the management policy permits it **and** the deletion policy asks for it — concretely, when management is `default` and deletion is `delete` (the default), or when management is `observe-delete`. In every other case the external resource is orphaned. One subtlety to keep in mind: `observe-delete` deletes regardless of the deletion policy.
