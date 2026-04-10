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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodAntiAffinityLevel defines the level of pod anti-affinity to apply.
// +kubebuilder:validation:Enum=none;preferred;required
type PodAntiAffinityLevel string

const (
	// PodAntiAffinityNone disables automatic pod anti-affinity rules.
	PodAntiAffinityNone PodAntiAffinityLevel = "none"
	// PodAntiAffinityPreferred adds a preferred (soft) pod anti-affinity rule.
	PodAntiAffinityPreferred PodAntiAffinityLevel = "preferred"
	// PodAntiAffinityRequired adds a required (hard) pod anti-affinity rule.
	// Pods will not be scheduled on the same node if another Aerospike pod exists.
	PodAntiAffinityRequired PodAntiAffinityLevel = "required"
)

// TemplateHeartbeatConfig defines heartbeat configuration defaults.
type TemplateHeartbeatConfig struct {
	// Mode is the heartbeat mode. Must be "mesh" for CE.
	// +optional
	Mode string `json:"mode,omitempty"`

	// Interval is the heartbeat interval in milliseconds.
	// +optional
	Interval int `json:"interval,omitempty"`

	// Timeout is the heartbeat timeout in milliseconds.
	// +optional
	Timeout int `json:"timeout,omitempty"`
}

// TemplateNetworkConfig defines network configuration defaults.
type TemplateNetworkConfig struct {
	// Heartbeat defines heartbeat configuration defaults.
	// +optional
	Heartbeat *TemplateHeartbeatConfig `json:"heartbeat,omitempty"`
}

// TemplateAerospikeConfig defines Aerospike configuration defaults for a template.
//
// +kubebuilder:object:generate=false
type TemplateAerospikeConfig struct {
	// NamespaceDefaults is applied as a base configuration to every namespace
	// defined in the cluster's aerospikeConfig. Cluster-level namespace settings
	// override these defaults. Uses the same format as aerospikeConfig.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	NamespaceDefaults *AerospikeConfigSpec `json:"namespaceDefaults,omitempty"`

	// Service defines service section defaults for aerospikeConfig.
	// Uses the same format as aerospikeConfig.service.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Service *AerospikeConfigSpec `json:"service,omitempty"`

	// Network defines network configuration defaults.
	// +optional
	Network *TemplateNetworkConfig `json:"network,omitempty"`
}

// TemplateScheduling defines scheduling configuration defaults.
type TemplateScheduling struct {
	// PodAntiAffinityLevel sets the pod anti-affinity policy.
	// "none": no anti-affinity; "preferred": soft rule; "required": hard rule.
	// +optional
	PodAntiAffinityLevel PodAntiAffinityLevel `json:"podAntiAffinityLevel,omitempty"`

	// NodeAffinity defines node affinity rules for pod scheduling.
	// +optional
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`

	// Tolerations defines pod scheduling tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// TopologySpreadConstraints defines how pods are spread across topology domains.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// PodManagementPolicy controls how pods are created during initial scale up,
	// when replacing pods on nodes, and when scaling down.
	// +optional
	PodManagementPolicy appsv1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`
}

// TemplateStorage defines storage defaults for the cluster's data volume.
type TemplateStorage struct {
	// StorageClassName is the Kubernetes StorageClass to use for the data PVC.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// VolumeMode specifies whether the volume should be formatted or used raw.
	// +optional
	VolumeMode corev1.PersistentVolumeMode `json:"volumeMode,omitempty"`

	// AccessModes defines the access modes for the data PVC.
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`

	// Resources defines the storage resource requirements for the data PVC.
	// +optional
	Resources corev1.VolumeResourceRequirements `json:"resources,omitempty"`

	// LocalPVRequired indicates that a local PersistentVolume with
	// WaitForFirstConsumer binding mode is required.
	// When true, storageClassName must also be specified.
	// +optional
	LocalPVRequired bool `json:"localPVRequired,omitempty"`
}

// TemplateRackConfig defines rack-level configuration defaults.
type TemplateRackConfig struct {
	// MaxRacksPerNode is the maximum number of racks allowed per Kubernetes node.
	// Used for cross-field validation (e.g., maxRacksPerNode==1 requires podAntiAffinityLevel==required).
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxRacksPerNode int `json:"maxRacksPerNode,omitempty"`
}

// AerospikeClusterTemplateSpec defines the reusable configuration profile.
type AerospikeClusterTemplateSpec struct {
	// Description describes the intended purpose and recommended environment for this template.
	// E.g., "Single-node cluster for development" or "Production multi-rack cluster"
	// +kubebuilder:validation:MaxLength=500
	// +optional
	Description string `json:"description,omitempty"`

	// AerospikeConfig defines Aerospike configuration defaults.
	// +optional
	AerospikeConfig *TemplateAerospikeConfig `json:"aerospikeConfig,omitempty"`

	// Scheduling defines pod scheduling defaults.
	// +optional
	Scheduling *TemplateScheduling `json:"scheduling,omitempty"`

	// Storage defines the default data volume configuration.
	// +optional
	Storage *TemplateStorage `json:"storage,omitempty"`

	// Resources defines default CPU/memory resource requests and limits
	// for the Aerospike server container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// RackConfig defines rack-level configuration defaults.
	// +optional
	RackConfig *TemplateRackConfig `json:"rackConfig,omitempty"`

	// Image is the default Aerospike CE container image for clusters using this template.
	// Must be a community edition image (e.g., aerospike:ce-8.1.1.1).
	// Clusters can override this by explicitly setting spec.image.
	// +optional
	Image string `json:"image,omitempty"`

	// Size is the default number of Aerospike nodes for clusters using this template.
	// CE limits this to a maximum of 8.
	// Clusters that explicitly set spec.size (non-zero) will override this value.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8
	// +optional
	Size *int32 `json:"size,omitempty"`

	// Monitoring configures default Prometheus monitoring via an exporter sidecar.
	// Clusters that explicitly set spec.monitoring will override this entirely.
	// +optional
	Monitoring *AerospikeMonitoringSpec `json:"monitoring,omitempty"`

	// AerospikeNetworkPolicy defines the default network access configuration.
	// Clusters that explicitly set spec.aerospikeNetworkPolicy will override this entirely.
	// +optional
	AerospikeNetworkPolicy *AerospikeNetworkPolicy `json:"aerospikeNetworkPolicy,omitempty"`
}

// AerospikeClusterTemplateStatus defines the observed state of AerospikeClusterTemplate.
type AerospikeClusterTemplateStatus struct {
	// UsedBy lists the AerospikeCluster resources that reference this template.
	// +optional
	UsedBy []string `json:"usedBy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=asct,scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="AntiAffinity",type=string,JSONPath=`.spec.scheduling.podAntiAffinityLevel`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.spec.storage.storageClassName`,priority=1
// +kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`,priority=1

// AerospikeClusterTemplate is a reusable configuration profile for AerospikeCluster.
// Clusters reference a template via spec.templateRef and can override individual fields
// via spec.overrides. Template changes are not automatically propagated to clusters;
// use the annotation "acko.io/resync-template: true" to trigger a manual resync.
type AerospikeClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AerospikeClusterTemplateSpec   `json:"spec,omitempty"`
	Status AerospikeClusterTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AerospikeClusterTemplateList contains a list of AerospikeClusterTemplate.
type AerospikeClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AerospikeClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AerospikeClusterTemplate{}, &AerospikeClusterTemplateList{})
}
