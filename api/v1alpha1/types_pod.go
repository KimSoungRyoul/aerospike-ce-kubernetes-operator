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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// AerospikePodSpec defines pod-level customization for Aerospike pods.
type AerospikePodSpec struct {
	// AerospikeContainerSpec customizes the main Aerospike container.
	// +optional
	AerospikeContainerSpec *AerospikeContainerSpec `json:"aerospikeContainer,omitempty"`

	// Sidecars is a list of sidecar containers to add to the pod.
	// +optional
	Sidecars []corev1.Container `json:"sidecars,omitempty"`

	// InitContainers is a list of additional init containers to add to the pod.
	// These run after the operator's built-in init container.
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// ImagePullSecrets is a list of references to secrets for pulling images.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// NodeSelector is a map of node labels for pod scheduling.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are tolerations for pod scheduling.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity defines pod affinity/anti-affinity rules.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// SecurityContext defines pod-level security attributes.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// DNSPolicy defines the DNS policy for the pod.
	// +optional
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty"`

	// HostNetwork enables host networking for the pod.
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// MultiPodPerHost controls whether multiple Aerospike pods can be scheduled
	// on the same Kubernetes node. When false (or nil with hostNetwork=true),
	// a RequiredDuringSchedulingIgnoredDuringExecution pod anti-affinity rule
	// is automatically injected to ensure one pod per node.
	// +optional
	MultiPodPerHost *bool `json:"multiPodPerHost,omitempty"`

	// TerminationGracePeriodSeconds is the grace period for pod termination.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// TopologySpreadConstraints describes how pods ought to spread across topology domains.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// PodManagementPolicy controls how pods are created during scaling.
	// Valid values are OrderedReady (default) or Parallel.
	// +kubebuilder:validation:Enum=OrderedReady;Parallel
	// +optional
	PodManagementPolicy appsv1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`

	// Metadata defines additional labels and annotations for the pods.
	// +optional
	Metadata *AerospikePodMetadata `json:"metadata,omitempty"`

	// ReadinessGateEnabled enables the custom Pod Readiness Gate "acko.io/aerospike-ready".
	// When true, the operator injects the gate into each pod's spec and patches
	// pod.Status.Conditions to True only after Aerospike has joined the cluster mesh
	// and finished all data migrations. Pods are excluded from Service endpoints
	// until the gate is satisfied.
	// Defaults to false for backward compatibility.
	// +optional
	ReadinessGateEnabled *bool `json:"readinessGateEnabled,omitempty"`

	// PriorityClassName is the name of the PriorityClass for Aerospike pods.
	// This controls pod scheduling priority and preemption behavior.
	// The PriorityClass must already exist in the cluster.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// AerospikeContainerSpec customizes the Aerospike server container.
type AerospikeContainerSpec struct {
	// Resources defines CPU and memory resource requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// SecurityContext defines container-level security attributes.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// AerospikePodMetadata defines extra labels and annotations for pods.
type AerospikePodMetadata struct {
	// Labels are additional labels to add to the pods.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are additional annotations to add to the pods.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}
