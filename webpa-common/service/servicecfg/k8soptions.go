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
	"time"
)

const (
	// DefaultNamespace is the default Kubernetes namespace to watch
	DefaultNamespace = "default"

	// DefaultScheme is the default URI scheme for discovered instances
	DefaultScheme = "http"

	// DefaultResyncPeriod is the default period for resyncing the informer cache
	DefaultResyncPeriod = 30 * time.Second

	// DefaultPort is the default port if not specified
	DefaultPort = 8080
)

// Watch represents a Kubernetes service to watch for endpoint changes
type Watch struct {
	// Service is the name of the Kubernetes Service to watch
	Service string `json:"service"`

	// Namespace is the Kubernetes namespace where the service resides.
	// If empty, DefaultNamespace is used.
	Namespace string `json:"namespace,omitempty"`

	// LabelSelector is an optional label selector to filter endpoints.
	// Example: "app=talaria,environment=prod"
	LabelSelector string `json:"labelSelector,omitempty"`

	// PortName is the named port to use from the endpoint.
	// If empty, the first port is used.
	PortName string `json:"portName,omitempty"`

	// Scheme is the URI scheme for the discovered instances (http or https).
	// If empty, DefaultScheme is used.
	Scheme string `json:"scheme,omitempty"`

	// AllNamespaces indicates whether to watch all namespaces.
	// If true, Namespace is ignored.
	AllNamespaces bool `json:"allNamespaces,omitempty"`
}

func (w Watch) namespace() string {
	if w.AllNamespaces {
		return ""
	}
	if len(w.Namespace) > 0 {
		return w.Namespace
	}
	return DefaultNamespace
}

func (w Watch) scheme() string {
	if len(w.Scheme) > 0 {
		return w.Scheme
	}
	return DefaultScheme
}

// Registration represents how this service should register itself in Kubernetes.
// Note: In Kubernetes, services are typically registered via Pod labels and Service selectors.
// This is provided for cases where manual endpoint management is needed.
type Registration struct {
	// Service is the Kubernetes Service name to register under
	Service string `json:"service"`

	// Namespace is the Kubernetes namespace for the registration
	Namespace string `json:"namespace,omitempty"`

	// Address is the IP address or hostname to register
	Address string `json:"address"`

	// Port is the port number to register
	Port int `json:"port,omitempty"`

	// Scheme is the URI scheme (http/https)
	Scheme string `json:"scheme,omitempty"`
}

func (r Registration) namespace() string {
	if len(r.Namespace) > 0 {
		return r.Namespace
	}
	return DefaultNamespace
}

func (r Registration) port() int {
	if r.Port > 0 {
		return r.Port
	}
	return DefaultPort
}

func (r Registration) scheme() string {
	if len(r.Scheme) > 0 {
		return r.Scheme
	}
	return DefaultScheme
}

// ClientConfig holds Kubernetes client configuration options
type ClientConfig struct {
	// Kubeconfig is the path to the kubeconfig file.
	// If empty, in-cluster configuration is used.
	Kubeconfig string `json:"kubeconfig,omitempty"`

	// MasterURL is the URL of the Kubernetes API server.
	// If empty, it's determined from kubeconfig or in-cluster config.
	MasterURL string `json:"masterURL,omitempty"`

	// QPS is the maximum queries per second to the API server.
	// If zero, a reasonable default is used.
	QPS float32 `json:"qps,omitempty"`

	// Burst is the maximum burst for throttling.
	// If zero, a reasonable default is used.
	Burst int `json:"burst,omitempty"`

	// Timeout is the timeout for API requests.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// K8sOptions represents the complete set of configurable attributes for Kubernetes service discovery
type K8sOptions struct {
	// Client holds the Kubernetes client options
	Client ClientConfig `json:"client"`

	// Registrations are the ways in which the host process should be registered.
	// In Kubernetes, this is typically handled by Pod labels, but manual registration is supported.
	Registrations []Registration `json:"registrations,omitempty"`

	// Watches are the Kubernetes services to watch for endpoint updates.
	Watches []Watch `json:"watches,omitempty"`

	// ResyncPeriod is the period for resyncing the informer cache with the API server.
	// If zero, DefaultResyncPeriod is used.
	ResyncPeriod time.Duration `json:"resyncPeriod,omitempty"`
}

func (o *K8sOptions) resyncPeriod() time.Duration {
	if o != nil && o.ResyncPeriod > 0 {
		return o.ResyncPeriod
	}
	return DefaultResyncPeriod
}

func (o *K8sOptions) registrations() []Registration {
	if o != nil && len(o.Registrations) > 0 {
		return o.Registrations
	}
	return nil
}

func (o *K8sOptions) watches() []Watch {
	if o != nil && len(o.Watches) > 0 {
		return o.Watches
	}
	return nil
}
