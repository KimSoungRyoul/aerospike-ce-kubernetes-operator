/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// AerospikeMonitoringSpec defines Prometheus monitoring configuration for the cluster.
type AerospikeMonitoringSpec struct {
	// Enabled enables the Prometheus exporter sidecar.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ExporterImage is the Aerospike Prometheus exporter container image.
	// Defaults to "aerospike/aerospike-prometheus-exporter:1.16.1".
	// +optional
	ExporterImage string `json:"exporterImage,omitempty"`

	// Port is the metrics port for the exporter. Defaults to 9145.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// Resources defines CPU and memory resource requests/limits for the exporter.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Env specifies additional environment variables for the exporter container.
	// These can be used for metric filtering, custom configuration, etc.
	// User-provided env vars are appended last, allowing intentional overrides.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// MetricLabels specifies custom labels to add to all exported metrics.
	// These are passed to the exporter via the METRIC_LABELS environment variable
	// as sorted TOML inline-table entries (key="value" pairs).
	// +optional
	MetricLabels map[string]string `json:"metricLabels,omitempty"`

	// ServiceMonitor configures automatic ServiceMonitor creation for Prometheus Operator.
	// +optional
	ServiceMonitor *ServiceMonitorSpec `json:"serviceMonitor,omitempty"`

	// PrometheusRule configures automatic PrometheusRule creation for Aerospike cluster alerts.
	// +optional
	PrometheusRule *PrometheusRuleSpec `json:"prometheusRule,omitempty"`
}

// ServiceMonitorSpec defines ServiceMonitor configuration.
type ServiceMonitorSpec struct {
	// Enabled enables ServiceMonitor creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Interval is the Prometheus scrape interval. Defaults to "30s".
	// +optional
	Interval string `json:"interval,omitempty"`

	// Labels are additional labels to add to the ServiceMonitor for discovery.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PrometheusRuleSpec defines PrometheusRule configuration for Aerospike cluster alerts.
type PrometheusRuleSpec struct {
	// Enabled enables PrometheusRule creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Labels are additional labels to add to the PrometheusRule for discovery.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// CustomRules completely replaces the default alert rules when provided.
	// When set, the built-in alerts (NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage)
	// are NOT generated. Each entry must be a complete Prometheus rule group object.
	// +optional
	CustomRules []apiextensionsv1.JSON `json:"customRules,omitempty"`
}
