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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AerospikeConfigSpec holds the raw Aerospike configuration as an unstructured JSON object.
// The data is converted to aerospike.conf format by the operator.
//
// +kubebuilder:object:generate=false
// +kubebuilder:pruning:PreserveUnknownFields
// +kubebuilder:validation:Type=object
type AerospikeConfigSpec struct {
	Value map[string]any `json:"-"`
}

// MarshalJSON implements json.Marshaler.
func (a AerospikeConfigSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Value)
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *AerospikeConfigSpec) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &a.Value)
}

// DeepCopy returns a deep copy of the AerospikeConfigSpec.
func (a *AerospikeConfigSpec) DeepCopy() *AerospikeConfigSpec {
	if a == nil {
		return nil
	}
	out := &AerospikeConfigSpec{
		Value: deepCopyMap(a.Value),
	}
	return out
}

// DeepCopyInto writes a deep copy of AerospikeConfigSpec into out.
func (a *AerospikeConfigSpec) DeepCopyInto(out *AerospikeConfigSpec) {
	out.Value = deepCopyMap(a.Value)
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, val := range m {
		out[key] = deepCopyValue(val)
	}
	return out
}

func deepCopyValue(val any) any {
	switch v := val.(type) {
	case map[string]any:
		return deepCopyMap(v)
	case []any:
		out := make([]any, len(v))
		for i, inner := range v {
			out[i] = deepCopyValue(inner)
		}
		return out
	default:
		return v
	}
}

