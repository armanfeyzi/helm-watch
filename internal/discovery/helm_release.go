package discovery

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/afeyzirealyticsio/helm-watch/internal/model"
)

const helmOwnerSelector = "owner=helm"
const helmDeployedSelector = "owner=helm,status=deployed"

type HelmReleaseDiscoverer struct {
	client kubernetes.Interface
}

func NewHelmReleaseDiscoverer(client kubernetes.Interface) *HelmReleaseDiscoverer {
	return &HelmReleaseDiscoverer{client: client}
}

func (d *HelmReleaseDiscoverer) Discover(ctx context.Context) ([]model.WorkloadRecord, error) {
	if deployed, err := d.discoverWithSelector(ctx, helmDeployedSelector); err == nil && len(deployed) > 0 {
		return deployed, nil
	}

	// Fallback: if deployed filtering is unavailable, use owner-only and keep latest revision per release.
	return d.discoverWithSelector(ctx, helmOwnerSelector)
}

func (d *HelmReleaseDiscoverer) discoverWithSelector(ctx context.Context, selector string) ([]model.WorkloadRecord, error) {
	out := make([]model.WorkloadRecord, 0)
	seenRelease := make(map[string]revisionCandidate)

	secrets, err := d.client.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list helm secrets: %w", err)
	}

	for _, s := range secrets.Items {
		rec := model.WorkloadRecord{
			ID:              workloadID(model.SourceTypeHelmReleaseSecret, s.Namespace, s.Name),
			AppName:         resolveReleaseName(s.Labels, s.Name),
			Namespace:       s.Namespace,
			SourceNamespace: s.Namespace,
			SourceType:      model.SourceTypeHelmReleaseSecret,
			DeploymentType:  inferDeploymentTypeFromLabels(s.Labels),
			DetectedAt:      nowUTC(),
		}
		key := releaseKey(rec.Namespace, rec.AppName)
		considerLatest(seenRelease, key, rec, parseRevision(s.Labels))
	}

	configMaps, err := d.client.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list helm configmaps: %w", err)
	}

	for _, c := range configMaps.Items {
		rec := model.WorkloadRecord{
			ID:              workloadID(model.SourceTypeHelmReleaseCM, c.Namespace, c.Name),
			AppName:         resolveReleaseName(c.Labels, c.Name),
			Namespace:       c.Namespace,
			SourceNamespace: c.Namespace,
			SourceType:      model.SourceTypeHelmReleaseCM,
			DeploymentType:  inferDeploymentTypeFromLabels(c.Labels),
			DetectedAt:      nowUTC(),
		}
		key := releaseKey(rec.Namespace, rec.AppName)
		considerLatest(seenRelease, key, rec, parseRevision(c.Labels))
	}

	for _, candidate := range seenRelease {
		out = append(out, candidate.record)
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

type revisionCandidate struct {
	record   model.WorkloadRecord
	revision int
}

func parseRevision(labels map[string]string) int {
	if labels == nil {
		return -1
	}
	v := labels["version"]
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

func releaseKey(namespace, name string) string {
	return namespace + "/" + name
}

func considerLatest(store map[string]revisionCandidate, key string, rec model.WorkloadRecord, revision int) {
	current, ok := store[key]
	if !ok || revision > current.revision {
		store[key] = revisionCandidate{record: rec, revision: revision}
	}
}
