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
	"sync"
	"time"

	"github.com/go-kit/kit/sd"
	"github.com/xmidt-org/webpa-common/v2/adapter"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultRegistrationTimeout is the default timeout for registration operations
	defaultRegistrationTimeout = 10 * time.Second
)

// RegistrarOptions holds the configuration for creating a new Kubernetes registrar
type RegistrarOptions struct {
	// Client is the Kubernetes client
	Client Client

	// Logger is the logger to use
	Logger *zap.Logger

	// Registration contains the service registration configuration
	Registration Registration

	// Timeout is the timeout for registration operations
	Timeout time.Duration
}

// registrar implements sd.Registrar for Kubernetes
// Note: In Kubernetes, service registration is typically handled automatically via
// Pod labels and Service selectors. This registrar is provided for cases where
// manual endpoint management is needed (e.g., external services, testing).
type registrar struct {
	client       Client
	logger       *zap.Logger
	registration Registration
	timeout      time.Duration

	lock       sync.Mutex
	registered bool
}

// NewRegistrar creates a new Kubernetes registrar
func NewRegistrar(o RegistrarOptions) sd.Registrar {
	if o.Logger == nil {
		o.Logger = adapter.DefaultLogger().Logger
	}

	timeout := o.Timeout
	if timeout == 0 {
		timeout = defaultRegistrationTimeout
	}

	return &registrar{
		client: o.Client,
		logger: o.Logger.With(
			zap.String("service", o.Registration.Service),
			zap.String("namespace", o.Registration.namespace()),
			zap.String("address", o.Registration.Address),
			zap.Int("port", o.Registration.port()),
		),
		registration: o.Registration,
		timeout:      timeout,
	}
}

// Register registers this service instance with Kubernetes.
// It creates or updates an Endpoints object with this instance's address.
func (r *registrar) Register() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.registered {
		r.logger.Debug("already registered")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	namespace := r.registration.namespace()
	serviceName := r.registration.Service

	r.logger.Info("registering service instance")

	// Get or create the endpoints object
	endpoints, err := r.client.GetEndpoints(ctx, namespace, serviceName)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new endpoints
			endpoints = &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: r.registration.Address,
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Port:     int32(r.registration.port()),
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}

			_, err = r.client.Clientset().CoreV1().Endpoints(namespace).Create(ctx, endpoints, metav1.CreateOptions{})
			if err != nil {
				r.logger.Error("failed to create endpoints", zap.Error(err))
				return
			}

			r.logger.Info("created endpoints successfully")
			r.registered = true
			return
		}

		r.logger.Error("failed to get endpoints", zap.Error(err))
		return
	}

	// Update existing endpoints - add our address if not present
	updated := r.addAddressToEndpoints(endpoints)
	if !updated {
		r.logger.Debug("address already present in endpoints")
		r.registered = true
		return
	}

	_, err = r.client.Clientset().CoreV1().Endpoints(namespace).Update(ctx, endpoints, metav1.UpdateOptions{})
	if err != nil {
		r.logger.Error("failed to update endpoints", zap.Error(err))
		return
	}

	r.logger.Info("updated endpoints successfully")
	r.registered = true
}

// Deregister removes this service instance from Kubernetes endpoints
func (r *registrar) Deregister() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.registered {
		r.logger.Debug("not registered, skipping deregister")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	namespace := r.registration.namespace()
	serviceName := r.registration.Service

	r.logger.Info("deregistering service instance")

	endpoints, err := r.client.GetEndpoints(ctx, namespace, serviceName)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("endpoints not found, nothing to deregister")
			r.registered = false
			return
		}
		r.logger.Error("failed to get endpoints for deregistration", zap.Error(err))
		return
	}

	// Remove our address from the endpoints
	updated := r.removeAddressFromEndpoints(endpoints)
	if !updated {
		r.logger.Debug("address not found in endpoints")
		r.registered = false
		return
	}

	// If no addresses remain, delete the endpoints
	if len(endpoints.Subsets) == 0 || (len(endpoints.Subsets) == 1 && len(endpoints.Subsets[0].Addresses) == 0) {
		err = r.client.Clientset().CoreV1().Endpoints(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			r.logger.Error("failed to delete empty endpoints", zap.Error(err))
			return
		}
		r.logger.Info("deleted empty endpoints")
	} else {
		_, err = r.client.Clientset().CoreV1().Endpoints(namespace).Update(ctx, endpoints, metav1.UpdateOptions{})
		if err != nil {
			r.logger.Error("failed to update endpoints for deregistration", zap.Error(err))
			return
		}
		r.logger.Info("removed address from endpoints")
	}

	r.registered = false
}

// addAddressToEndpoints adds this registrar's address to the endpoints object
// Returns true if the endpoints were modified, false if the address was already present
func (r *registrar) addAddressToEndpoints(endpoints *corev1.Endpoints) bool {
	targetAddr := r.registration.Address
	targetPort := int32(r.registration.port())

	// Check if address already exists
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.IP == targetAddr {
				// Check if port matches
				for _, port := range subset.Ports {
					if port.Port == targetPort {
						return false // Already present
					}
				}
			}
		}
	}

	// Add to first subset or create new one
	if len(endpoints.Subsets) == 0 {
		endpoints.Subsets = []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{{IP: targetAddr}},
				Ports:     []corev1.EndpointPort{{Port: targetPort, Protocol: corev1.ProtocolTCP}},
			},
		}
	} else {
		// Find a subset with matching port or add to first subset
		added := false
		for i := range endpoints.Subsets {
			for _, port := range endpoints.Subsets[i].Ports {
				if port.Port == targetPort {
					endpoints.Subsets[i].Addresses = append(
						endpoints.Subsets[i].Addresses,
						corev1.EndpointAddress{IP: targetAddr},
					)
					added = true
					break
				}
			}
			if added {
				break
			}
		}

		if !added {
			// Add new subset with our address and port
			endpoints.Subsets = append(endpoints.Subsets, corev1.EndpointSubset{
				Addresses: []corev1.EndpointAddress{{IP: targetAddr}},
				Ports:     []corev1.EndpointPort{{Port: targetPort, Protocol: corev1.ProtocolTCP}},
			})
		}
	}

	return true
}

// removeAddressFromEndpoints removes this registrar's address from the endpoints object
// Returns true if the endpoints were modified, false if the address was not found
func (r *registrar) removeAddressFromEndpoints(endpoints *corev1.Endpoints) bool {
	targetAddr := r.registration.Address
	modified := false

	for i := range endpoints.Subsets {
		var newAddresses []corev1.EndpointAddress
		for _, addr := range endpoints.Subsets[i].Addresses {
			if addr.IP != targetAddr {
				newAddresses = append(newAddresses, addr)
			} else {
				modified = true
			}
		}
		endpoints.Subsets[i].Addresses = newAddresses
	}

	// Clean up empty subsets
	var newSubsets []corev1.EndpointSubset
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			newSubsets = append(newSubsets, subset)
		}
	}
	endpoints.Subsets = newSubsets

	return modified
}
