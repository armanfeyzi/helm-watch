package discovery

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

var argoCDApplicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

type ArgoCDApplicationDiscoverer struct {
	client dynamic.Interface
}

func NewArgoCDApplicationDiscoverer(client dynamic.Interface) *ArgoCDApplicationDiscoverer {
	return &ArgoCDApplicationDiscoverer{client: client}
}

func (d *ArgoCDApplicationDiscoverer) Discover(ctx context.Context) ([]model.WorkloadRecord, error) {
	apps, err := d.client.Resource(argoCDApplicationGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list argocd applications: %w", err)
	}

	out := make([]model.WorkloadRecord, 0, len(apps.Items))
	for _, app := range apps.Items {
		if !applicationUsesHelm(app) {
			continue
		}

		name := app.GetName()
		appNamespace := app.GetNamespace()
		destinationNamespace := applicationDestinationNamespace(app)
		out = append(out, model.WorkloadRecord{
			// Keep the ID namespace as the Application CR namespace so the ID
			// stays stable per Argo CD Application object.
			ID:      workloadID(model.SourceTypeArgoCDApplication, appNamespace, name),
			AppName: name,
			// Namespace is the workload destination namespace (used in metrics).
			Namespace: destinationNamespace,
			// SourceNamespace is where the Application CR lives (used internally
			// to re-fetch it during enrichment).
			SourceNamespace: appNamespace,
			SourceType:      model.SourceTypeArgoCDApplication,
			DeploymentType:  model.DeploymentTypeArgoCD,
			DetectedAt:      nowUTC(),
		})
	}
	return out, nil
}

func applicationUsesHelm(app unstructured.Unstructured) bool {
	spec, ok, err := unstructured.NestedMap(app.Object, "spec")
	if !ok || err != nil {
		return false
	}

	if source, ok := spec["source"].(map[string]any); ok && sourceUsesHelm(source) {
		return true
	}

	if sources, ok := spec["sources"].([]any); ok {
		for _, rawSource := range sources {
			sourceMap, ok := rawSource.(map[string]any)
			if !ok {
				continue
			}
			if sourceUsesHelm(sourceMap) {
				return true
			}
		}
	}

	return false
}

func sourceUsesHelm(source map[string]any) bool {
	// Argo CD Helm source generally contains either `chart` or `helm` fields.
	if _, ok := source["chart"].(string); ok {
		return true
	}
	if _, ok := source["helm"].(map[string]any); ok {
		return true
	}
	return false
}

func applicationDestinationNamespace(app unstructured.Unstructured) string {
	// Prefer target namespace where workloads are deployed.
	if ns, ok, err := unstructured.NestedString(app.Object, "spec", "destination", "namespace"); err == nil && ok && ns != "" {
		return ns
	}
	// Fallback to Application CR namespace if destination namespace is omitted.
	return app.GetNamespace()
}
