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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// RackConfig defines rack-aware deployment configuration.
type RackConfig struct {
	// Racks is the list of rack definitions.
	// +kubebuilder:validation:MinItems=1
	Racks []Rack `json:"racks"`

	// Namespaces lists Aerospike namespace names that are rack-aware.
	// If empty, all namespaces use the default replication factor.
	// CE supports a maximum of 2 namespaces.
	// +kubebuilder:validation:MaxItems=2
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`

	// ScaleDownBatchSize is the number of pods to scale down simultaneously per rack.
	// Can be an absolute number or percentage string (e.g., "25%").
	// Defaults to 1.
	// +optional
	ScaleDownBatchSize *intstr.IntOrString `json:"scaleDownBatchSize,omitempty"`

	// MaxIgnorablePods is the maximum number of pending/failed pods to ignore
	// during reconciliation. Useful when pods are stuck due to scheduling issues.
	// +optional
	MaxIgnorablePods *intstr.IntOrString `json:"maxIgnorablePods,omitempty"`

	// RollingUpdateBatchSize is the number of pods to restart simultaneously per rack.
	// Can be an absolute number or percentage string (e.g., "25%").
	// Defaults to 1. Takes precedence over spec.rollingUpdateBatchSize when set.
	// +optional
	RollingUpdateBatchSize *intstr.IntOrString `json:"rollingUpdateBatchSize,omitempty"`
}

// Rack defines a single rack in the cluster topology.
type Rack struct {
	// ID is a unique rack identifier (integer). Must be >= 1; rack ID 0 is reserved.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	ID int `json:"id"`

	// Zone is the cloud provider zone label value for scheduling.
	// Maps to topology.kubernetes.io/zone.
	// +optional
	Zone string `json:"zone,omitempty"`

	// Region is the cloud provider region label value for scheduling.
	// Maps to topology.kubernetes.io/region.
	// +optional
	Region string `json:"region,omitempty"`

	// NodeName constrains this rack to a specific Kubernetes node.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// RackLabel is a custom label value for rack-based affinity scheduling.
	// When set, pods are scheduled to nodes with the label "acko.io/rack=<rackLabel>".
	// +optional
	RackLabel string `json:"rackLabel,omitempty"`

	// MaxUnavailable is the maximum number of pods in THIS rack that may be
	// voluntarily disrupted at once. It overrides spec.maxUnavailable for this
	// rack's PodDisruptionBudget.
	//
	// Leave unset to get the default: replication-factor - 1, the number of nodes
	// Aerospike can lose at once without a partition becoming unavailable,
	// measured against this rack's effective aerospikeConfig and floored at 1.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Revision is a version identifier for controlled rack migrations.
	// +optional
	Revision string `json:"revision,omitempty"`

	// AerospikeConfig overrides the cluster-level Aerospike config for this rack.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	AerospikeConfig *AerospikeConfigSpec `json:"aerospikeConfig,omitempty"`

	// Storage overrides the cluster-level storage config for this rack.
	// +optional
	Storage *AerospikeStorageSpec `json:"storage,omitempty"`

	// PodSpec overrides the cluster-level pod spec for this rack.
	// +optional
	PodSpec *RackPodSpec `json:"podSpec,omitempty"`
}

// RackPodSpec defines rack-level pod scheduling overrides.
type RackPodSpec struct {
	// Affinity overrides the cluster-level affinity for this rack.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations overrides the cluster-level tolerations for this rack.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector overrides the cluster-level node selector for this rack.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// AerospikeAccessControlSpec defines ACL configuration for Aerospike CE 7.x+.
type AerospikeAccessControlSpec struct {
	// Roles defines Aerospike roles with privileges.
	// +optional
	Roles []AerospikeRoleSpec `json:"roles,omitempty"`

	// Users defines Aerospike users with role bindings.
	// +optional
	Users []AerospikeUserSpec `json:"users,omitempty"`

	// AdminPolicy defines the client policy for admin operations.
	// +optional
	AdminPolicy *AerospikeClientAdminPolicy `json:"adminPolicy,omitempty"`
}

// AerospikeRoleSpec defines an Aerospike role.
type AerospikeRoleSpec struct {
	// Name is the role name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Privileges is a list of privilege strings (e.g., "read-write", "sys-admin").
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`

	// Whitelist is a list of allowed CIDR ranges for this role.
	// +optional
	Whitelist []string `json:"whitelist,omitempty"`
}

// AerospikeUserSpec defines an Aerospike user.
type AerospikeUserSpec struct {
	// Name is the username.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// SecretName is the Kubernetes Secret containing the user's password.
	// The secret must have a "password" key.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`

	// Roles is a list of role names assigned to this user.
	// +kubebuilder:validation:MinItems=1
	Roles []string `json:"roles"`
}

// AerospikeClientAdminPolicy defines timeout settings for admin client operations.
type AerospikeClientAdminPolicy struct {
	// Timeout is the admin operation timeout in milliseconds.
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=30000
	// +kubebuilder:default=2000
	// +optional
	Timeout int `json:"timeout,omitempty"`
}
