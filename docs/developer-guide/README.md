# unstructured-runtime — Developer Guide

A contributor-facing guide to the library that gives Krateo's **dynamic** controllers a managed-resource loop over untyped objects: implement a small set of operations, and the framework reconciles any resource type.

> Audience: engineers **building a controller on top of this library, or maintaining the library itself**. This guide explains *ideas and flows*, not line-by-line code. For product concepts, see [docs.krateo.io](https://docs.krateo.io).

## What it is

`unstructured-runtime` is a controller framework for reconciling Kubernetes resources **dynamically** — using untyped objects rather than generated, compile-time types. A controller author targets a resource type chosen at runtime and implements a handful of operations; the framework owns everything else: the watch over that resource type, a de-duplicating priority work queue, a worker pool, finalizers, status and conditions, event recording, retry and rate-limiting, and metrics.

It is **"unstructured"** because the consuming controller doesn't know the resource's type at compile time. The primary consumer, the **composition-dynamic-controller (CDC)**, is told its resource type at startup. Unlike `provider-runtime`, this library does not build on a controller-runtime manager; it wires the lower-level pieces directly. See [`01-mental-model.md`](./01-mental-model.md).

> **Sibling library.** `provider-runtime` is the **typed analog** of this library — the same lifecycle applied to a compile-time-typed resource. The two are meant to stay functionally equivalent; the mapping is in [`04-equivalence.md`](./04-equivalence.md).

## Documents in this folder

| Document | What it covers |
| --- | --- |
| [`01-mental-model.md`](./01-mental-model.md) | The managed-resource lifecycle and the operations you implement (and the headline difference from provider-runtime: no Connect step). |
| [`02-architecture.md`](./02-architecture.md) | What the library provides, how a reconcile flows, and the truth about worker scaling. |
| [`03-building-a-controller.md`](./03-building-a-controller.md) | The steps to stand up a controller, using the CDC and the runnable example as references. |
| [`04-equivalence.md`](./04-equivalence.md) | How `provider-runtime` and `unstructured-runtime` line up, concept by concept. |

## See also

- **Ecosystem overview (canonical)** — how Krateo Composable Operations fits together lives in the **core-provider** repo: `core-provider/docs/developer-guide/00-ecosystem-overview.md`.
- **The exemplar consumer** — the **composition-dynamic-controller** is the reference controller built on this library.
- **Runnable example** — the integration example in the repo is the canonical getting-started: build the controller, register a client, run it against a real cluster.
