
# unstructured-runtime

[![Go Report Card](https://goreportcard.com/badge/github.com/krateoplatformops/unstructured-runtime)](https://goreportcard.com/report/github.com/krateoplatformops/unstructured-runtime)

`unstructured-runtime` is a lightweight controller framework to reconcile
Kubernetes custom resources using dynamic clients and unstructured objects.
It provides a pluggable controller builder, a priority queue with rate
limiting, and helpers to implement ExternalClient adapters for your
infrastructure.

## Key features
- Dynamic client-based controllers (no generated typed clients required).
- Priority queue with rate limiting and metrics integration.
- Pluggable ExternalClient interface to observe/create/update/delete external resources.
- Test helpers and coverage for builder and worker logic.

## Getting started
1. See how a controller is constructed in the builder tests for a minimal example:
   - [pkg/controller/builder/builder_test.go](pkg/controller/builder/builder_test.go)
   - The builder implementation: [pkg/controller/builder/builder.go](pkg/controller/builder/builder.go)
2. The builder creates a controller with the core entrypoint [`controller.New`](pkg/controller/controller.go).
   Review [`controller.New`](pkg/controller/controller.go) for how informers, queue and handlers are wired.
3. Tests in this repo demonstrate typical usage patterns and fake clients to run unit tests without a real cluster.

## Typical workflow
- Implement an ExternalClient that satisfies the `ExternalClient` interface and handles Observe/Create/Update/Delete.
- Use the builder to construct a controller for a given GroupVersionResource (GVR).
- Start the controller with a context and desired number of workers.

## Testing
- Unit tests are available under pkg/controller and show examples of how to build and run controllers in tests.
- Run the test suite with:
  ```sh
  go test ./... -v