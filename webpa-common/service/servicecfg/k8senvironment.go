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
	"fmt"

	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/webpa-common/v2/service"
	"go.uber.org/zap"
)

// Environment is a Kubernetes-specific interface for the service discovery environment.
// A primary use case is obtaining access to the underlying Kubernetes client for use
// in direct API calls.
type Environment interface {
	service.Environment

	// Client returns the Kubernetes Client interface exposed by this package
	Client() Client
}

type environment struct {
	service.Environment
	client Client
}

func (e environment) Client() Client {
	return e.client
}

// newInstancerKey creates a unique key for an instancer based on the watch configuration
func newInstancerKey(w Watch) string {
	namespace := w.namespace()
	if w.AllNamespaces {
		namespace = "all"
	}
	return fmt.Sprintf("k8s:%s/%s{port=%s}", namespace, w.Service, w.PortName)
}

// newInstancers creates instancers for all configured watches
func newInstancers(l *zap.Logger, c Client, o K8sOptions) (service.Instancers, error) {
	var i service.Instancers

	for _, w := range o.watches() {
		key := newInstancerKey(w)
		if i.Has(key) {
			l.Warn("skipping duplicate watch",
				zap.String("service", w.Service),
				zap.String("namespace", w.namespace()),
				zap.String("portName", w.PortName),
			)
			continue
		}

		inst := service.NewContextualInstancer(
			NewInstancer(InstancerOptions{
				Client:       c,
				Logger:       l,
				Watch:        w,
				ResyncPeriod: o.resyncPeriod(),
			}),
			map[string]interface{}{
				"service":   w.Service,
				"namespace": w.namespace(),
				"portName":  w.PortName,
				"scheme":    w.scheme(),
			},
		)
		i.Set(key, inst)
	}

	return i, nil
}

// newRegistrars creates registrars for all configured registrations
func newRegistrars(l *zap.Logger, c Client, o K8sOptions) service.Registrars {
	var r service.Registrars

	for _, reg := range o.registrations() {
		instance := service.FormatInstance(reg.scheme(), reg.Address, reg.port())
		if r.Has(instance) {
			l.Warn("skipping duplicate registration", zap.String("instance", instance))
			continue
		}

		registrar := NewRegistrar(RegistrarOptions{
			Client:       c,
			Logger:       l,
			Registration: reg,
		})
		r.Add(instance, registrar)
	}

	return r
}

// NewK8sEnvironment constructs a Kubernetes-based service.Environment using both
// Kubernetes K8sOptions (typically unmarshaled from configuration) and an optional
// extra set of environment options.
func NewK8sEnvironment(l *adapter.Logger, o K8sOptions, eo ...service.Option) (Environment, error) {
	if l == nil {
		l = adapter.DefaultLogger()
	}

	if len(o.Watches) == 0 && len(o.Registrations) == 0 {
		return nil, service.ErrIncomplete
	}

	// Create Kubernetes client
	client, err := clientFactory(o.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create instancers for watching services
	instancers, err := newInstancers(l.Logger, client, o)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create instancers: %w", err)
	}

	// Create registrars for service registration
	registrars := newRegistrars(l.Logger, client, o)

	// Create the base service environment
	baseEnv := service.NewEnvironment(
		append(
			eo,
			service.WithRegistrars(registrars),
			service.WithInstancers(instancers),
			service.WithCloser(func() error {
				client.Close()
				return nil
			}),
		)...,
	)

	return environment{
		Environment: baseEnv,
		client:      client,
	}, nil
}
