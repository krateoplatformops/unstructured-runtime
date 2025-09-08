# unstructured-runtime

[![Go Report Card](https://goreportcard.com/badge/github.com/krateoplatformops/unstructured-runtime)](https://goreportcard.com/report/github.com/krateoplatformops/unstructured-runtime)

`unstructured-runtime` is a small, focused controller framework to reconcile
Kubernetes custom resources using dynamic clients and unstructured objects.
It provides a pluggable controller builder, a priority queue with rate
limiting, Prometheus metrics integration, and helpers to implement ExternalClient
adapters for your infrastructure.

## Key features
- Dynamic client-based controllers (no generated typed clients required).
- Priority queue with rate limiting and metrics integration.
- Pluggable `ExternalClient` interface to Observe/Create/Update/Delete external resources.
- Test helpers and coverage for builder and worker logic.
- Minimal, dependency-light primitives suitable for embedding into operators.

## Getting started

Prerequisites:
- Go 1.18+ and modules enabled.
- (Optional) a kubeconfig for running controllers against a cluster.

1. See how a controller is constructed in the builder tests for a minimal example:
   - [pkg/controller/builder/builder_test.go](pkg/controller/builder/builder_test.go)
   - Implementation: [pkg/controller/builder/builder.go](pkg/controller/builder/builder.go)
2. The builder creates a controller with the core entrypoint [`controller.New`](pkg/controller/controller.go).
   Review [`controller.New`](pkg/controller/controller.go) for how informers, queue and handlers are wired.
   The main controller types and the `ExternalClient` interface are in:
   - [`controller.New`](pkg/controller/controller.go)
   - [`controller.ExternalClient`](pkg/controller/controller.go)
3. Tests in this repo demonstrate typical usage patterns and fake clients to run unit tests without a real cluster.

## Basic example

Below is a minimal example that shows two ways to run a controller:
- "production" style using the builder (`pkg/controller/builder`) and a real kubeconfig.
- "test" style using a fake dynamic client (similar to the tests under [`pkg/controller`]).

See the full runnable snippet:
- integration-style test with local kind cluster: [examples/integration/sample_test.go](examples/integration/sample_test.go)

Useful internal modules for building ExternalClient implementations:
- Conditions & Status helpers: [`pkg/tools/unstructured/unstructured.go`](pkg/tools/unstructured/unstructured.go)
- Metadata/annotation helpers: [`pkg/meta/meta.go`](pkg/meta/meta.go)
- Pluralizer utilities (GVK <-> GVR): [pkg/pluralizer](pkg/pluralizer)
- Short id generator (deterministic ids for tests): [`pkg/shortid/shortid.go`](pkg/shortid/shortid.go)
- Workqueue metrics provider & integrations: [`pkg/workqueue/metrics/workqueue.go`](pkg/workqueue/metrics/workqueue.go)

## Typical workflow
- Implement an `ExternalClient` that satisfies the [`controller.ExternalClient`](pkg/controller/controller.go) interface and handles Observe/Create/Update/Delete.
- Use the builder to construct a controller for a given GroupVersionResource (GVR).
- Start the controller with a context and a desired number of workers.

## Build & test

To build the repository:

```sh
go build ./...
```
