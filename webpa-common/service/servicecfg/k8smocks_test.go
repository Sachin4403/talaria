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

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// mockClient implements the Client interface for testing
type mockClient struct {
	clientset *fake.Clientset
	endpoints map[string]*corev1.Endpoints
	slices    map[string]*discoveryv1.EndpointSliceList
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func newMockClient() *mockClient {
	return &mockClient{
		clientset: fake.NewSimpleClientset(),
		endpoints: make(map[string]*corev1.Endpoints),
		slices:    make(map[string]*discoveryv1.EndpointSliceList),
	}
}

func (m *mockClient) Clientset() kubernetes.Interface {
	return m.clientset
}

func (m *mockClient) GetEndpoints(ctx context.Context, namespace, service string) (*corev1.Endpoints, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	key := namespace + "/" + service
	if ep, ok := m.endpoints[key]; ok {
		return ep, nil
	}
	return nil, nil
}

func (m *mockClient) GetEndpointSlices(ctx context.Context, namespace, service string) (*discoveryv1.EndpointSliceList, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	key := namespace + "/" + service
	if sl, ok := m.slices[key]; ok {
		return sl, nil
	}
	return &discoveryv1.EndpointSliceList{}, nil
}

func (m *mockClient) NewSharedInformerFactory(namespace string, resyncPeriod metav1.Duration) informers.SharedInformerFactory {
	if namespace == "" {
		return informers.NewSharedInformerFactory(m.clientset, resyncPeriod.Duration)
	}
	return informers.NewSharedInformerFactoryWithOptions(
		m.clientset,
		resyncPeriod.Duration,
		informers.WithNamespace(namespace),
	)
}

func (m *mockClient) Close() {}

// Helper function to create test endpoints
func createTestEndpoints(namespace, service string, addresses []string, port int32) *corev1.Endpoints {
	var endpointAddresses []corev1.EndpointAddress
	for _, addr := range addresses {
		endpointAddresses = append(endpointAddresses, corev1.EndpointAddress{IP: addr})
	}

	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service,
			Namespace: namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: endpointAddresses,
				Ports: []corev1.EndpointPort{
					{
						Port:     port,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		},
	}
}
