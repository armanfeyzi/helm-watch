package discovery

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

const helmOwnerSelector = "owner=helm"

type HelmReleaseDiscoverer struct {
	client kubernetes.Interface
}

func NewHelmReleaseDiscoverer(client kubernetes.Interface) *HelmReleaseDiscoverer {
	return &HelmReleaseDiscoverer{client: client}
}

func (d *HelmReleaseDiscoverer) Discover(ctx context.Context) ([]model.WorkloadRecord, error) {
	out := make([]model.WorkloadRecord, 0)

	secrets, err := d.client.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: helmOwnerSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list helm secrets: %w", err)
	}

	for _, s := range secrets.Items {
		out = append(out, model.WorkloadRecord{
			ID:             workloadID(model.SourceTypeHelmReleaseSecret, s.Namespace, s.Name),
			AppName:        resolveReleaseName(s.Labels, s.Name),
			Namespace:      s.Namespace,
			SourceType:     model.SourceTypeHelmReleaseSecret,
			DeploymentType: inferDeploymentTypeFromLabels(s.Labels),
			DetectedAt:     nowUTC(),
		})
	}

	configMaps, err := d.client.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: helmOwnerSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list helm configmaps: %w", err)
	}

	for _, c := range configMaps.Items {
		out = append(out, model.WorkloadRecord{
			ID:             workloadID(model.SourceTypeHelmReleaseCM, c.Namespace, c.Name),
			AppName:        resolveReleaseName(c.Labels, c.Name),
			Namespace:      c.Namespace,
			SourceType:     model.SourceTypeHelmReleaseCM,
			DeploymentType: inferDeploymentTypeFromLabels(c.Labels),
			DetectedAt:     nowUTC(),
		})
	}

	return out, nil
}

func resolveReleaseName(labels map[string]string, fallback string) string {
	if labels == nil {
		return fallback
	}
	if name := labels["name"]; name != "" {
		return name
	}
	return fallback
}

func inferDeploymentTypeFromLabels(labels map[string]string) model.DeploymentType {
	if labels == nil {
		return model.DeploymentTypeHelm
	}

	managedBy := labels["app.kubernetes.io/managed-by"]
	if managedBy == "Terraform" {
		return model.DeploymentTypeTerraform
	}
	return model.DeploymentTypeHelm
}
