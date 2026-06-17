# Building a controller

The steps to stand up a dynamic controller on this library, using the **composition-dynamic-controller (CDC)** and the runnable example as references.

## 1. Build the controller

Give the controller the resource type to watch and the options it needs: a resync interval, a rate limiter, optional label/field selectors (to scope or shard it), and — if you want — a remapping of which change triggers which operation (the CDC remaps update to observe). Client-side throttling is off by default, leaving load to the API server's fairness controls.

## 2. Implement the operations over untyped objects

Implement **Observe**, **Create**, **Update**, **Delete**, following the contract in [`01-mental-model.md`](./01-mental-model.md). Read desired state out of the object's spec, and write conditions and status back through the provided helpers (status lands on the status subresource). There is **no Connect step** — build whatever clients you need inside these operations.

## 3. Register the client and run

Register your implementation with the controller and run it with a chosen number of workers. The CDC does exactly this, and the repository's runnable integration example shows the whole flow end to end against a real cluster with a no-op implementation.

## Testing

Unit tests can drive the controller against a fake dynamic client — no cluster required. The integration example covers the real path on a throwaway cluster.

> One thing to get right: the resource type you watch must line up with how the object's kind resolves to a resource. For kinds with irregular plurals, supply a custom resolver so the controller reads and writes the right resource.
