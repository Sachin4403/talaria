/**
 * Copyright 2024 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package servicecfg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client extends the Kubernetes clientset with behaviors specific to XMiDT service discovery
type Client interface {
	// Clientset returns the underlying Kubernetes clientset
	Clientset() kubernetes.Interface

	// GetEndpoints returns the endpoints for a service in a namespace
	GetEndpoints(ctx context.Context, namespace, service string) (*corev1.Endpoints, error)

	// GetEndpointSlices returns the endpoint slices for a service in a namespace (K8s 1.21+)
	GetEndpointSlices(ctx context.Context, namespace, service string) (*discoveryv1.EndpointSliceList, error)

	// NewSharedInformerFactory creates a new shared informer factory
	NewSharedInformerFactory(namespace string, resyncPeriod metav1.Duration) informers.SharedInformerFactory

	// Close stops the client and releases resources
	Close()
}

// client implements the Client interface
type client struct {
	clientset kubernetes.Interface
	config    *rest.Config
}

// NewClient creates a new Kubernetes client from the given options
func NewClient(cfg ClientConfig) (Client, error) {
	config, err := buildConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// Apply QPS and Burst settings if specified
	if cfg.QPS > 0 {
		config.QPS = cfg.QPS
	}
	if cfg.Burst > 0 {
		config.Burst = cfg.Burst
	}
	if cfg.Timeout > 0 {
		config.Timeout = cfg.Timeout
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &client{
		clientset: clientset,
		config:    config,
	}, nil
}

// buildConfig creates a rest.Config from the client configuration
func buildConfig(cfg ClientConfig) (*rest.Config, error) {
	// If kubeconfig is specified, use it
	if len(cfg.Kubeconfig) > 0 {
		return clientcmd.BuildConfigFromFlags(cfg.MasterURL, cfg.Kubeconfig)
	}

	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to default kubeconfig location
	if home := homeDir(); home != "" {
		kubeconfigPath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfigPath); err == nil {
			return clientcmd.BuildConfigFromFlags(cfg.MasterURL, kubeconfigPath)
		}
	}

	return nil, fmt.Errorf("unable to build kubernetes config: not in cluster and no kubeconfig found")
}

// homeDir returns the user's home directory
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // Windows
}

func (c *client) Clientset() kubernetes.Interface {
	return c.clientset
}

func (c *client) GetEndpoints(ctx context.Context, namespace, service string) (*corev1.Endpoints, error) {
	return c.clientset.CoreV1().Endpoints(namespace).Get(ctx, service, metav1.GetOptions{})
}

func (c *client) GetEndpointSlices(ctx context.Context, namespace, service string) (*discoveryv1.EndpointSliceList, error) {
	labelSelector := fmt.Sprintf("kubernetes.io/service-name=%s", service)
	return c.clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (c *client) NewSharedInformerFactory(namespace string, resyncPeriod metav1.Duration) informers.SharedInformerFactory {
	if namespace == "" {
		return informers.NewSharedInformerFactory(c.clientset, resyncPeriod.Duration)
	}
	return informers.NewSharedInformerFactoryWithOptions(
		c.clientset,
		resyncPeriod.Duration,
		informers.WithNamespace(namespace),
	)
}

func (c *client) Close() {
	// The kubernetes clientset doesn't have a Close method,
	// but we include this for consistency and future-proofing
}

// clientFactory is used for testing to inject mock clients
var clientFactory = func(cfg ClientConfig) (Client, error) {
	return NewClient(cfg)
}