// TemplateRef is a reference to a cluster-scoped AerospikeClusterTemplate.
type TemplateRef struct {
	// Name is the name of the AerospikeClusterTemplate resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TemplateSnapshotStatus records which template version was resolved and when.
type TemplateSnapshotStatus struct {
	// Name is the name of the referenced AerospikeClusterTemplate.
	Name string `json:"name"`

	// ResourceVersion is the resourceVersion of the template at snapshot time.
	// +optional
	ResourceVersion string `json:"resourceVersion,omitempty"`

	// SnapshotTimestamp is when the snapshot was taken.
	// +optional
	SnapshotTimestamp metav1.Time `json:"snapshotTimestamp,omitempty"`

	// Synced indicates whether the cluster is using the latest version of the template.
	// Set to false when the template changes after the snapshot was taken.
	Synced bool `json:"synced"`

	// Spec is the resolved template spec at snapshot time.
	// +optional
	Spec *AerospikeClusterTemplateSpec `json:"spec,omitempty"`
}

// OperationKind defines the type of on-demand operation.
// +kubebuilder:validation:Enum=WarmRestart;PodRestart
type OperationKind string

const (
	// OperationWarmRestart sends SIGUSR1 to the asd process for a graceful warm restart
	// without data loss.
	OperationWarmRestart OperationKind = "WarmRestart"
	// OperationPodRestart deletes and recreates the pod for a full cold restart.
	OperationPodRestart OperationKind = "PodRestart"
)

// OperationSpec defines an on-demand operation to trigger.
type OperationSpec struct {
	// Kind is the type of operation.
	// WarmRestart sends SIGUSR1 to the Aerospike process.
	// PodRestart deletes and recreates the pod.
	// +kubebuilder:validation:Required
	Kind OperationKind `json:"kind"`

	// ID is a unique identifier for tracking the operation (1-20 chars).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=20
	ID string `json:"id"`

	// PodList is an optional list of specific pod names to target.
	// If empty, the operation applies to all pods.
	// +optional
	PodList []string `json:"podList,omitempty"`
}

// OperationStatus tracks the status of an on-demand operation.
type OperationStatus struct {
	// ID is the operation identifier.
	ID string `json:"id,omitempty"`

	// Kind is the operation type.
	Kind OperationKind `json:"kind,omitempty"`

	// Phase is the operation phase: InProgress, Completed, Error.
	Phase AerospikePhase `json:"phase,omitempty"`

	// CompletedPods lists pods that have completed the operation.
	// +optional
	CompletedPods []string `json:"completedPods,omitempty"`

	// FailedPods lists pods where the operation failed.
	// +optional
	FailedPods []string `json:"failedPods,omitempty"`
}

// ValidationPolicySpec controls validation behavior.
type ValidationPolicySpec struct {
	// SkipWorkDirValidate skips validation that the Aerospike work directory
	// is mounted on persistent storage.
	// +optional
	SkipWorkDirValidate bool `json:"skipWorkDirValidate,omitempty"`
}

// AerospikeObjectMeta defines custom metadata for Kubernetes objects.
type AerospikeObjectMeta struct {
	// Annotations is a map of custom annotations.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels is a map of custom labels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// AerospikeServiceSpec defines custom metadata for a Kubernetes Service.
type AerospikeServiceSpec struct {
	// Metadata defines custom annotations and labels for the service.
	// +optional
	Metadata *AerospikeObjectMeta `json:"metadata,omitempty"`

	// ServiceType is the Kubernetes Service type for per-pod services.
	// Defaults to ClusterIP. Set to NodePort or LoadBalancer for external access.
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +optional
	ServiceType string `json:"serviceType,omitempty"`
}

// AerospikeClusterSpec defines the desired state of an Aerospike CE cluster.
type AerospikeClusterSpec struct {
	// Size is the number of Aerospike nodes (pods) in the cluster.
	// CE limits this to a maximum of 8.
	// When spec.templateRef is set, size may be omitted and the template's default will be used.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=8
	// +optional
	Size int32 `json:"size,omitempty"`

	// Image is the Aerospike CE server container image.
	// Must be a community edition image (e.g., aerospike:ce-8.1.1.1).
	// When spec.templateRef is set, image may be omitted and the template's default will be used.
	// +optional
	Image string `json:"image,omitempty"`

	// AerospikeConfig is the raw Aerospike configuration map.
	// This is converted to aerospike.conf by the operator.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	AerospikeConfig *AerospikeConfigSpec `json:"aerospikeConfig,omitempty"`

	// Storage defines volumes and volume mounts for Aerospike pods.
	// +optional
	Storage *AerospikeStorageSpec `json:"storage,omitempty"`

	// RackConfig defines rack-aware deployment topology.
	// +optional
	RackConfig *RackConfig `json:"rackConfig,omitempty"`

	// AerospikeNetworkPolicy defines how clients access the Aerospike cluster.
	// +optional
	AerospikeNetworkPolicy *AerospikeNetworkPolicy `json:"aerospikeNetworkPolicy,omitempty"`

	// PodSpec defines pod-level configuration (sidecars, resources, scheduling).
	// +optional
	PodSpec *AerospikePodSpec `json:"podSpec,omitempty"`

	// AerospikeAccessControl defines ACL roles and users for CE 7.x+.
	// +optional
	AerospikeAccessControl *AerospikeAccessControlSpec `json:"aerospikeAccessControl,omitempty"`

	// Monitoring configures Prometheus monitoring via an exporter sidecar.
	// +optional
	Monitoring *AerospikeMonitoringSpec `json:"monitoring,omitempty"`

	// NetworkPolicyConfig configures automatic NetworkPolicy creation for the cluster.
	// +optional
	NetworkPolicyConfig *NetworkPolicyConfig `json:"networkPolicyConfig,omitempty"`

	// BandwidthConfig defines bandwidth annotations for CNI traffic shaping.
	// +optional
	BandwidthConfig *BandwidthConfig `json:"bandwidthConfig,omitempty"`

	// EnableDynamicConfigUpdate enables runtime configuration changes
	// without pod restart using Aerospike's set-config command.
	// +optional
	EnableDynamicConfigUpdate *bool `json:"enableDynamicConfigUpdate,omitempty"`

	// RollingUpdateBatchSize is the number of pods to restart in parallel
	// during a rolling restart. Defaults to 1 (sequential).
	// +kubebuilder:validation:Minimum=1
	// +optional
	RollingUpdateBatchSize *int32 `json:"rollingUpdateBatchSize,omitempty"`

	// DisablePDB disables PodDisruptionBudget creation for the cluster.
	// +optional
	DisablePDB *bool `json:"disablePDB,omitempty"`

	// MaxUnavailable is the maximum number of pods that can be unavailable during disruption.
	// Used for PodDisruptionBudget. Defaults to 1.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Paused stops reconciliation when set to true.
	// +optional
	Paused *bool `json:"paused,omitempty"`

	// SeedsFinderServices configures LoadBalancer services for seed discovery.
	// +optional
	SeedsFinderServices *SeedsFinderServices `json:"seedsFinderServices,omitempty"`

	// K8sNodeBlockList contains Kubernetes node names that should not run Aerospike pods.
	//
	// Each name is added as a `kubernetes.io/hostname NotIn` requirement on the
	// pods' required node affinity, so the scheduler will not place a pod on a
	// listed node. Changing the list changes the pod template hash and therefore
	// rolls the rack, which is what moves pods already running on a listed node.
	// Existing pods are not evicted by this field alone — node affinity is
	// IgnoredDuringExecution — they move as the rolling restart recreates them.
	// +optional
	K8sNodeBlockList []string `json:"k8sNodeBlockList,omitempty"`

	// Operations defines on-demand operations (e.g., WarmRestart, PodRestart).
	// Only one operation can be active at a time.
	// +kubebuilder:validation:MaxItems=1
	// +optional
	Operations []OperationSpec `json:"operations,omitempty"`

	// ValidationPolicy controls validation behavior.
	// +optional
	ValidationPolicy *ValidationPolicySpec `json:"validationPolicy,omitempty"`

	// HeadlessService defines custom metadata for the headless service.
	// +optional
	HeadlessService *AerospikeServiceSpec `json:"headlessService,omitempty"`

	// PodService defines custom metadata for per-pod services.
	// When set, the operator creates an individual Service for each pod.
	// +optional
	PodService *AerospikeServiceSpec `json:"podService,omitempty"`

	// TemplateRef references an AerospikeClusterTemplate to use as a configuration base.
	// When set, the template's spec is resolved at creation time and stored as a snapshot
	// in status.templateSnapshot. Template changes are not automatically propagated;
	// use the annotation "acko.io/resync-template: true" to trigger a manual resync.
	// +optional
	TemplateRef *TemplateRef `json:"templateRef,omitempty"`

	// Overrides contains fields that override the referenced template's spec.
	// Merge priority: overrides > template.spec > operator defaults.
	// +optional
	Overrides *AerospikeClusterTemplateSpec `json:"overrides,omitempty"`
}

// Condition type constants for AerospikeCluster status conditions.
const (
	// ConditionAvailable indicates at least one pod is ready to serve requests.
	ConditionAvailable = "Available"
	// ConditionReady indicates all desired pods are running and ready.
	ConditionReady = "Ready"
	// ConditionConfigApplied indicates all pods have the desired Aerospike configuration.
	ConditionConfigApplied = "ConfigApplied"
	// ConditionACLSynced indicates ACL roles and users are synchronized with the cluster.
	ConditionACLSynced = "ACLSynced"
	// ConditionMigrationComplete indicates no data migrations are pending.
	ConditionMigrationComplete = "MigrationComplete"
	// ConditionReconciliationPaused indicates reconciliation is paused by the user.
	ConditionReconciliationPaused = "ReconciliationPaused"
	// ConditionReconcileHealthy indicates whether reconciliation is healthy (no permanent errors).
	// Set to False with reason "PermanentError" when a ValidationError is detected.
	// Set to True with reason "ReconcileSucceeded" when the circuit breaker resets.
	ConditionReconcileHealthy = "ReconcileHealthy"
	// ConditionDynamicConfigDegraded indicates that a dynamic config rollback failed,
	// leaving some pods with inconsistent configuration. Set to True when rollback
	// fails and cleared when the cluster returns to a healthy state.
	ConditionDynamicConfigDegraded = "DynamicConfigDegraded"
)

// AerospikePhase represents the current phase of the cluster.
// +kubebuilder:validation:Enum=InProgress;Completed;Error;ScalingUp;ScalingDown;WaitingForMigration;RollingRestart;ACLSync;Paused;Deleting;ConfigDegraded;BackoffActive
type AerospikePhase string

const (
	// AerospikePhaseInProgress indicates reconciliation is actively in progress (generic).
	AerospikePhaseInProgress AerospikePhase = "InProgress"
	// AerospikePhaseCompleted indicates the cluster has reached the desired state.
	AerospikePhaseCompleted AerospikePhase = "Completed"
	// AerospikePhaseError indicates an unrecoverable error during reconciliation.
	AerospikePhaseError AerospikePhase = "Error"
	// AerospikePhaseScalingUp indicates the cluster is scaling up (adding pods).
	AerospikePhaseScalingUp AerospikePhase = "ScalingUp"
	// AerospikePhaseScalingDown indicates the cluster is scaling down (removing pods).
	AerospikePhaseScalingDown AerospikePhase = "ScalingDown"
	// AerospikePhaseWaitingForMigration indicates a scale-down is deferred because
	// data migration has not yet completed. The controller will retry after the
	// migration finishes to prevent data loss.
	AerospikePhaseWaitingForMigration AerospikePhase = "WaitingForMigration"
	// AerospikePhaseRollingRestart indicates a rolling restart is in progress.
	AerospikePhaseRollingRestart AerospikePhase = "RollingRestart"
	// AerospikePhaseACLSync indicates ACL roles and users are being synchronized.
	AerospikePhaseACLSync AerospikePhase = "ACLSync"
	// AerospikePhasePaused indicates reconciliation is paused by the user.
	AerospikePhasePaused AerospikePhase = "Paused"
	// AerospikePhaseDeleting indicates the cluster is being deleted.
	AerospikePhaseDeleting AerospikePhase = "Deleting"
	// AerospikePhaseConfigDegraded indicates that a dynamic config rollback failed,
	// leaving some pods with inconsistent configuration. The operator will attempt
	// cold restart recovery on the next reconcile.
	AerospikePhaseConfigDegraded AerospikePhase = "ConfigDegraded"
	// AerospikePhaseBackoffActive indicates the reconciler has hit the
	// failed-reconcile threshold and is paused in exponential backoff.
	// The cluster is not actively reconciling until backoff expires.
	AerospikePhaseBackoffActive AerospikePhase = "BackoffActive"
)

// RestartReason describes why a pod was restarted by the operator.
// +kubebuilder:validation:Enum=ConfigChanged;ImageChanged;PodSpecChanged;ManualRestart;WarmRestart
type RestartReason string

const (
	// RestartReasonConfigChanged indicates a cold restart triggered by an Aerospike config change.
	RestartReasonConfigChanged RestartReason = "ConfigChanged"
	// RestartReasonImageChanged indicates the pod image was updated.
	RestartReasonImageChanged RestartReason = "ImageChanged"
	// RestartReasonPodSpecChanged indicates the pod spec (resources, env, etc.) changed.
	RestartReasonPodSpecChanged RestartReason = "PodSpecChanged"
	// RestartReasonManualRestart indicates an on-demand pod restart (OperationPodRestart).
	RestartReasonManualRestart RestartReason = "ManualRestart"
	// RestartReasonWarmRestart indicates an on-demand or rolling warm restart (SIGUSR1).
	RestartReasonWarmRestart RestartReason = "WarmRestart"
)

// AerospikeClusterStatus defines the observed state of the Aerospike CE cluster.
type AerospikeClusterStatus struct {
	// Phase indicates the overall cluster phase.
	// +optional
	Phase AerospikePhase `json:"phase,omitempty"`

	// Size is the current number of ready pods.
	// +optional
	Size int32 `json:"size,omitempty"`

	// Health is a human-readable summary of pod readiness in "ready/total" format (e.g. "1/3").
	// +optional
	Health string `json:"health,omitempty"`

	// Conditions represent the latest observations of the cluster state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Pods contains status information for each pod in the cluster.
	// +optional
	Pods map[string]AerospikePodStatus `json:"pods,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Selector is the label selector for HPA compatibility.
	// +optional
	Selector string `json:"selector,omitempty"`

	// AerospikeConfig is the last applied Aerospike configuration.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	AerospikeConfig *AerospikeConfigSpec `json:"aerospikeConfig,omitempty"`

	// OperationStatus tracks the current on-demand operation status.
	// +optional
	OperationStatus *OperationStatus `json:"operationStatus,omitempty"`

	// PhaseReason provides a human-readable explanation of the current phase.
	// Examples: "Rolling restart in progress for rack 1", "Scaling up rack 2 from 2 to 3 pods".
	// +optional
	PhaseReason string `json:"phaseReason,omitempty"`

	// AppliedSpec is a copy of the last successfully reconciled spec.
	// Use this to detect configuration drift or compare against the current spec.
	// +optional
	AppliedSpec *AerospikeClusterSpec `json:"appliedSpec,omitempty"`

	// AerospikeClusterSize is the Aerospike cluster-size as reported by asinfo.
	// This may differ from the number of ready K8s pods during split-brain or rolling restarts.
	// +optional
	AerospikeClusterSize int32 `json:"aerospikeClusterSize,omitempty"`

	// OperatorVersion is the version of the operator that last reconciled this cluster.
	// Injected via ldflags at build time.
	// +optional
	OperatorVersion string `json:"operatorVersion,omitempty"`

	// PendingRestartPods lists pods that are queued for restart in the current rolling restart.
	// Cleared when the rolling restart completes.
	// +optional
	PendingRestartPods []string `json:"pendingRestartPods,omitempty"`

	// LastReconcileTime is the timestamp of the last successful reconciliation.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// TemplateSnapshot holds the resolved template spec at the time of last sync.
	// This is the basis for template-derived configuration; changes to the template
	// are not propagated until a manual resync is triggered.
	// +optional
	TemplateSnapshot *TemplateSnapshotStatus `json:"templateSnapshot,omitempty"`

	// FailedReconcileCount is the number of consecutive failed reconciliations.
	// Reset to 0 on a successful reconcile. When this exceeds the circuit breaker
	// threshold (default 10), the operator backs off exponentially to prevent
	// excessive retries on persistently failing clusters.
	// +optional
	FailedReconcileCount int32 `json:"failedReconcileCount,omitempty"`

	// LastReconcileError is the error message from the most recent failed reconciliation.
	// Cleared on successful reconciliation.
	// +optional
	LastReconcileError string `json:"lastReconcileError,omitempty"`

	// MigrationStatus tracks data migration progress across the cluster.
	// Updated on each successful reconciliation by querying Aerospike nodes
	// for partition migration statistics.
	// +optional
	MigrationStatus *MigrationStatus `json:"migrationStatus,omitempty"`

	// Endpoints is a comma-separated list of externally reachable endpoints
	// (host:port) for client access. Populated from per-pod LoadBalancer/NodePort
	// services and the seeds finder service. Empty when no external services are configured.
	// +optional
	Endpoints string `json:"endpoints,omitempty"`

	// SeedsEndpoint is the external endpoint (host:port) of the seeds finder
	// LoadBalancer service. Clients use this as the initial seed address.
	// +optional
	SeedsEndpoint string `json:"seedsEndpoint,omitempty"`
}

// MigrationStatus represents the current data migration state of the cluster.
type MigrationStatus struct {
	// InProgress indicates if any data migration is currently happening.
	InProgress bool `json:"inProgress"`
	// RemainingPartitions is the total number of partitions still to be migrated
	// across all nodes (from migrate_partitions_remaining). 0 = complete.
	RemainingPartitions int64 `json:"remainingPartitions"`
	// LastChecked is the timestamp of the last migration check.
	LastChecked metav1.Time `json:"lastChecked"`
}

// DynamicConfigChangeStatus records the result of a single dynamic config change on a pod.
type DynamicConfigChangeStatus struct {
	// Path is the config parameter path (e.g., "service.proto-fd-max").
	Path string `json:"path"`
	// OldValue is the previous value before the change (empty for new keys).
	// +optional
	OldValue string `json:"oldValue,omitempty"`
	// NewValue is the value that was applied or attempted.
	NewValue string `json:"newValue"`
	// Result indicates whether the change succeeded or failed.
	// Possible values: "Applied", "Failed", "RolledBack", "RollbackFailed".
	Result string `json:"result"`
}

// AerospikePodStatus holds per-pod status information.
type AerospikePodStatus struct {
	// PodIP is the IP address of the pod.
	PodIP string `json:"podIP,omitempty"`
	// HostIP is the IP address of the host node.
	HostIP string `json:"hostIP,omitempty"`
	// Image is the container image running on the pod.
	Image string `json:"image,omitempty"`
	// PodPort is the Aerospike service port on the pod.
	PodPort int32 `json:"podPort,omitempty"`
	// ServicePort is the Aerospike service port exposed via node/LB.
	ServicePort int32 `json:"servicePort,omitempty"`
	// Rack is the rack ID assigned to this pod.
	Rack int `json:"rack,omitempty"`
	// InitializedVolumes lists volumes that have been initialized.
	InitializedVolumes []string `json:"initializedVolumes,omitempty"`
	// IsRunningAndReady indicates whether the pod is running and ready.
	IsRunningAndReady bool `json:"isRunningAndReady,omitempty"`
	// ConfigHash is the SHA256 hash of the Aerospike configuration applied to this pod.
	ConfigHash string `json:"configHash,omitempty"`
	// PodSpecHash is the hash of the pod template spec applied to this pod.
	PodSpecHash string `json:"podSpecHash,omitempty"`
	// DynamicConfigStatus indicates the last dynamic config update result.
	// Possible values: "", "Applied", "Failed", "Pending".
	DynamicConfigStatus string `json:"dynamicConfigStatus,omitempty"`
	// DynamicConfigChanges records per-change details from the last dynamic config update.
	// +optional
	DynamicConfigChanges []DynamicConfigChangeStatus `json:"dynamicConfigChanges,omitempty"`
	// DirtyVolumes lists volumes that need initialization or cleanup.
	DirtyVolumes []string `json:"dirtyVolumes,omitempty"`

	// NodeID is the Aerospike-assigned node identifier (e.g. "BB9020012AC4202").
	// Populated by querying the node via asinfo; empty if the node is unreachable.
	// +optional
	NodeID string `json:"nodeID,omitempty"`

	// ClusterName is the Aerospike cluster name as reported by the node.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// AccessEndpoints are the network endpoints (host:port) for direct client access.
	// Populated via the asinfo "service" command.
	// +optional
	AccessEndpoints []string `json:"accessEndpoints,omitempty"`

	// ReadinessGateSatisfied reflects whether the "acko.io/aerospike-ready" readiness gate
	// condition is currently True for this pod. Only meaningful when
	// spec.podSpec.readinessGateEnabled=true.
	// +optional
	ReadinessGateSatisfied bool `json:"readinessGateSatisfied,omitempty"`

	// LastRestartReason is the reason the pod was last restarted by the operator.
	// +optional
	LastRestartReason *RestartReason `json:"lastRestartReason,omitempty"`

	// LastRestartTime is the timestamp when the pod was last restarted by the operator.
	// +optional
	LastRestartTime *metav1.Time `json:"lastRestartTime,omitempty"`

	// UnstableSince records the first time this pod became NotReady.
	// Reset to nil when the pod returns to Ready. Useful for alerting on long-running instability.
	// +optional
	UnstableSince *metav1.Time `json:"unstableSince,omitempty"`

	// MigratingPartitions is the number of partitions this pod is currently migrating.
	// Populated by querying the node's migrate_partitions_remaining statistic.
	// Nil if the node is unreachable or migration info is unavailable.
	// +optional
	MigratingPartitions *int64 `json:"migratingPartitions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.size,statuspath=.status.size,selectorpath=.status.selector
// +kubebuilder:resource:shortName=asc
// +kubebuilder:printcolumn:name="RackSize",type=integer,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=='Available')].status`
// +kubebuilder:printcolumn:name="Seed",type=string,JSONPath=`.status.seedsEndpoint`,priority=0
// +kubebuilder:printcolumn:name="Endpoints",type=string,JSONPath=`.status.endpoints`,priority=1
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`,priority=1
// +kubebuilder:printcolumn:name="ObservedGen",type=integer,JSONPath=`.status.observedGeneration`,priority=1
// +kubebuilder:printcolumn:name="PhaseReason",type=string,JSONPath=`.status.phaseReason`,priority=1
// +kubebuilder:printcolumn:name="AS-Size",type=integer,JSONPath=`.status.aerospikeClusterSize`,priority=1

// AerospikeCluster is the Schema for the aerospikeclusters API.
// It manages the lifecycle of an Aerospike Community Edition cluster.
type AerospikeCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AerospikeClusterSpec   `json:"spec"`
	Status AerospikeClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AerospikeClusterList contains a list of AerospikeCluster.
type AerospikeClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AerospikeCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AerospikeCluster{}, &AerospikeClusterList{})
}
