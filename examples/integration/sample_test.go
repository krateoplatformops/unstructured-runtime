//go:build integration
// +build integration

package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/krateoplatformops/plumbing/e2e"
	prettylog "github.com/krateoplatformops/plumbing/slogs/pretty"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller"
	"github.com/krateoplatformops/unstructured-runtime/pkg/controller/builder"
	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
	"github.com/krateoplatformops/unstructured-runtime/pkg/pluralizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/support/kind"

	xenv "github.com/krateoplatformops/plumbing/env"
)

// MinimalExternalClient is a no-op ExternalClient for the demo.
type MinimalExternalClient struct {
	observeOk bool
	createOk  bool
	deleteOk  bool
	updateOk  bool
	logger    logging.Logger
}

func (m *MinimalExternalClient) Observe(ctx context.Context, mg *unstructured.Unstructured) (controller.ExternalObservation, error) {
	m.logger.Info("Observe called for", "name", mg.GetName())
	m.observeOk = true
	return controller.ExternalObservation{ResourceExists: false, ResourceUpToDate: false}, nil
}
func (m *MinimalExternalClient) Create(ctx context.Context, mg *unstructured.Unstructured) error {
	m.logger.Info("Create called for", "name", mg.GetName())
	m.createOk = true
	return nil
}
func (m *MinimalExternalClient) Update(ctx context.Context, mg *unstructured.Unstructured) error {
	m.logger.Info("Update called for", "name", mg.GetName())
	m.updateOk = true
	return nil
}
func (m *MinimalExternalClient) Delete(ctx context.Context, mg *unstructured.Unstructured) error {
	m.logger.Info("Delete called for", "name", mg.GetName())
	m.deleteOk = true
	return nil
}

type LocalPluralizer struct {
}

func (p LocalPluralizer) GVKtoGVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind) + "s", // very naive pluralization
	}, nil
}

var _ pluralizer.PluralizerInterface = &LocalPluralizer{}

var (
	testenv     env.Environment
	clusterName string
	namespace   string
)

func TestMain(m *testing.M) {
	xenv.SetTestMode(true)

	namespace = "demo-system"
	clusterName = "examples-basic"
	testenv = env.New()

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), clusterName),
		envfuncs.SetupCRDs("crds", "*.yaml"),

		func(ctx context.Context, c *envconf.Config) (context.Context, error) {
			time.Sleep(10 * time.Second) // wait for the crds to be ready

			return ctx, nil
		},
		e2e.CreateNamespace(namespace),
	).Finish(
		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(clusterName),
	)

	os.Exit(testenv.Run(m))
}

// TestKindWithE2EFramework demonstrates creating a Kind cluster with the e2e-framework
// and using the returned kubeconfig/rest.Config to build a controller via the builder.
//
// Notes:
//   - This is an integration-style test: it creates a real Kind cluster locally.
//   - Ensure 'kind' is installed and available in PATH for the provider to work.
//   - The controller is run only briefly for demonstration purposes. In a real scenario
//     you would typically run it for longer and have a proper ExternalClient implementation.
func TestKindWithE2EFramework(t *testing.T) {
	f := features.New("Setup").
		Setup(e2e.Logger("test")).
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return ctx
		}).
		Assess("Run controller against Kind cluster", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			gvr := schema.GroupVersionResource{Group: "finops.krateo.io", Version: "v1", Resource: "focusconfigs"}

			cfgBuilder := builder.Configuration{
				Config:       cfg.Client().RESTConfig(),
				GVR:          gvr,
				ProviderName: "deployment-provider",
			}

			lh := prettylog.New(&slog.HandlerOptions{
				Level:     slog.LevelDebug,
				AddSource: false,
			},
				prettylog.WithDestinationWriter(os.Stderr),
				prettylog.WithOutputEmptyAttrs(),
			)
			logger := logging.NewLogrLogger(logr.FromSlogHandler(slog.New(lh).Handler()))

			ctr, err := builder.Build(context.Background(), cfgBuilder,
				builder.WithNamespace("default"),
				builder.WithResyncInterval(2*time.Second),
				builder.WithLogger(logger),
				builder.WithPluralizer(LocalPluralizer{}),
			)

			require.NoError(t, err)
			require.NotNil(t, ctr)

			extCli := &MinimalExternalClient{
				logger: logger,
			}
			// register no-op ExternalClient
			ctr.SetExternalClient(extCli)

			// run controller briefly
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			go ctr.Run(ctx, 1)

			// Create a Deployment and ensure the controller picks it up
			dep := &unstructured.Unstructured{}
			dep.SetGroupVersionKind(schema.GroupVersionKind{Group: "finops.krateo.io", Version: "v1", Kind: "FocusConfig"})
			dep.SetNamespace("default")
			dep.SetName("demo-role")

			r, err := resources.New(cfg.Client().RESTConfig())
			require.NoError(t, err)
			r.WithNamespace("default")

			err = r.Create(ctx, dep)
			require.NoError(t, err)

			// wait a bit to let the controller pick up the event
			time.Sleep(5 * time.Second)

			assert.True(t, extCli.observeOk, "ExternalClient Observe should have been called and state set to true")
			assert.True(t, extCli.createOk, "ExternalClient Create should have been called and state set to true")

			err = r.Get(ctx, dep.GetName(), dep.GetNamespace(), dep)
			require.NoError(t, err)
			// Now update the resource to trigger an Update call
			unstructured.SetNestedField(dep.Object, "test", "spec", "focusSpec", "billingAccountId")
			err = r.Update(ctx, dep)
			require.NoError(t, err)

			err = r.Delete(ctx, dep)
			require.NoError(t, err)

			// wait a bit to let the controller pick up the delete event
			time.Sleep(5 * time.Second)
			assert.True(t, extCli.deleteOk, "ExternalClient Delete should have been called and state set to true")

			return nil
		}).
		Feature()

	testenv.Test(t, f)
}
