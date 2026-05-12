package kube

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Clients struct {
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
}

func NewClients(kubeconfigPath string, qps float32, burst int) (*Clients, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
	}

	if qps > 0 {
		cfg.QPS = qps
	}
	if burst > 0 {
		cfg.Burst = burst
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &Clients{
		Kubernetes: kubeClient,
		Dynamic:    dynClient,
	}, nil
}
