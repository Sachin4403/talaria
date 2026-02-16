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
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

const (
	// K8sEndpointUpdates is the metric name for endpoint update events
	K8sEndpointUpdates = "k8s_endpoint_updates_total"

	// K8sEndpointInstances is the metric name for the current number of instances
	K8sEndpointInstances = "k8s_endpoint_instances"

	// K8sWatchErrors is the metric name for watch error events
	K8sWatchErrors = "k8s_watch_errors_total"

	// K8sRegistrations is the metric name for registration operations
	K8sRegistrations = "k8s_registrations_total"

	// K8sCacheSyncs is the metric name for cache sync operations
	K8sCacheSyncs = "k8s_cache_syncs_total"

	// ServiceLabel is the label for service name
	ServiceLabel = "service"

	// NamespaceLabel is the label for namespace
	NamespaceLabel = "namespace"

	// OperationLabel is the label for operation type (add, update, delete)
	OperationLabel = "operation"

	// OutcomeLabel is the label for operation outcome (success, failure)
	OutcomeLabel = "outcome"
)

// Metrics returns the Metrics relevant to the k8s service discovery package
func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name:       K8sEndpointUpdates,
			Type:       xmetrics.CounterType,
			Help:       "Counter for the number of Kubernetes endpoint update events",
			LabelNames: []string{ServiceLabel, NamespaceLabel, OperationLabel},
		},
		{
			Name:       K8sEndpointInstances,
			Type:       xmetrics.GaugeType,
			Help:       "Gauge for the current number of healthy endpoint instances",
			LabelNames: []string{ServiceLabel, NamespaceLabel},
		},
		{
			Name:       K8sWatchErrors,
			Type:       xmetrics.CounterType,
			Help:       "Counter for the number of Kubernetes watch errors",
			LabelNames: []string{ServiceLabel, NamespaceLabel},
		},
		{
			Name:       K8sRegistrations,
			Type:       xmetrics.CounterType,
			Help:       "Counter for Kubernetes registration operations",
			LabelNames: []string{ServiceLabel, NamespaceLabel, OperationLabel, OutcomeLabel},
		},
		{
			Name:       K8sCacheSyncs,
			Type:       xmetrics.CounterType,
			Help:       "Counter for Kubernetes cache sync operations",
			LabelNames: []string{ServiceLabel, NamespaceLabel, OutcomeLabel},
		},
	}
}
