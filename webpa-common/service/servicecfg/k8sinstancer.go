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
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/sd"
	"github.com/xmidt-org/webpa-common/v2/adapter"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var (
	errStopped = errors.New("instancer stopped")
)

// InstancerOptions holds the configuration for creating a new Kubernetes instancer
type InstancerOptions struct {
	// Client is the Kubernetes client
	Client Client

	// Logger is the logger to use
	Logger *zap.Logger

	// Watch contains the service watch configuration
	Watch Watch

	// ResyncPeriod is the period for resyncing the cache
	ResyncPeriod time.Duration
}

// NewInstancer creates a new sd.Instancer that watches Kubernetes endpoints
func NewInstancer(o InstancerOptions) sd.Instancer {
	if o.Logger == nil {
		o.Logger = adapter.DefaultLogger().Logger
	}

	namespace := o.Watch.namespace()

	i := &instancer{
		client:       o.Client,
		logger:       o.Logger.With(zap.String("service", o.Watch.Service), zap.String("namespace", namespace)),
		watch:        o.Watch,
		stop:         make(chan struct{}),
		registry:     make(map[chan<- sd.Event]bool),
		resyncPeriod: o.ResyncPeriod,
	}

	// Create informer factory for the namespace
	i.factory = o.Client.NewSharedInformerFactory(namespace, metav1.Duration{Duration: o.ResyncPeriod})

	// Set up endpoints informer with event handlers
	i.informer = i.factory.Core().V1().Endpoints().Informer()
	i.informer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			endpoints, ok := obj.(*corev1.Endpoints)
			if !ok {
				return false
			}
			// Only handle events for our target service
			return endpoints.Name == o.Watch.Service
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    i.onAdd,
			UpdateFunc: i.onUpdate,
			DeleteFunc: i.onDelete,
		},
	})

	// Start the informer
	go i.factory.Start(i.stop)

	// Wait for cache sync
	i.logger.Info("waiting for cache sync")
	if !cache.WaitForCacheSync(i.stop, i.informer.HasSynced) {
		i.logger.Error("failed to sync cache")
		i.update(sd.Event{Err: errors.New("failed to sync kubernetes cache")})
	} else {
		i.logger.Info("cache synced successfully")
		// Get initial state
		instances := i.getCurrentInstances()
		i.logger.Info("initial instances", zap.Int("count", len(instances)), zap.Strings("instances", instances))
		i.update(sd.Event{Instances: instances})
	}

	return i
}

type instancer struct {
	client       Client
	logger       *zap.Logger
	watch        Watch
	factory      informers.SharedInformerFactory
	informer     cache.SharedIndexInformer
	resyncPeriod time.Duration

	stop         chan struct{}
	registerLock sync.Mutex
	state        sd.Event
	registry     map[chan<- sd.Event]bool
}

// getCurrentInstances extracts instances from the current endpoints
func (i *instancer) getCurrentInstances() []string {
	obj, exists, err := i.informer.GetStore().GetByKey(fmt.Sprintf("%s/%s", i.watch.namespace(), i.watch.Service))
	if err != nil || !exists {
		return nil
	}

	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		return nil
	}

	return i.extractInstances(endpoints)
}

// extractInstances converts Kubernetes Endpoints to a list of instance strings
func (i *instancer) extractInstances(endpoints *corev1.Endpoints) []string {
	var instances []string

	for _, subset := range endpoints.Subsets {
		// Find the port to use
		port := i.findPort(subset.Ports)
		if port == 0 {
			continue
		}

		// Add all ready addresses
		for _, addr := range subset.Addresses {
			instance := formatEndpointInstance(i.watch.scheme(), addr.Hostname, endpoints.Namespace, i.watch.Service, port)
			instances = append(instances, instance)
		}
	}

	return instances
}

// findPort finds the appropriate port from the endpoint ports
func (i *instancer) findPort(ports []corev1.EndpointPort) int32 {
	if len(ports) == 0 {
		return 0
	}

	// If a port name is specified, look for it
	if i.watch.PortName != "" {
		for _, p := range ports {
			if p.Name == i.watch.PortName {
				return p.Port
			}
		}
		// Port name not found, return 0
		return 0
	}

	// Otherwise, use the first port
	return ports[0].Port
}

// formatEndpointInstance creates an instance string from endpoint data
func formatEndpointInstance(scheme, address, namespace, serviceName string, port int32) string {
	return fmt.Sprintf("%s://%s.%s.%s.svc.cluster.local:%d", scheme, address, serviceName, namespace, port)
}

// update notifies all registered channels of a new event
func (i *instancer) update(e sd.Event) {
	sort.Strings(e.Instances)
	i.registerLock.Lock()
	defer i.registerLock.Unlock()
	if reflect.DeepEqual(i.state, e) {
		return
	}

	i.state = e
	for c := range i.registry {
		c <- i.state
	}
}

// onAdd handles endpoint add events
func (i *instancer) onAdd(obj interface{}) {
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}

	instances := i.extractInstances(endpoints)
	i.logger.Info("endpoints added",
		zap.String("service", endpoints.Name),
		zap.Int("instanceCount", len(instances)),
		zap.Strings("instances", instances),
	)
	i.update(sd.Event{Instances: instances})
}

// onUpdate handles endpoint update events
func (i *instancer) onUpdate(oldObj, newObj interface{}) {
	endpoints, ok := newObj.(*corev1.Endpoints)
	if !ok {
		return
	}

	instances := i.extractInstances(endpoints)
	i.logger.Info("endpoints updated",
		zap.String("service", endpoints.Name),
		zap.Int("instanceCount", len(instances)),
		zap.Strings("instances", instances),
	)
	i.update(sd.Event{Instances: instances})
}

// onDelete handles endpoint delete events
func (i *instancer) onDelete(obj interface{}) {
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		// Handle DeletedFinalStateUnknown
		if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			endpoints, ok = tombstone.Obj.(*corev1.Endpoints)
			if !ok {
				return
			}
		} else {
			return
		}
	}

	i.logger.Info("endpoints deleted",
		zap.String("service", endpoints.Name),
	)
	i.update(sd.Event{Instances: nil})
}

// Register registers a channel to receive events
func (i *instancer) Register(ch chan<- sd.Event) {
	i.registerLock.Lock()
	defer i.registerLock.Unlock()

	i.registry[ch] = true
	// Push the current state to the new channel
	ch <- i.state
}

// Deregister removes a channel from receiving events
func (i *instancer) Deregister(ch chan<- sd.Event) {
	i.registerLock.Lock()
	defer i.registerLock.Unlock()

	delete(i.registry, ch)
}

// Stop stops the instancer
func (i *instancer) Stop() {
	i.registerLock.Lock()
	defer i.registerLock.Unlock()

	select {
	case <-i.stop:
		// Already stopped
	default:
		close(i.stop)
		i.factory.Shutdown()
	}
}
